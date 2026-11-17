// Package ports declares the notification service's dependencies.
package ports

import (
	"context"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/domain"
)

// Delivery is a push or e-mail to send.
type Delivery struct {
	Notification domain.Notification
	Channel      domain.Channel
	Attempts     int
}

type Store interface {
	SetOwner(ctx context.Context, account uuid.UUID, customer string) error
	Owner(ctx context.Context, account uuid.UUID) (string, bool, error)
	Preferences(ctx context.Context, customer string) (domain.Preferences, error)
	SetPreferences(ctx context.Context, customer string, p domain.Preferences) error
	// Save records the notifications of one event exactly once (inbox), with
	// their pending deliveries, and wakes the customers' live streams.
	Save(ctx context.Context, eventID string, ns []domain.Notification) error
	// List returns notifications newest first, before beforeID if set.
	List(ctx context.Context, customer string, unreadOnly bool, beforeID uuid.UUID, limit int) ([]domain.Notification, int, error)
	// After returns notifications newer than afterID, oldest first.
	After(ctx context.Context, customer string, afterID uuid.UUID, limit int) ([]domain.Notification, error)
	Latest(ctx context.Context, customer string) (uuid.UUID, error)
	MarkRead(ctx context.Context, customer string, ids []uuid.UUID) (int, error)
	PendingDeliveries(ctx context.Context, limit int) ([]Delivery, error)
	SetDelivery(ctx context.Context, notification uuid.UUID, ch domain.Channel, status string) error
}

type Accounts interface {
	// Customer returns the owner of an account.
	Customer(ctx context.Context, account uuid.UUID) (string, error)
}

// Sender delivers a push or an e-mail (simulated).
type Sender interface {
	Send(ctx context.Context, ch domain.Channel, n domain.Notification) error
}
