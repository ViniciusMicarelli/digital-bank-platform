// Package application implements the ledger use cases on top of the ports.
package application

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/ports"
)

const (
	source = "ledger-service"
	topic  = "bank.ledger.events"
)

// Service is the ledger application service.
type Service struct {
	store ports.Store
	log   *slog.Logger
}

func New(store ports.Store, log *slog.Logger) *Service {
	return &Service{store: store, log: log}
}

// CreateAccount creates a ledger account. Retrying with the same id and
// attributes returns the existing account.
func (s *Service) CreateAccount(ctx context.Context, a domain.Account) (domain.Account, error) {
	a, err := domain.NewAccount(a.ID, a.Kind, a.Currency, a.OverdraftLimit, a.Code)
	if err != nil {
		return domain.Account{}, err
	}
	var stored domain.Account
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		var created bool
		var err error
		stored, created, err = tx.InsertAccount(ctx, a)
		if err != nil {
			return err
		}
		if !created && !stored.SameAttributes(a) {
			return fmt.Errorf("%w: %s", domain.ErrAccountConflict, a.ID)
		}
		return nil
	})
	return stored, err
}

func (s *Service) Account(ctx context.Context, id uuid.UUID) (domain.Account, error) {
	return s.store.Account(ctx, id)
}

// PostEntry validates and posts a balanced entry. It is idempotent: the same
// key with the same request returns the original entry; with a different
// request it fails with ErrIdempotencyConflict.
func (s *Service) PostEntry(ctx context.Context, key string, postings []domain.Posting, description string, metadata map[string]string) (domain.Entry, error) {
	e, err := domain.NewEntry(key, postings, description, metadata)
	if err != nil {
		return domain.Entry{}, err
	}
	var result domain.Entry
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		var err error
		result, err = s.post(ctx, tx, e, true)
		return err
	})
	return result, err
}

// post claims the idempotency key, applies the entry to locked balances and
// records the postings and the event. Must run inside a transaction.
func (s *Service) post(ctx context.Context, tx ports.Tx, e domain.Entry, enforceFloor bool) (domain.Entry, error) {
	existing, fp, err := tx.ClaimEntry(ctx, e)
	if err != nil {
		return domain.Entry{}, err
	}
	if existing != nil {
		if !bytes.Equal(fp, e.Fingerprint()) {
			return domain.Entry{}, fmt.Errorf("%w: %q", domain.ErrIdempotencyConflict, e.IdempotencyKey)
		}
		return *existing, nil
	}

	ids := e.AccountIDs()
	accounts, err := tx.Accounts(ctx, ids)
	if err != nil {
		return domain.Entry{}, err
	}
	balances, err := tx.LockBalances(ctx, ids)
	if err != nil {
		return domain.Entry{}, err
	}
	if err := e.Apply(accounts, balances, enforceFloor); err != nil {
		return domain.Entry{}, err
	}
	if err := tx.InsertPostings(ctx, e); err != nil {
		return domain.Entry{}, err
	}
	if err := tx.SaveBalances(ctx, balances); err != nil {
		return domain.Entry{}, err
	}
	e.CreatedAt = time.Now().UTC()
	if err := s.publish(ctx, tx, "ledger.entry.posted", e.ID.String(), entryPayload(e)); err != nil {
		return domain.Entry{}, err
	}
	return e, nil
}

func (s *Service) Entry(ctx context.Context, id uuid.UUID) (domain.Entry, error) {
	return s.store.Entry(ctx, id)
}

// ReverseEntry posts the opposite of an entry. Each entry can be reversed once;
// retrying with the same key returns the reversal.
func (s *Service) ReverseEntry(ctx context.Context, key string, entryID uuid.UUID, description string) (domain.Entry, error) {
	var result domain.Entry
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		original, err := tx.EntryForUpdate(ctx, entryID)
		if err != nil {
			return err
		}
		rev, err := original.Reversal(key, description)
		if err != nil {
			return err
		}
		if prior, err := tx.ReversalOf(ctx, entryID); err != nil {
			return err
		} else if prior != nil {
			if prior.IdempotencyKey == key {
				result = *prior
				return nil
			}
			return fmt.Errorf("%w: %s", domain.ErrAlreadyReversed, entryID)
		}
		result, err = s.post(ctx, tx, rev, true)
		return err
	})
	return result, err
}

// Hold reserves funds on an account. Idempotent on the key.
func (s *Service) Hold(ctx context.Context, key string, account, counter uuid.UUID, amount money.Money, description string) (domain.Hold, error) {
	h, err := domain.NewHold(key, account, counter, amount, description)
	if err != nil {
		return domain.Hold{}, err
	}
	var result domain.Hold
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		existing, fp, err := tx.ClaimHold(ctx, h)
		if err != nil {
			return err
		}
		if existing != nil {
			if !bytes.Equal(fp, h.Fingerprint()) {
				return fmt.Errorf("%w: %q", domain.ErrIdempotencyConflict, key)
			}
			result = *existing
			return nil
		}
		accounts, err := tx.Accounts(ctx, []uuid.UUID{account, counter})
		if err != nil {
			return err
		}
		if accounts[counter].Currency != amount.Currency() {
			return fmt.Errorf("%w: counter account currency differs", domain.ErrInvalidArgument)
		}
		balances, err := tx.LockBalances(ctx, []uuid.UUID{account})
		if err != nil {
			return err
		}
		if err := h.Reserve(accounts[account], balances[account]); err != nil {
			return err
		}
		if err := tx.SaveBalances(ctx, balances); err != nil {
			return err
		}
		h.CreatedAt = time.Now().UTC()
		result = h
		return s.publish(ctx, tx, "ledger.hold.created", h.ID.String(), holdPayload(h))
	})
	return result, err
}

// Capture posts a hold (fully, or partially when amount is set). Capturing an
// already captured hold returns it unchanged, so retries are safe.
func (s *Service) Capture(ctx context.Context, holdID uuid.UUID, amount *money.Money) (domain.Hold, domain.Entry, error) {
	var (
		hold  domain.Hold
		entry domain.Entry
	)
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		var err error
		hold, err = tx.HoldForUpdate(ctx, holdID)
		if err != nil {
			return err
		}
		if hold.Status == domain.HoldCaptured {
			entry, err = tx.EntryForUpdate(ctx, hold.CaptureEntryID)
			return err
		}
		capture := hold.Amount
		if amount != nil {
			capture = *amount
		}
		balances, err := tx.LockBalances(ctx, []uuid.UUID{hold.AccountID, hold.CounterAccountID})
		if err != nil {
			return err
		}
		e, err := hold.Capture(capture, "hold:"+hold.ID.String()+":capture", balances[hold.AccountID])
		if err != nil {
			return err
		}
		if err := tx.SaveBalances(ctx, balances); err != nil {
			return err
		}
		// Funds were reserved by the hold, so the floor is not checked again.
		if entry, err = s.post(ctx, tx, e, false); err != nil {
			return err
		}
		if err := tx.UpdateHold(ctx, hold); err != nil {
			return err
		}
		return s.publish(ctx, tx, "ledger.hold.captured", hold.ID.String(), holdPayload(hold))
	})
	return hold, entry, err
}

// Release returns held funds. Releasing a released hold is a no-op.
func (s *Service) Release(ctx context.Context, holdID uuid.UUID) (domain.Hold, error) {
	var hold domain.Hold
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		var err error
		hold, err = tx.HoldForUpdate(ctx, holdID)
		if err != nil {
			return err
		}
		if hold.Status == domain.HoldReleased {
			return nil
		}
		balances, err := tx.LockBalances(ctx, []uuid.UUID{hold.AccountID})
		if err != nil {
			return err
		}
		if err := hold.Release(balances[hold.AccountID]); err != nil {
			return err
		}
		if err := tx.SaveBalances(ctx, balances); err != nil {
			return err
		}
		if err := tx.UpdateHold(ctx, hold); err != nil {
			return err
		}
		return s.publish(ctx, tx, "ledger.hold.released", hold.ID.String(), holdPayload(hold))
	})
	return hold, err
}

// Balance returns the balance with the account's currency.
func (s *Service) Balance(ctx context.Context, accountID uuid.UUID) (domain.Account, domain.Balance, error) {
	acc, err := s.store.Account(ctx, accountID)
	if err != nil {
		return domain.Account{}, domain.Balance{}, err
	}
	b, err := s.store.Balance(ctx, accountID)
	return acc, b, err
}

// BalanceAt rebuilds the posted balance at a point in time from postings.
func (s *Service) BalanceAt(ctx context.Context, accountID uuid.UUID, at time.Time) (domain.Account, int64, error) {
	acc, err := s.store.Account(ctx, accountID)
	if err != nil {
		return domain.Account{}, 0, err
	}
	posted, err := s.store.PostedAt(ctx, accountID, at)
	return acc, posted, err
}

// Postings lists an account's postings, newest first.
func (s *Service) Postings(ctx context.Context, accountID uuid.UUID, before int64, limit int) ([]ports.AccountPosting, error) {
	if _, err := s.store.Account(ctx, accountID); err != nil {
		return nil, err
	}
	return s.store.Postings(ctx, accountID, before, limit)
}

func (s *Service) publish(ctx context.Context, tx ports.Tx, eventType, key string, payload any) error {
	ev, err := outbox.NewEvent(ctx, source, eventType, payload)
	if err != nil {
		return err
	}
	return tx.Publish(ctx, outbox.Message{Topic: topic, Key: key, Event: ev})
}
