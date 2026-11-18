// Package application implements the account use cases.
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	doc "github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/document"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/ports"
)

const (
	source = "account-service"
	topic  = "bank.account.events"
)

type Service struct {
	store  ports.Store
	ledger ports.Ledger
	users  ports.Users
	now    func() time.Time
}

// RequireKYC makes accounts open only for customers user-service verified,
// with the verified name and CPF.
func (s *Service) RequireKYC(users ports.Users) { s.users = users }

// New builds the service. now may be nil (defaults to time.Now).
func New(store ports.Store, ledger ports.Ledger, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, ledger: ledger, now: now}
}

// OpenAccount creates the ledger account first (idempotent, deterministic id),
// then the account record. A crash between the two steps is fixed by retrying
// with the same key; an orphan ledger account would only hold a zero balance.
func (s *Service) OpenAccount(ctx context.Context, key, customerID, holderName, document string, currency money.Currency) (domain.Account, error) {
	if s.users != nil {
		id, err := s.users.Identity(ctx, customerID)
		if err != nil {
			return domain.Account{}, err
		}
		if id.KYCStatus != "APPROVED" {
			return domain.Account{}, fmt.Errorf("%w (verification is %s)", domain.ErrKYCRequired, id.KYCStatus)
		}
		if document != "" {
			if sent, err := doc.NormalizeCPF(document); err != nil || sent != id.CPF {
				return domain.Account{}, domain.ErrDocumentMismatch
			}
		}
		holderName, document = id.FullName, id.CPF
	}
	a, err := domain.NewAccount(key, customerID, holderName, document, currency)
	if err != nil {
		return domain.Account{}, err
	}
	if err := s.ledger.CreateCustomerAccount(ctx, a.ID, a.Currency); err != nil {
		return domain.Account{}, fmt.Errorf("create ledger account: %w", err)
	}
	var stored domain.Account
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		var created bool
		var err error
		stored, created, err = tx.InsertAccount(ctx, a, domain.DefaultLimits)
		if err != nil {
			return err
		}
		if !created {
			if !stored.SameRequest(a) {
				return fmt.Errorf("%w: %q", domain.ErrIdempotencyConflict, key)
			}
			return nil
		}
		return s.publish(ctx, tx, "account.opened", stored)
	})
	return stored, err
}

func (s *Service) Account(ctx context.Context, id uuid.UUID) (domain.Account, error) {
	return s.store.Account(ctx, id)
}

func (s *Service) AccountsByCustomer(ctx context.Context, customerID string) ([]domain.Account, error) {
	if customerID == "" {
		return nil, fmt.Errorf("%w: customer id is required", domain.ErrInvalidArgument)
	}
	return s.store.AccountsByCustomer(ctx, customerID)
}

func (s *Service) Balance(ctx context.Context, id uuid.UUID) (ports.Balance, error) {
	if _, err := s.store.Account(ctx, id); err != nil {
		return ports.Balance{}, err
	}
	return s.ledger.Balance(ctx, id)
}

func (s *Service) Block(ctx context.Context, id uuid.UUID, reason string) (domain.Account, error) {
	return s.transition(ctx, id, "account.blocked", func(a *domain.Account) error { return a.Block(reason) })
}

func (s *Service) Unblock(ctx context.Context, id uuid.UUID) (domain.Account, error) {
	return s.transition(ctx, id, "account.unblocked", func(a *domain.Account) error { return a.Unblock() })
}

// Close requires a zero ledger balance with nothing held. The account row is
// locked while the balance is checked, so no limit can be consumed meanwhile.
func (s *Service) Close(ctx context.Context, id uuid.UUID, reason string) (domain.Account, error) {
	return s.transition(ctx, id, "account.closed", func(a *domain.Account) error {
		if a.Status != domain.StatusActive {
			return a.Close(reason, s.now())
		}
		b, err := s.ledger.Balance(ctx, id)
		if err != nil {
			return fmt.Errorf("read ledger balance: %w", err)
		}
		if !b.Posted.IsZero() || !b.Pending.IsZero() {
			return fmt.Errorf("%w: posted %s, pending %s", domain.ErrBalanceNotZero, b.Posted.Decimal(), b.Pending.Decimal())
		}
		return a.Close(reason, s.now())
	})
}

func (s *Service) transition(ctx context.Context, id uuid.UUID, event string, change func(*domain.Account) error) (domain.Account, error) {
	var a domain.Account
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		var err error
		if a, err = tx.AccountForUpdate(ctx, id); err != nil {
			return err
		}
		if err := change(&a); err != nil {
			return err
		}
		if err := tx.UpdateAccount(ctx, a); err != nil {
			return err
		}
		return s.publish(ctx, tx, event, a)
	})
	return a, err
}

// Limits returns the configured limits and the current usage.
func (s *Service) Limits(ctx context.Context, id uuid.UUID) (domain.Limits, domain.Usage, error) {
	l, err := s.store.Limits(ctx, id)
	if err != nil {
		return domain.Limits{}, domain.Usage{}, err
	}
	day, night := domain.Window(s.now())
	u, err := s.store.Usage(ctx, id, day, night)
	return l, u, err
}

func (s *Service) UpdateLimits(ctx context.Context, id uuid.UUID, l domain.Limits) (domain.Limits, error) {
	if err := l.Validate(); err != nil {
		return domain.Limits{}, err
	}
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		a, err := tx.AccountForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if a.Status == domain.StatusClosed {
			return fmt.Errorf("%w: account is closed", domain.ErrNotActive)
		}
		if err := tx.SaveLimits(ctx, id, l); err != nil {
			return err
		}
		return s.publishPayload(ctx, tx, "account.limits.changed", id.String(), map[string]any{
			"account_id": id.String(), "per_transaction_minor": l.PerTransaction,
			"daily_minor": l.Daily, "nightly_minor": l.Nightly,
		})
	})
	return l, err
}

// ConsumeLimit checks amount against the limits and records the usage, all
// under a row lock so concurrent transactions cannot both squeeze through.
// Calling it again with the same transaction id returns without counting twice.
func (s *Service) ConsumeLimit(ctx context.Context, id uuid.UUID, txID string, amount money.Money) (domain.Usage, error) {
	if txID == "" {
		return domain.Usage{}, fmt.Errorf("%w: transaction id is required", domain.ErrInvalidArgument)
	}
	var usage domain.Usage
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		a, err := tx.AccountForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if a.Status != domain.StatusActive {
			return fmt.Errorf("%w: %s", domain.ErrNotActive, a.Status)
		}
		if amount.Currency() != a.Currency {
			return fmt.Errorf("%w: account currency is %s", domain.ErrInvalidArgument, a.Currency)
		}
		limits, err := tx.LimitsForUpdate(ctx, id)
		if err != nil {
			return err
		}
		day, night := domain.Window(s.now())
		if usage, err = tx.Usage(ctx, id, day, night); err != nil {
			return err
		}
		recorded, err := tx.RecordUsage(ctx, id, txID, amount.Minor(), day, night)
		if err != nil || !recorded {
			return err // already counted: idempotent retry
		}
		if err := limits.Check(amount.Minor(), usage, night != nil); err != nil {
			return err // rolls back the usage row
		}
		usage.Daily += amount.Minor()
		if night != nil {
			usage.Nightly += amount.Minor()
		}
		return nil
	})
	return usage, err
}

// RestoreLimit gives back the usage of a transaction (compensation). It is
// idempotent and a no-op for unknown transaction ids.
func (s *Service) RestoreLimit(ctx context.Context, id uuid.UUID, txID string) error {
	return s.store.InTx(ctx, func(tx ports.Tx) error {
		return tx.RestoreUsage(ctx, id, txID)
	})
}

type accountPayload struct {
	AccountID  string `json:"account_id"`
	CustomerID string `json:"customer_id"`
	HolderName string `json:"holder_name"`
	Branch     string `json:"branch"`
	Number     string `json:"number"`
	Currency   string `json:"currency"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
}

func (s *Service) publish(ctx context.Context, tx ports.Tx, eventType string, a domain.Account) error {
	return s.publishPayload(ctx, tx, eventType, a.ID.String(), accountPayload{
		AccountID: a.ID.String(), CustomerID: a.CustomerID, HolderName: a.HolderName,
		Branch: a.Branch, Number: a.Number, Currency: string(a.Currency),
		Status: string(a.Status), Reason: a.StatusReason,
	})
}

func (s *Service) publishPayload(ctx context.Context, tx ports.Tx, eventType, key string, payload any) error {
	ev, err := outbox.NewEvent(ctx, source, eventType, payload)
	if err != nil {
		return err
	}
	return tx.Publish(ctx, outbox.Message{Topic: topic, Key: key, Event: ev})
}
