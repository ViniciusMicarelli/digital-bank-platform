// Package ports declares the user service's dependencies.
package ports

import (
	"context"
	"time"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/domain"
)

type Store interface {
	Profile(ctx context.Context, customer string) (domain.Profile, error)
	// Save upserts the profile; a CPF already verified or in verification
	// for another customer is refused (DOCUMENT_IN_USE).
	Save(ctx context.Context, p domain.Profile, events ...outbox.Message) error
	// Pending lists customers whose verification waits for the bureau.
	Pending(ctx context.Context, before time.Time, limit int) ([]string, error)
	// InReview lists the customers waiting for an analyst, oldest first.
	InReview(ctx context.Context, limit int) ([]string, error)
}

// Bureau checks a person against credit bureaus / government records.
type Bureau interface {
	Check(ctx context.Context, p domain.Profile) (domain.BureauDecision, string, error)
}
