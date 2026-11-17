// Package application implements Pix keys and transfers.
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

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
}

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

// Send creates an outgoing Pix and drives it as far as it can go.
func (s *Service) Send(ctx context.Context, idemKey string, from uuid.UUID, key string, amount money.Money, description string) (domain.Transfer, error) {
	_, value, receiver, receiverAccount, err := s.Lookup(ctx, key)
	if err != nil {
		return domain.Transfer{}, err
	}
	t, err := domain.NewOutgoing(idemKey, from, value, receiver, receiverAccount, amount, description, s.now())
	if err != nil {
		return domain.Transfer{}, err
	}
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
	return s.Process(ctx, t.ID)
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
	return map[string]string{
		"pix_transfer_id":   t.ID.String(),
		"end_to_end_id":     t.EndToEndID,
		"pix_key":           t.Key,
		"counterparty_name": t.Counterparty.Name,
		"counterparty_bank": t.Counterparty.BankName,
	}
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
	if err := s.store.SaveTransfer(ctx, *t, ev); err != nil {
		return err
	}
	telemetry.BusinessEvents.WithLabelValues(eventType, string(t.Status)).Inc()
	return nil
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
	}
}
