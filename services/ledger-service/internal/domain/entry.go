package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

// MaxPostings bounds the size of an entry.
const MaxPostings = 50

// Posting is a signed amount applied to one account. Positive credits the
// account (increases its balance); negative debits it.
type Posting struct {
	AccountID uuid.UUID
	Amount    money.Money
}

// Entry is an immutable journal entry. Its postings always sum to zero.
type Entry struct {
	ID              uuid.UUID
	IdempotencyKey  string
	Postings        []Posting
	Description     string
	Metadata        map[string]string
	ReversesEntryID uuid.UUID // uuid.Nil unless this is a reversal
	CreatedAt       time.Time
}

// NewEntry validates and builds an entry:
//   - an idempotency key is required;
//   - at least two postings, each on a distinct account, none zero;
//   - all postings share one currency and sum to exactly zero.
func NewEntry(key string, postings []Posting, description string, metadata map[string]string) (Entry, error) {
	if key == "" {
		return Entry{}, fmt.Errorf("%w: idempotency key is required", ErrInvalidArgument)
	}
	if len(postings) < 2 || len(postings) > MaxPostings {
		return Entry{}, fmt.Errorf("%w: an entry needs between 2 and %d postings", ErrInvalidArgument, MaxPostings)
	}
	currency := postings[0].Amount.Currency()
	seen := make(map[uuid.UUID]bool, len(postings))
	for _, p := range postings {
		if p.AccountID == uuid.Nil {
			return Entry{}, fmt.Errorf("%w: posting account id is required", ErrInvalidArgument)
		}
		if seen[p.AccountID] {
			return Entry{}, fmt.Errorf("%w: account %s appears twice", ErrInvalidArgument, p.AccountID)
		}
		seen[p.AccountID] = true
		if p.Amount.IsZero() {
			return Entry{}, fmt.Errorf("%w: zero posting on account %s", ErrInvalidArgument, p.AccountID)
		}
		if p.Amount.Currency() != currency {
			return Entry{}, fmt.Errorf("%w: entry mixes currencies", ErrInvalidArgument)
		}
	}
	amounts := make([]money.Money, len(postings))
	for i, p := range postings {
		amounts[i] = p.Amount
	}
	total, err := money.Sum(currency, amounts...)
	if err != nil {
		return Entry{}, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	if !total.IsZero() {
		return Entry{}, fmt.Errorf("%w: postings sum to %s, must be zero", ErrInvalidArgument, total.Decimal())
	}
	if metadata == nil {
		metadata = map[string]string{}
	}
	return Entry{
		ID:             uuid.Must(uuid.NewV7()),
		IdempotencyKey: key,
		Postings:       slices.Clone(postings),
		Description:    description,
		Metadata:       maps.Clone(metadata),
	}, nil
}

// Reversal builds the entry that cancels e (every posting negated).
func (e Entry) Reversal(key, description string) (Entry, error) {
	if e.ReversesEntryID != uuid.Nil {
		return Entry{}, fmt.Errorf("%w: a reversal cannot be reversed", ErrInvalidArgument)
	}
	postings := make([]Posting, len(e.Postings))
	for i, p := range e.Postings {
		neg, err := p.Amount.Neg()
		if err != nil {
			return Entry{}, err
		}
		postings[i] = Posting{AccountID: p.AccountID, Amount: neg}
	}
	if description == "" {
		description = "Reversal of " + e.ID.String()
	}
	r, err := NewEntry(key, postings, description, map[string]string{"reverses": e.ID.String()})
	if err != nil {
		return Entry{}, err
	}
	r.ReversesEntryID = e.ID
	return r, nil
}

// Currency of the entry.
func (e Entry) Currency() money.Currency { return e.Postings[0].Amount.Currency() }

// AccountIDs returns the distinct accounts touched by the entry.
func (e Entry) AccountIDs() []uuid.UUID {
	ids := make([]uuid.UUID, len(e.Postings))
	for i, p := range e.Postings {
		ids[i] = p.AccountID
	}
	return ids
}

// Fingerprint hashes what makes two requests "the same": postings (order
// independent), description and metadata. Used for idempotency checks.
func (e Entry) Fingerprint() []byte {
	h := sha256.New()
	postings := slices.Clone(e.Postings)
	slices.SortFunc(postings, func(a, b Posting) int { return slices.Compare(a.AccountID[:], b.AccountID[:]) })
	for _, p := range postings {
		h.Write(p.AccountID[:])
		_ = binary.Write(h, binary.BigEndian, p.Amount.Minor())
		h.Write([]byte(p.Amount.Currency()))
	}
	h.Write([]byte{0})
	h.Write([]byte(e.Description))
	for _, k := range slices.Sorted(maps.Keys(e.Metadata)) {
		h.Write([]byte{0})
		h.Write([]byte(k))
		h.Write([]byte{1})
		h.Write([]byte(e.Metadata[k]))
	}
	return h.Sum(nil)
}

// Apply validates e against the accounts and updates the balances in place.
// When enforceFloor is true, customer accounts must stay above their floor.
// Captures pass false: the money was already reserved by the hold.
func (e Entry) Apply(accounts map[uuid.UUID]Account, balances map[uuid.UUID]*Balance, enforceFloor bool) error {
	for _, p := range e.Postings {
		acc, ok := accounts[p.AccountID]
		if !ok {
			return fmt.Errorf("%w: account %s", ErrNotFound, p.AccountID)
		}
		if acc.Currency != p.Amount.Currency() {
			return fmt.Errorf("%w: account %s is %s, posting is %s",
				ErrInvalidArgument, acc.ID, acc.Currency, p.Amount.Currency())
		}
		b := balances[p.AccountID]
		next, err := addInt64(b.Posted, p.Amount.Minor())
		if err != nil {
			return err
		}
		b.Posted = next
		// Only debits can break the floor; credits always make things better.
		if enforceFloor && p.Amount.IsNegative() {
			if err := checkFloor(acc, *b); err != nil {
				return err
			}
		}
	}
	return nil
}

func addInt64(a, b int64) (int64, error) {
	s := a + b
	if (a > 0 && b > 0 && s < 0) || (a < 0 && b < 0 && s >= 0) {
		return 0, fmt.Errorf("%w: balance overflow", ErrInvalidArgument)
	}
	return s, nil
}
