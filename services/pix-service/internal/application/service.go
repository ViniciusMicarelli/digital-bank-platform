// Package application implements Pix keys and transfers.
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/document"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/ports"
)

const (
	source = "pix-service"
	topic  = "bank.pix.events"
	lease  = 30 * time.Second
)

// ErrAccountNotActive is returned when the customer account cannot use Pix.
var ErrAccountNotActive = errors.New("account is not active")

type Service struct {
	store    ports.Store
	accounts ports.Accounts
	txs      ports.Transactions
	dict     ports.Directory
	spi      ports.SPI
	log      *slog.Logger
	now      func() time.Time
	qr       *QRSigner
	fetcher  ports.QRFetcher
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func New(store ports.Store, accounts ports.Accounts, txs ports.Transactions, dict ports.Directory, spi ports.SPI, log *slog.Logger) *Service {
	return &Service{store: store, accounts: accounts, txs: txs, dict: dict, spi: spi, log: log, now: time.Now}
}

// RegisterKey adds a key to an active account. A CPF key must be the holder's own CPF.
func (s *Service) RegisterKey(ctx context.Context, account uuid.UUID, t domain.KeyType, value string) (domain.Key, error) {
	holder, err := s.accounts.Holder(ctx, account)
	if err != nil {
		return domain.Key{}, err
	}
	if !holder.Active {
		return domain.Key{}, ErrAccountNotActive
	}
	k, err := domain.NewKey(account, t, value)
	if err != nil {
		return domain.Key{}, err
	}
	if t == domain.KeyCPF && k.Value != holder.Document {
		return domain.Key{}, fmt.Errorf("%w: a CPF key must be the account holder's CPF", domain.ErrInvalidArgument)
	}
	ev, err := s.event(ctx, "pix.key.registered", k.ID.String(), keyPayload(k))
	if err != nil {
		return domain.Key{}, err
	}
	if err := s.store.InsertKey(ctx, k, ev); err != nil {
		return domain.Key{}, err
	}
	k.CreatedAt = s.now()
	return k, nil
}

func (s *Service) DeleteKey(ctx context.Context, account, keyID uuid.UUID) error {
	return s.store.DeleteKey(ctx, account, keyID, func(k domain.Key) (outbox.Message, error) {
		return s.event(ctx, "pix.key.deleted", k.ID.String(), keyPayload(k))
	})
}

func (s *Service) Keys(ctx context.Context, account uuid.UUID) ([]domain.Key, error) {
	return s.store.Keys(ctx, account)
}

// Lookup resolves a key typed by the customer. For our own keys it also
// returns the account; for other banks it asks the DICT.
func (s *Service) Lookup(ctx context.Context, raw string) (domain.KeyType, string, domain.Party, uuid.UUID, error) {
	t, value, err := domain.DetectKeyType(raw)
	if err != nil {
		return "", "", domain.Party{}, uuid.Nil, err
	}
	k, err := s.store.KeyByValue(ctx, value)
	switch {
	case err == nil:
		holder, err := s.accounts.Holder(ctx, k.AccountID)
		if err != nil {
			return "", "", domain.Party{}, uuid.Nil, err
		}
		if !holder.Active {
			return "", "", domain.Party{}, uuid.Nil, fmt.Errorf("%w: key %s", domain.ErrNotFound, value)
		}
		return t, value, domain.Party{
			Name: holder.Name, DocumentMasked: document.MaskCPF(holder.Document),
			ISPB: domain.OurISPB, BankName: domain.OurBankName,
		}, k.AccountID, nil
	case errors.Is(err, domain.ErrNotFound):
		p, err := s.dict.Lookup(ctx, value)
		return t, value, p, uuid.Nil, err
	default:
		return "", "", domain.Party{}, uuid.Nil, err
	}
}

// Contacts are favorites first, then the most recent receivers.
func (s *Service) Contacts(ctx context.Context, account uuid.UUID) ([]domain.Contact, error) {
	return s.store.Contacts(ctx, account, 30)
}

// SaveFavorite stores a key as a favorite: it must resolve (our customer or
// the DICT) and not be one of the account's own keys.
func (s *Service) SaveFavorite(ctx context.Context, account uuid.UUID, key, nickname string) (domain.Contact, error) {
	nickname = strings.TrimSpace(nickname)
	if utf8.RuneCountInString(nickname) > 30 {
		return domain.Contact{}, fmt.Errorf("%w: nickname longer than 30 characters", domain.ErrInvalidArgument)
	}
	_, value, party, owner, err := s.Lookup(ctx, key)
	if err != nil {
		return domain.Contact{}, err
	}
	if owner == account {
		return domain.Contact{}, domain.Reject("OWN_KEY", "this key is yours")
	}
	c := domain.Contact{Key: value, Name: party.Name, BankName: party.BankName, Nickname: nickname, Favorite: true}
	return c, s.store.SaveFavorite(ctx, account, c)
}

func (s *Service) RemoveFavorite(ctx context.Context, account uuid.UUID, key string) error {
	if _, value, err := domain.DetectKeyType(key); err == nil {
		key = value
	}
	return s.store.RemoveFavorite(ctx, account, key)
}

// Send creates an outgoing Pix and drives it as far as it can go.
func (s *Service) Send(ctx context.Context, idemKey string, from uuid.UUID, key string, amount money.Money, description string) (domain.Transfer, error) {
	return s.send(ctx, idemKey, from, key, amount, description, "")
}

func (s *Service) send(ctx context.Context, idemKey string, from uuid.UUID, key string, amount money.Money, description, qrTxID string) (domain.Transfer, error) {
	_, value, receiver, receiverAccount, err := s.Lookup(ctx, key)
	if err != nil {
		return domain.Transfer{}, err
	}
	t, err := domain.NewOutgoing(idemKey, from, value, receiver, receiverAccount, amount, description, s.now())
	if err != nil {
		return domain.Transfer{}, err
	}
	t.QRTxID = qrTxID
	existing, fp, err := s.store.CreateTransfer(ctx, t)
	if err != nil {
		return domain.Transfer{}, err
	}
	if existing != nil {
		if string(fp) != string(t.Fingerprint()) {
			return domain.Transfer{}, fmt.Errorf("%w: %q", domain.ErrIdempotencyConflict, idemKey)
		}
		if existing.Status.Terminal() {
			return *existing, nil
		}
		t = *existing
	}
	if qrTxID != "" {
		// Reserve the charge before any money moves; a second payer of a
		// one-time code is rejected here, not after paying.
		if err := s.store.ClaimQRCode(ctx, qrTxID, t.ID); errors.Is(err, domain.ErrQRUnavailable) {
			if err := s.finish(ctx, &t, domain.StatusRejected, "QR_CODE_UNAVAILABLE"); err != nil {
				return domain.Transfer{}, err
			}
			return s.store.Transfer(ctx, t.ID)
		} else if err != nil {
			return domain.Transfer{}, err
		}
	}
	return s.Process(ctx, t.ID)
}

// QRCity is printed on the QR codes this bank issues.
const QRCity = "SAO PAULO"

// CreateQRCode issues a charge for one of the account's keys. With oneTime it
// can be paid once; ttl (0 = no expiry) bounds how long it is valid.
func (s *Service) CreateQRCode(ctx context.Context, account uuid.UUID, key string, amount *money.Money, description string, oneTime, dynamic bool, ttl time.Duration) (domain.QRCode, string, error) {
	if dynamic && s.qr == nil {
		return domain.QRCode{}, "", ErrDynamicQRDisabled
	}
	holder, err := s.accounts.Holder(ctx, account)
	if err != nil {
		return domain.QRCode{}, "", err
	}
	if !holder.Active {
		return domain.QRCode{}, "", ErrAccountNotActive
	}
	_, value, err := domain.DetectKeyType(key)
	if err != nil {
		return domain.QRCode{}, "", err
	}
	k, err := s.store.KeyByValue(ctx, value)
	if err != nil || k.AccountID != account {
		return domain.QRCode{}, "", fmt.Errorf("%w: the key must belong to the account", domain.ErrInvalidArgument)
	}
	if ttl < 0 || ttl > 365*24*time.Hour {
		return domain.QRCode{}, "", fmt.Errorf("%w: expiry must be between 0 and 365 days", domain.ErrInvalidArgument)
	}
	q := domain.QRCode{
		TxID: domain.NewQRTxID(), AccountID: account, Key: value, Amount: amount,
		Description: description, OneTime: oneTime, CreatedAt: s.now(),
	}
	if ttl > 0 {
		exp := s.now().Add(ttl)
		q.ExpiresAt = &exp
	}
	code := domain.BRCode{
		Key: value, Amount: amount, MerchantName: holder.Name, MerchantCity: QRCity,
		TxID: q.TxID, Description: description, OneTime: oneTime,
	}
	if dynamic {
		// The code only points at the signed payload: key and amount travel
		// in the JWS, so they can be changed or revoked after printing.
		code = domain.BRCode{URL: s.qr.Location + "/" + q.TxID, MerchantName: holder.Name, MerchantCity: QRCity, OneTime: oneTime}
	}
	payload, err := code.Encode()
	if err != nil {
		return domain.QRCode{}, "", err
	}
	if err := s.store.InsertQRCode(ctx, q); err != nil {
		return domain.QRCode{}, "", err
	}
	return q, payload, nil
}

// DecodedQR is what a payer sees before confirming.
type DecodedQR struct {
	Code     domain.BRCode
	Receiver domain.Party
	// Charge is set when the code was issued by this bank.
	Charge *domain.QRCode
}

// DecodeQRCode validates a payload and resolves its receiver.
func (s *Service) DecodeQRCode(ctx context.Context, payload string) (DecodedQR, error) {
	code, err := domain.DecodeBRCode(payload)
	if err != nil {
		return DecodedQR{}, err
	}
	if code.URL != "" {
		if err := s.resolveDynamic(ctx, &code); err != nil {
			return DecodedQR{}, err
		}
	}
	_, value, receiver, _, err := s.Lookup(ctx, code.Key)
	if err != nil {
		return DecodedQR{}, err
	}
	out := DecodedQR{Code: code, Receiver: receiver}
	if code.HasTxID() {
		if q, err := s.store.QRCode(ctx, code.TxID); err == nil {
			// The checksum proves integrity, not origin: make sure the code
			// still matches the charge we issued.
			if q.Key != value || !sameAmount(q.Amount, code.Amount) {
				return DecodedQR{}, fmt.Errorf("%w: QR code does not match the issued charge", domain.ErrInvalidArgument)
			}
			if err := q.Payable(s.now(), uuid.Nil); err != nil {
				return DecodedQR{}, err
			}
			out.Charge = &q
		} else if !errors.Is(err, domain.ErrNotFound) {
			return DecodedQR{}, err
		}
	}
	return out, nil
}

func sameAmount(a, b *money.Money) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// PayQRCode pays a QR code. amount is required only when the code has none.
func (s *Service) PayQRCode(ctx context.Context, idemKey string, from uuid.UUID, payload string, amount *money.Money, description string) (domain.Transfer, error) {
	d, err := s.DecodeQRCode(ctx, payload)
	if err != nil {
		// A retry after a successful payment of a one-time code must still
		// return that payment, so look the idempotency key up first.
		if errors.Is(err, domain.ErrQRUnavailable) {
			if t, ok := s.existingByKey(ctx, idemKey); ok {
				return t, nil
			}
		}
		return domain.Transfer{}, err
	}
	pay := d.Code.Amount
	switch {
	case pay == nil && amount == nil:
		return domain.Transfer{}, fmt.Errorf("%w: this QR code has no amount, inform one", domain.ErrInvalidArgument)
	case pay != nil && amount != nil && *amount != *pay:
		return domain.Transfer{}, fmt.Errorf("%w: this QR code is for %s", domain.ErrInvalidArgument, pay.Decimal())
	case pay == nil:
		pay = amount
	}
	if description == "" {
		description = d.Code.Description
	}
	txid := ""
	if d.Charge != nil {
		txid = d.Charge.TxID
	}
	return s.send(ctx, idemKey, from, d.Code.Key, *pay, description, txid)
}

func (s *Service) existingByKey(ctx context.Context, idemKey string) (domain.Transfer, bool) {
	t, err := s.store.TransferByKey(ctx, idemKey)
	return t, err == nil
}

// Refund returns (part of) a Pix the account received. Concurrent refunds
// can never add up to more than the original amount.
func (s *Service) Refund(ctx context.Context, idemKey string, account, originalID uuid.UUID, amount money.Money, reason string) (domain.Transfer, error) {
	original, err := s.store.Transfer(ctx, originalID)
	if err != nil {
		return domain.Transfer{}, err
	}
	if original.AccountID != account {
		return domain.Transfer{}, fmt.Errorf("%w: pix transfer", domain.ErrNotFound)
	}
	now := s.now()
	refund, err := domain.NewRefund(idemKey, original, amount, reason, now)
	if err != nil {
		if t, ok := s.existingByKey(ctx, idemKey); ok && t.RefundOf == originalID {
			return t, nil // retry after the refund already used up the balance
		}
		return domain.Transfer{}, err
	}
	existing, fp, err := s.store.CreateRefund(ctx, refund, func(locked domain.Transfer) error {
		// Re-check under the row lock: another refund may have just run.
		_, err := domain.NewRefund(idemKey, locked, amount, reason, now)
		return err
	})
	if err != nil {
		return domain.Transfer{}, err
	}
	if existing != nil {
		if string(fp) != string(refund.Fingerprint()) {
			return domain.Transfer{}, fmt.Errorf("%w: %q", domain.ErrIdempotencyConflict, idemKey)
		}
		if existing.Status.Terminal() {
			return *existing, nil
		}
		refund = *existing
	}
	return s.Process(ctx, refund.ID)
}

// Receive handles a Pix sent to one of our keys by another bank (via SPI).
// It is idempotent on the E2E id.
func (s *Service) Receive(ctx context.Context, e2e, key string, amount money.Money, payer domain.Party, description string) (domain.Transfer, error) {
	if !domain.ValidEndToEndID(e2e) {
		return domain.Transfer{}, fmt.Errorf("%w: invalid end-to-end id", domain.ErrInvalidArgument)
	}
	if !amount.IsPositive() || amount.Currency() != money.BRL {
		return domain.Transfer{}, fmt.Errorf("%w: invalid amount", domain.ErrInvalidArgument)
	}
	_, value, err := domain.DetectKeyType(key)
	if err != nil {
		return domain.Transfer{}, err
	}
	k, err := s.store.KeyByValue(ctx, value)
	if err != nil {
		return domain.Transfer{}, err
	}
	if payer.ISPB == "" {
		payer.ISPB = "00000000"
	}
	t := domain.Transfer{
		ID: uuid.Must(uuid.NewV7()), IdempotencyKey: "in:" + e2e, EndToEndID: e2e,
		Direction: domain.Incoming, Status: domain.StatusCreated, AccountID: k.AccountID,
		Key: value, Counterparty: payer, Amount: amount, Description: description,
	}
	existing, _, err := s.store.CreateTransfer(ctx, t)
	if err != nil {
		return domain.Transfer{}, err
	}
	if existing != nil {
		if existing.Status.Terminal() {
			return *existing, nil
		}
		t = *existing
	}
	return s.Process(ctx, t.ID)
}

// Transfers lists an account's Pix, newest first.
func (s *Service) Transfers(ctx context.Context, account uuid.UUID, direction domain.Direction, beforeAt time.Time, beforeID uuid.UUID, limit int) ([]domain.Transfer, error) {
	return s.store.Transfers(ctx, account, direction, beforeAt, beforeID, limit)
}

func (s *Service) Transfer(ctx context.Context, id uuid.UUID) (domain.Transfer, error) {
	return s.store.Transfer(ctx, id)
}

// Process advances a transfer under a lease. Every call it makes downstream is
// idempotent (keys derive from the transfer id / E2E id), so it can be
// re-run after any failure.
func (s *Service) Process(ctx context.Context, id uuid.UUID) (domain.Transfer, error) {
	t, ok, err := s.store.Acquire(ctx, id, lease)
	if err != nil {
		return domain.Transfer{}, err
	}
	if !ok {
		return t, fmt.Errorf("%w: %s", domain.ErrBusy, id)
	}
	runErr := s.run(ctx, &t)
	bg := context.WithoutCancel(ctx)
	if err := s.store.Release(bg, id); err != nil {
		s.log.ErrorContext(ctx, "release lease", "transfer_id", id, "error", err)
	}
	latest, err := s.store.Transfer(bg, id)
	if err != nil {
		return t, errors.Join(runErr, err)
	}
	return latest, runErr
}

func (s *Service) run(ctx context.Context, t *domain.Transfer) error {
	for !t.Status.Terminal() {
		var err error
		switch {
		case t.Direction == domain.Incoming:
			err = s.credit(ctx, t)
		case t.Status == domain.StatusCreated && t.Internal():
			err = s.sendInternal(ctx, t)
		case t.Status == domain.StatusCreated:
			err = s.reserve(ctx, t)
		case t.Status == domain.StatusFundsReserved:
			t.Status = domain.StatusSentToSPI // record the intent before calling SPI
			err = s.store.SaveTransfer(ctx, *t)
		case t.Status == domain.StatusSentToSPI:
			err = s.settle(ctx, t)
		default:
			return fmt.Errorf("unexpected state %s", t.Status)
		}
		if err != nil {
			s.log.WarnContext(ctx, "pix step failed, will retry", "transfer_id", t.ID, "status", t.Status, "error", err)
			return err
		}
	}
	return nil
}

func (s *Service) metadata(t *domain.Transfer) map[string]string {
	md := map[string]string{
		"pix_transfer_id":   t.ID.String(),
		"end_to_end_id":     t.EndToEndID,
		"pix_key":           t.Key,
		"counterparty_name": t.Counterparty.Name,
		"counterparty_bank": t.Counterparty.BankName,
	}
	if t.RefundOf != uuid.Nil {
		md["refund_of"] = t.RefundOf.String()
	}
	if t.QRTxID != "" {
		md["qr_txid"] = t.QRTxID
	}
	return md
}

// sendInternal moves money between two of our accounts in one saga.
func (s *Service) sendInternal(ctx context.Context, t *domain.Transfer) error {
	res, err := s.txs.Transfer(ctx, "pix:"+t.ID.String(), t.AccountID, t.InternalAccountID, t.Amount, t.Description, s.metadata(t))
	if err != nil {
		return s.rejectedOr(ctx, t, err)
	}
	t.TransactionID = res.ID
	switch res.Status {
	case "COMPLETED":
		return s.finish(ctx, t, domain.StatusCompleted, "")
	case "FAILED":
		return s.finish(ctx, t, domain.StatusRejected, res.FailureReason)
	}
	return fmt.Errorf("transaction %s still %s", res.ID, res.Status)
}

// reserve consumes the limit and holds the funds for an external transfer.
func (s *Service) reserve(ctx context.Context, t *domain.Transfer) error {
	res, err := s.txs.ReserveOutgoing(ctx, "pix:"+t.ID.String(), t.AccountID, t.Amount, t.Description, s.metadata(t))
	if err != nil {
		return s.rejectedOr(ctx, t, err)
	}
	t.TransactionID = res.ID
	switch res.Status {
	case "RESERVED":
		t.Status = domain.StatusFundsReserved
		return s.store.SaveTransfer(ctx, *t)
	case "FAILED":
		return s.finish(ctx, t, domain.StatusRejected, res.FailureReason)
	}
	return fmt.Errorf("transaction %s still %s", res.ID, res.Status)
}

// settle sends the payment through SPI, then captures or releases the hold.
func (s *Service) settle(ctx context.Context, t *domain.Transfer) error {
	err := s.spi.Send(ctx, t.EndToEndID, t.Counterparty, t.Key, t.Amount)
	if rej, ok := domain.AsRejected(err); ok {
		if _, err := s.txs.Compensate(ctx, t.TransactionID, rej.Reason); err != nil {
			return err
		}
		return s.finish(ctx, t, domain.StatusFailed, rej.Reason)
	}
	if err != nil {
		return err
	}
	if _, err := s.txs.Complete(ctx, t.TransactionID); err != nil {
		return err
	}
	return s.finish(ctx, t, domain.StatusCompleted, "")
}

// credit posts an incoming Pix to the customer's account.
func (s *Service) credit(ctx context.Context, t *domain.Transfer) error {
	res, err := s.txs.CreditIncoming(ctx, "pix-in:"+t.EndToEndID, t.AccountID, t.Amount, t.Description, s.metadata(t))
	if err != nil {
		return s.rejectedOr(ctx, t, err)
	}
	t.TransactionID = res.ID
	if res.Status == "COMPLETED" {
		return s.finish(ctx, t, domain.StatusCompleted, "")
	}
	// The real SPI would return the money to the payer's bank here.
	return s.finish(ctx, t, domain.StatusRejected, res.FailureReason)
}

func (s *Service) rejectedOr(ctx context.Context, t *domain.Transfer, err error) error {
	if rej, ok := domain.AsRejected(err); ok {
		return s.finish(ctx, t, domain.StatusRejected, rej.Reason)
	}
	return err
}

func (s *Service) finish(ctx context.Context, t *domain.Transfer, st domain.Status, reason string) error {
	t.Status, t.FailureReason = st, reason
	eventType := "pix.transfer.completed"
	switch {
	case st != domain.StatusCompleted:
		eventType = "pix.transfer.failed"
	case t.Direction == domain.Incoming:
		eventType = "pix.transfer.received"
	}
	ev, err := s.event(ctx, eventType, t.ID.String(), transferPayload(*t))
	if err != nil {
		return err
	}
	if st == domain.StatusCompleted && t.Direction == domain.Outgoing && t.Internal() {
		if err := s.completeInternal(ctx, t, ev); err != nil {
			return err
		}
	} else if err := s.store.SaveTransfer(ctx, *t, ev); err != nil {
		return err
	}
	telemetry.BusinessEvents.WithLabelValues(eventType, string(t.Status)).Inc()
	return nil
}

// completeInternal also records the Pix as received by the other customer.
func (s *Service) completeInternal(ctx context.Context, t *domain.Transfer, sent outbox.Message) error {
	sender, err := s.accounts.Holder(ctx, t.AccountID)
	if err != nil {
		return fmt.Errorf("read sender: %w", err)
	}
	in := t.IncomingMirror(domain.Party{
		Name: sender.Name, DocumentMasked: document.MaskCPF(sender.Document),
		ISPB: domain.OurISPB, BankName: domain.OurBankName,
	})
	received, err := s.event(ctx, "pix.transfer.received", in.ID.String(), transferPayload(in))
	if err != nil {
		return err
	}
	return s.store.CompleteInternal(ctx, *t, in, sent, received)
}

// RecoverOnce resumes transfers left unfinished by crashes or outages.
func (s *Service) RecoverOnce(ctx context.Context, staleAfter time.Duration) int {
	ids, err := s.store.Unfinished(ctx, s.now().Add(-staleAfter), 100)
	if err != nil {
		s.log.ErrorContext(ctx, "pix recovery scan", "error", err)
		return 0
	}
	for _, id := range ids {
		if _, err := s.Process(ctx, id); err != nil && !errors.Is(err, domain.ErrBusy) {
			s.log.WarnContext(ctx, "pix recovery attempt failed", "transfer_id", id, "error", err)
		}
	}
	return len(ids)
}

func (s *Service) RunRecovery(ctx context.Context, interval, staleAfter time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RecoverOnce(ctx, staleAfter)
		}
	}
}

func (s *Service) event(ctx context.Context, eventType, key string, payload any) (outbox.Message, error) {
	ev, err := outbox.NewEvent(ctx, source, eventType, payload)
	if err != nil {
		return outbox.Message{}, err
	}
	return outbox.Message{Topic: topic, Key: key, Event: ev}, nil
}

func keyPayload(k domain.Key) map[string]string {
	return map[string]string{"key_id": k.ID.String(), "account_id": k.AccountID.String(), "type": string(k.Type)}
}

func transferPayload(t domain.Transfer) map[string]any {
	return map[string]any{
		"transfer_id": t.ID.String(), "end_to_end_id": t.EndToEndID, "direction": string(t.Direction),
		"status": string(t.Status), "account_id": t.AccountID.String(),
		"counterparty_name": t.Counterparty.Name, "counterparty_bank": t.Counterparty.BankName,
		"amount_minor": t.Amount.Minor(), "currency": string(t.Amount.Currency()),
		"transaction_id": t.TransactionID.String(), "failure_reason": t.FailureReason,
		"refund_of": nullableID(t.RefundOf), "qr_txid": t.QRTxID,
	}
}

func nullableID(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}
