package application

import (
	"time"

	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/domain"
)

// Event payloads published on bank.ledger.events. Amounts are minor units.

type PostingPayload struct {
	AccountID   string `json:"account_id"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

type EntryPostedPayload struct {
	EntryID         string            `json:"entry_id"`
	Postings        []PostingPayload  `json:"postings"`
	Description     string            `json:"description"`
	Metadata        map[string]string `json:"metadata"`
	ReversesEntryID string            `json:"reverses_entry_id,omitempty"`
	PostedAt        time.Time         `json:"posted_at"`
}

type HoldPayload struct {
	HoldID           string `json:"hold_id"`
	AccountID        string `json:"account_id"`
	CounterAccountID string `json:"counter_account_id"`
	AmountMinor      int64  `json:"amount_minor"`
	Currency         string `json:"currency"`
	Status           string `json:"status"`
	CaptureEntryID   string `json:"capture_entry_id,omitempty"`
}

func entryPayload(e domain.Entry) EntryPostedPayload {
	p := EntryPostedPayload{
		EntryID:     e.ID.String(),
		Description: e.Description,
		Metadata:    e.Metadata,
		PostedAt:    e.CreatedAt,
	}
	if e.ReversesEntryID != [16]byte{} {
		p.ReversesEntryID = e.ReversesEntryID.String()
	}
	for _, ps := range e.Postings {
		p.Postings = append(p.Postings, PostingPayload{
			AccountID:   ps.AccountID.String(),
			AmountMinor: ps.Amount.Minor(),
			Currency:    string(ps.Amount.Currency()),
		})
	}
	return p
}

func holdPayload(h domain.Hold) HoldPayload {
	p := HoldPayload{
		HoldID:           h.ID.String(),
		AccountID:        h.AccountID.String(),
		CounterAccountID: h.CounterAccountID.String(),
		AmountMinor:      h.Amount.Minor(),
		Currency:         string(h.Amount.Currency()),
		Status:           string(h.Status),
	}
	if h.CaptureEntryID != [16]byte{} {
		p.CaptureEntryID = h.CaptureEntryID.String()
	}
	return p
}
