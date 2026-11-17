// Package application turns events into notifications, delivers them (in
// the app, by push, by e-mail) and streams them live to connected clients.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/ports"
)

type Service struct {
	store    ports.Store
	accounts ports.Accounts
	sender   ports.Sender
	log      *slog.Logger

	mu   sync.Mutex
	subs map[string]map[chan struct{}]struct{}
}

func New(store ports.Store, accounts ports.Accounts, sender ports.Sender, log *slog.Logger) *Service {
	return &Service{store: store, accounts: accounts, sender: sender, log: log, subs: map[string]map[chan struct{}]struct{}{}}
}

// HandleEvent is the Kafka handler.
func (s *Service) HandleEvent(ctx context.Context, ev outbox.Event) error {
	if ev.Type == "account.opened" {
		var p struct {
			AccountID  uuid.UUID `json:"account_id"`
			CustomerID string    `json:"customer_id"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return err
		}
		return s.store.SetOwner(ctx, p.AccountID, p.CustomerID)
	}
	drafts, err := domain.FromEvent(domain.Event{ID: ev.ID, Type: ev.Type, OccurredAt: ev.OccurredAt, Payload: ev.Payload}, time.Now())
	if err != nil || len(drafts) == 0 {
		return err
	}
	ns := make([]domain.Notification, 0, len(drafts))
	for _, d := range drafts {
		customer, err := s.customer(ctx, d.AccountID)
		if errors.Is(err, domain.ErrNotFound) {
			s.log.WarnContext(ctx, "notification for an unknown account dropped", "event", ev.Type, "account_id", d.AccountID)
			continue
		}
		if err != nil {
			return err
		}
		prefs, err := s.store.Preferences(ctx, customer)
		if err != nil {
			return err
		}
		if d.OccurredAt.IsZero() {
			d.OccurredAt = time.Now()
		}
		ns = append(ns, domain.Notification{ID: uuid.Must(uuid.NewV7()), CustomerID: customer, Draft: d, Channels: prefs.Channels(d)})
	}
	return s.store.Save(ctx, ev.ID, ns)
}

// customer finds who owns an account: from account.opened events, or by
// asking account-service when another topic's event arrived first.
func (s *Service) customer(ctx context.Context, account uuid.UUID) (string, error) {
	c, ok, err := s.store.Owner(ctx, account)
	if err != nil || ok {
		return c, err
	}
	if c, err = s.accounts.Customer(ctx, account); err != nil {
		return "", err
	}
	return c, s.store.SetOwner(ctx, account, c)
}

func (s *Service) List(ctx context.Context, customer string, unreadOnly bool, before uuid.UUID, limit int) ([]domain.Notification, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.store.List(ctx, customer, unreadOnly, before, limit)
}

// MarkRead marks ids (all unread when empty) as read.
func (s *Service) MarkRead(ctx context.Context, customer string, ids []uuid.UUID) (int, error) {
	return s.store.MarkRead(ctx, customer, ids)
}

func (s *Service) Preferences(ctx context.Context, customer string) (domain.Preferences, error) {
	return s.store.Preferences(ctx, customer)
}

func (s *Service) SetPreferences(ctx context.Context, customer string, p domain.Preferences) error {
	return s.store.SetPreferences(ctx, customer, p)
}

// ---- live stream ----

// Wake signals the subscribers of a customer (called by the listener).
func (s *Service) Wake(customer string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs[customer] {
		select {
		case ch <- struct{}{}:
		default: // already signalled
		}
	}
}

func (s *Service) subscribe(customer string) (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	if s.subs[customer] == nil {
		s.subs[customer] = map[chan struct{}]struct{}{}
	}
	s.subs[customer][ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs[customer], ch)
		if len(s.subs[customer]) == 0 {
			delete(s.subs, customer)
		}
		s.mu.Unlock()
	}
}

// Stream sends the customer's new notifications, after afterID (or from now
// when Nil), until ctx ends. It re-reads on every wake-up and every poll
// interval, so a lost NOTIFY only delays a notification.
func (s *Service) Stream(ctx context.Context, customer string, afterID uuid.UUID, poll time.Duration, send func(domain.Notification) error) error {
	wake, unsubscribe := s.subscribe(customer)
	defer unsubscribe()
	if afterID == uuid.Nil {
		latest, err := s.store.Latest(ctx, customer)
		if err != nil {
			return err
		}
		afterID = latest
	}
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		list, err := s.store.After(ctx, customer, afterID, 100)
		if err != nil {
			return err
		}
		for _, n := range list {
			if err := send(n); err != nil {
				return err
			}
			afterID = n.ID
		}
		select {
		case <-ctx.Done():
			return nil
		case <-wake:
		case <-t.C:
		}
	}
}

// ---- deliveries ----

// DeliverOnce sends pending pushes and e-mails; failures are retried, up
// to 5 attempts.
func (s *Service) DeliverOnce(ctx context.Context) int {
	pending, err := s.store.PendingDeliveries(ctx, 100)
	if err != nil {
		s.log.ErrorContext(ctx, "pending deliveries", "error", err)
		return 0
	}
	sent := 0
	for _, d := range pending {
		status := "SENT"
		if err := s.sender.Send(ctx, d.Channel, d.Notification); err != nil {
			s.log.WarnContext(ctx, "delivery failed", "notification_id", d.Notification.ID, "channel", d.Channel, "error", err)
			status = "PENDING"
			if d.Attempts+1 >= 5 {
				status = "FAILED"
			}
		} else {
			sent++
		}
		if err := s.store.SetDelivery(ctx, d.Notification.ID, d.Channel, status); err != nil {
			s.log.ErrorContext(ctx, "record delivery", "error", err)
		}
	}
	return sent
}

func (s *Service) RunDeliveries(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.DeliverOnce(ctx)
		}
	}
}

// LogSender simulates push and e-mail providers by logging.
type LogSender struct{ Log *slog.Logger }

func (l LogSender) Send(ctx context.Context, ch domain.Channel, n domain.Notification) error {
	l.Log.InfoContext(ctx, "notification delivered (simulated)", "channel", ch, "customer_id", n.CustomerID, "title", n.Title)
	return nil
}
