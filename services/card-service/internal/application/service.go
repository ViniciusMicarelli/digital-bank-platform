// Package application implements card issuing, controls and authorization.
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/secretbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/ports"
)

const (
	source = "card-service"
	topic  = "bank.card.events"
)

var (
	authLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "card_authorization_seconds", Help: "Time to answer an authorization request (target p99 < 200 ms).",
		Buckets: []float64{.005, .01, .025, .05, .1, .2, .5, 1},
	})
	authResults = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "card_authorizations_total", Help: "Authorization answers by response code.",
	}, []string{"code"})
)

// Keys protects the vault: PANs are encrypted, indexed and the CVV derived,
// each with its own key derived from one master key.
type Keys struct {
	box      *secretbox.Box
	panIndex []byte
	cvk      []byte
}

func NewKeys(master []byte) (Keys, error) {
	box, err := secretbox.New(master)
	if err != nil {
		return Keys{}, err
	}
	return Keys{box: box, panIndex: secretbox.DeriveKey(master, "card-pan-index-v1"), cvk: secretbox.DeriveKey(master, "card-cvk-v1")}, nil
}

func panAAD(card uuid.UUID) []byte { return []byte("card-pan:" + card.String()) }

type Service struct {
	store    ports.Store
	accounts ports.Accounts
	txs      ports.Transactions
	keys     Keys
	log      *slog.Logger
	now      func() time.Time
}

func New(store ports.Store, accounts ports.Accounts, txs ports.Transactions, keys Keys, log *slog.Logger) *Service {
	return &Service{store: store, accounts: accounts, txs: txs, keys: keys, log: log, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Issue creates a card for an active account. Idempotent on key.
func (s *Service) Issue(ctx context.Context, key string, account uuid.UUID, kind domain.Kind) (domain.Card, error) {
	holder, err := s.accounts.Holder(ctx, account)
	if err != nil {
		return domain.Card{}, err
	}
	if !holder.Active {
		return domain.Card{}, domain.ErrAccountNotActive
	}
	c, pan, err := domain.NewCard(key, account, kind, embossName(holder.Name), s.now())
	if err != nil {
		return domain.Card{}, err
	}
	ev, err := s.event(ctx, "card.issued", c.ID.String(), cardPayload(c))
	if err != nil {
		return domain.Card{}, err
	}
	existing, err := s.store.InsertCard(ctx, c, s.keys.box.Seal([]byte(pan), panAAD(c.ID)), secretbox.BlindIndex(s.keys.panIndex, pan), ev)
	if err != nil {
		return domain.Card{}, err
	}
	if existing != nil {
		if existing.AccountID != account || existing.Kind != kind {
			return domain.Card{}, fmt.Errorf("%w: idempotency key reused with a different request", domain.ErrInvalidArgument)
		}
		return *existing, nil
	}
	c.CreatedAt = s.now()
	return c, nil
}

// embossName is how the name is printed on the card: upper case, max 26.
func embossName(name string) string {
	if len(name) > 26 {
		name = name[:26]
	}
	out := []rune{}
	for _, r := range name {
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		out = append(out, r)
	}
	return string(out)
}

func (s *Service) Card(ctx context.Context, id uuid.UUID) (domain.Card, error) {
	return s.store.Card(ctx, id)
}

func (s *Service) Cards(ctx context.Context, account uuid.UUID) ([]domain.Card, error) {
	return s.store.CardsByAccount(ctx, account)
}

func (s *Service) Block(ctx context.Context, id uuid.UUID, reason string) (domain.Card, error) {
	return s.change(ctx, id, "card.blocked", func(c *domain.Card) error { return c.Block(reason) })
}

func (s *Service) Unblock(ctx context.Context, id uuid.UUID) (domain.Card, error) {
	return s.change(ctx, id, "card.unblocked", func(c *domain.Card) error { return c.Unblock() })
}

func (s *Service) UpdateControls(ctx context.Context, id uuid.UUID, ctl domain.Controls) (domain.Card, error) {
	return s.change(ctx, id, "card.controls.changed", func(c *domain.Card) error {
		if c.Status == domain.Cancelled {
			return fmt.Errorf("%w: card is cancelled", domain.ErrInvalidTransition)
		}
		c.Controls = ctl
		return nil
	})
}

func (s *Service) change(ctx context.Context, id uuid.UUID, eventType string, fn func(*domain.Card) error) (domain.Card, error) {
	c, err := s.store.Card(ctx, id)
	if err != nil {
		return domain.Card{}, err
	}
	if err := fn(&c); err != nil {
		return domain.Card{}, err
	}
	ev, err := s.event(ctx, eventType, c.ID.String(), cardPayload(c))
	if err != nil {
		return domain.Card{}, err
	}
	return c, s.store.SaveCard(ctx, c, ev)
}

// Details are the sensitive card data, shown only to the card holder.
type Details struct {
	PAN                     string
	ExpiryMonth, ExpiryYear int
	CVV                     string
}

func (s *Service) Reveal(ctx context.Context, id uuid.UUID) (Details, error) {
	c, err := s.store.Card(ctx, id)
	if err != nil {
		return Details{}, err
	}
	enc, err := s.store.PANEnc(ctx, id)
	if err != nil {
		return Details{}, err
	}
	pan, err := s.keys.box.Open(enc, panAAD(id))
	if err != nil {
		return Details{}, fmt.Errorf("decrypt card %s: %w", id, err)
	}
	return Details{PAN: string(pan), ExpiryMonth: c.ExpiryMonth, ExpiryYear: c.ExpiryYear,
		CVV: domain.DeriveCVV(s.keys.cvk, string(pan), c.ExpiryMonth, c.ExpiryYear)}, nil
}

// Authorize answers a network request. Every outcome is an Authorization;
// errors are only returned for malformed requests.
//
// The authorization is stored first as PENDING_REVERSAL ("cancel whatever
// was reserved"), and only becomes APPROVED once the reservation is
// confirmed. If anything fails in between, the network gets a 91 decline and
// a worker releases any reservation that did go through.
func (s *Service) Authorize(ctx context.Context, pan string, p domain.Purchase) (domain.Authorization, error) {
	start := time.Now()
	defer func() { authLatency.Observe(time.Since(start).Seconds()) }()
	if err := p.Validate(); err != nil {
		return domain.Authorization{}, err
	}
	a := domain.Authorization{
		ID: uuid.Must(uuid.NewV7()), NetworkReference: p.NetworkReference, Purchase: p,
		Status: domain.PendingReversal, Code: domain.CodeIssuerUnavailable, Reason: "processing",
	}
	card, token, cardErr := s.findCard(ctx, pan)
	if cardErr == nil {
		a.CardID, a.AccountID = card.ID, card.AccountID
		if token != nil {
			a.TokenID = token.ID
		}
	} else if !errors.Is(cardErr, domain.ErrNotFound) {
		return domain.Authorization{}, cardErr
	}

	existing, err := s.store.ClaimAuthorization(ctx, a)
	if err != nil {
		return domain.Authorization{}, err
	}
	if existing != nil {
		return *existing, nil // network retry: same answer
	}

	var d domain.Decision
	if cardErr != nil {
		d = domain.Decision{Code: domain.CodeInvalidCard, Reason: "unknown card"}
	} else if token != nil {
		d = card.DecideToken(p, *token, s.now())
	} else {
		d = card.Decide(p, domain.DeriveCVV(s.keys.cvk, pan, card.ExpiryMonth, card.ExpiryYear), s.now())
	}
	if !d.Approve {
		return s.decide(ctx, a, domain.Declined, d.Code, d.Reason)
	}

	res, err := s.txs.Reserve(ctx, a.TransactionKey(), card.AccountID, p.Amount, p.MerchantName, s.metadata(a, card))
	if err != nil {
		s.log.WarnContext(ctx, "reservation failed, declining with 91", "authorization_id", a.ID, "error", err)
		authResults.WithLabelValues(domain.CodeIssuerUnavailable).Inc()
		return a, nil // stays PENDING_REVERSAL: the worker cleans up
	}
	a.TransactionID = res.ID
	if res.Status != "RESERVED" {
		return s.decide(ctx, a, domain.Declined, domain.DeclineCodeFor(res.FailureReason), res.FailureReason)
	}
	return s.decide(ctx, a, domain.Approved, domain.CodeApproved, "")
}

// findCard resolves a card number: a card in the vault, or a token of one.
func (s *Service) findCard(ctx context.Context, pan string) (domain.Card, *domain.Token, error) {
	if !domain.ValidPAN(pan) {
		return domain.Card{}, nil, domain.ErrNotFound
	}
	index := secretbox.BlindIndex(s.keys.panIndex, pan)
	card, err := s.store.CardByPANIndex(ctx, index)
	if !errors.Is(err, domain.ErrNotFound) {
		return card, nil, err
	}
	t, err := s.store.TokenByPANIndex(ctx, index)
	if err != nil {
		return domain.Card{}, nil, err
	}
	card, err = s.store.Card(ctx, t.CardID)
	return card, &t, err
}

func tokenAAD(t uuid.UUID) []byte { return []byte("card-token:" + t.String()) }

// Tokenize provisions a token for a wallet or merchant. The token number is
// returned once (to the wallet) and kept encrypted in the vault.
func (s *Service) Tokenize(ctx context.Context, cardID uuid.UUID, wallet domain.Wallet, device string) (domain.Token, string, error) {
	c, err := s.store.Card(ctx, cardID)
	if err != nil {
		return domain.Token{}, "", err
	}
	t, pan, err := domain.NewToken(c, wallet, device)
	if err != nil {
		return domain.Token{}, "", err
	}
	ev, err := s.event(ctx, "card.token.created", t.ID.String(), map[string]string{
		"token_id": t.ID.String(), "card_id": c.ID.String(), "wallet": string(t.Wallet), "last4": t.Last4})
	if err != nil {
		return domain.Token{}, "", err
	}
	if err := s.store.InsertToken(ctx, t, s.keys.box.Seal([]byte(pan), tokenAAD(t.ID)), secretbox.BlindIndex(s.keys.panIndex, pan), ev); err != nil {
		return domain.Token{}, "", err
	}
	t.CreatedAt = s.now()
	return t, pan, nil
}

func (s *Service) Tokens(ctx context.Context, cardID uuid.UUID) ([]domain.Token, error) {
	return s.store.Tokens(ctx, cardID)
}

// UpdateToken suspends, resumes or deletes one token of a card.
func (s *Service) UpdateToken(ctx context.Context, cardID, tokenID uuid.UUID, action string) (domain.Token, error) {
	t, err := s.store.Token(ctx, tokenID)
	if err != nil || t.CardID != cardID {
		return domain.Token{}, fmt.Errorf("%w: token", domain.ErrNotFound)
	}
	if err := t.Transition(action); err != nil {
		return domain.Token{}, err
	}
	ev, err := s.event(ctx, "card.token."+strings.ToLower(string(t.Status)), t.ID.String(), map[string]string{
		"token_id": t.ID.String(), "card_id": cardID.String(), "status": string(t.Status)})
	if err != nil {
		return domain.Token{}, err
	}
	return t, s.store.SaveToken(ctx, t, ev)
}

func (s *Service) metadata(a domain.Authorization, c domain.Card) map[string]string {
	return map[string]string{
		"authorization_id": a.ID.String(), "counterparty_name": a.Purchase.MerchantName,
		"mcc": a.Purchase.MCC, "card_last4": c.Last4, "channel": string(a.Purchase.Channel),
	}
}

func (s *Service) decide(ctx context.Context, a domain.Authorization, st domain.AuthStatus, code, reason string) (domain.Authorization, error) {
	a.Status, a.Code, a.Reason = st, code, reason
	eventType := "card.authorization.approved"
	if st != domain.Approved {
		eventType = "card.authorization.declined"
	}
	ev, err := s.event(ctx, eventType, a.ID.String(), authPayload(a))
	if err != nil {
		return domain.Authorization{}, err
	}
	if err := s.store.SaveAuthorization(ctx, a, domain.PendingReversal, ev); err != nil {
		return domain.Authorization{}, err
	}
	authResults.WithLabelValues(code).Inc()
	return a, nil
}

// Clear settles an approved authorization for its amount or less.
func (s *Service) Clear(ctx context.Context, id uuid.UUID, amount *money.Money) (domain.Authorization, error) {
	a, err := s.store.Authorization(ctx, id)
	if err != nil {
		return domain.Authorization{}, err
	}
	switch a.Status {
	case domain.Cleared:
		return a, nil
	case domain.Approved:
	default:
		return domain.Authorization{}, fmt.Errorf("%w: cannot clear a %s authorization", domain.ErrInvalidState, a.Status)
	}
	if amount != nil && (amount.Currency() != money.BRL || !amount.IsPositive() || amount.Minor() > a.Purchase.Amount.Minor()) {
		return domain.Authorization{}, fmt.Errorf("%w: cleared amount must be positive and at most %s", domain.ErrInvalidArgument, a.Purchase.Amount.Decimal())
	}
	if _, err := s.txs.Complete(ctx, a.TransactionID, amount); err != nil {
		return domain.Authorization{}, err
	}
	a.Status, a.Cleared = domain.Cleared, a.Purchase.Amount.Minor()
	if amount != nil {
		a.Cleared = amount.Minor()
	}
	ev, err := s.event(ctx, "card.purchase.cleared", a.ID.String(), authPayload(a))
	if err != nil {
		return domain.Authorization{}, err
	}
	return a, s.store.SaveAuthorization(ctx, a, domain.Approved, ev)
}

// Reverse cancels an approved authorization and releases the money.
func (s *Service) Reverse(ctx context.Context, id uuid.UUID) (domain.Authorization, error) {
	a, err := s.store.Authorization(ctx, id)
	if err != nil {
		return domain.Authorization{}, err
	}
	switch a.Status {
	case domain.Reversed:
		return a, nil
	case domain.Approved:
	default:
		return domain.Authorization{}, fmt.Errorf("%w: cannot reverse a %s authorization", domain.ErrInvalidState, a.Status)
	}
	if _, err := s.txs.Compensate(ctx, a.TransactionID, "MERCHANT_REVERSAL"); err != nil {
		return domain.Authorization{}, err
	}
	a.Status = domain.Reversed
	ev, err := s.event(ctx, "card.authorization.reversed", a.ID.String(), authPayload(a))
	if err != nil {
		return domain.Authorization{}, err
	}
	return a, s.store.SaveAuthorization(ctx, a, domain.Approved, ev)
}

// RecoverOnce settles authorizations left PENDING_REVERSAL: it replays the
// reservation (idempotent, so it returns whatever exists or creates one) and
// releases it, then records the 91 decline the network already received.
func (s *Service) RecoverOnce(ctx context.Context, olderThan time.Duration) int {
	ids, err := s.store.PendingReversals(ctx, s.now().Add(-olderThan), 100)
	if err != nil {
		s.log.ErrorContext(ctx, "pending reversals scan", "error", err)
		return 0
	}
	done := 0
	for _, id := range ids {
		if err := s.reverseOrphan(ctx, id); err != nil {
			s.log.WarnContext(ctx, "orphan reservation cleanup failed", "authorization_id", id, "error", err)
			continue
		}
		done++
	}
	return done
}

func (s *Service) reverseOrphan(ctx context.Context, id uuid.UUID) error {
	a, err := s.store.Authorization(ctx, id)
	if err != nil {
		return err
	}
	if a.CardID != uuid.Nil {
		card, err := s.store.Card(ctx, a.CardID)
		if err != nil {
			return err
		}
		res, err := s.txs.Reserve(ctx, a.TransactionKey(), card.AccountID, a.Purchase.Amount, a.Purchase.MerchantName, s.metadata(a, card))
		if err != nil {
			return err
		}
		a.TransactionID = res.ID
		if res.Status == "RESERVED" {
			if _, err := s.txs.Compensate(ctx, res.ID, "ISSUER_TIMEOUT"); err != nil {
				return err
			}
		}
	}
	_, err = s.decide(ctx, a, domain.Declined, domain.CodeIssuerUnavailable, "issuer unavailable")
	return err
}

func (s *Service) RunRecovery(ctx context.Context, interval, olderThan time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RecoverOnce(ctx, olderThan)
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

// Events never carry the card number: id and last4 only.
func cardPayload(c domain.Card) map[string]any {
	return map[string]any{"card_id": c.ID.String(), "account_id": c.AccountID.String(), "kind": string(c.Kind),
		"status": string(c.Status), "reason": c.StatusReason, "last4": c.Last4}
}

func authPayload(a domain.Authorization) map[string]any {
	return map[string]any{
		"authorization_id": a.ID.String(), "card_id": a.CardID.String(), "status": string(a.Status),
		"response_code": a.Code, "reason": a.Reason, "amount_minor": a.Purchase.Amount.Minor(),
		"cleared_minor": a.Cleared, "merchant_name": a.Purchase.MerchantName, "mcc": a.Purchase.MCC,
		"country": a.Purchase.Country, "channel": string(a.Purchase.Channel),
	}
}
