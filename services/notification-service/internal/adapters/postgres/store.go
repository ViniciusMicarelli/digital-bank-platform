// Package postgres stores notifications and wakes live streams with
// LISTEN / NOTIFY, so it works with any number of replicas.
package postgres

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/inbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/ports"
)

const (
	consumer = "notification-service"
	channel  = "notifications"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var _ ports.Store = (*Store)(nil)

func (s *Store) SetOwner(ctx context.Context, account uuid.UUID, customer string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO account_owners (account_id, customer_id) VALUES ($1, $2)
		ON CONFLICT (account_id) DO NOTHING`, account, customer)
	return err
}

func (s *Store) Owner(ctx context.Context, account uuid.UUID) (string, bool, error) {
	var c string
	err := s.pool.QueryRow(ctx, `SELECT customer_id FROM account_owners WHERE account_id = $1`, account).Scan(&c)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return c, err == nil, err
}

func (s *Store) Preferences(ctx context.Context, customer string) (domain.Preferences, error) {
	p := domain.DefaultPreferences
	err := s.pool.QueryRow(ctx, `SELECT push, email FROM preferences WHERE customer_id = $1`, customer).Scan(&p.Push, &p.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DefaultPreferences, nil
	}
	return p, err
}

func (s *Store) SetPreferences(ctx context.Context, customer string, p domain.Preferences) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO preferences (customer_id, push, email) VALUES ($1, $2, $3)
		ON CONFLICT (customer_id) DO UPDATE SET push = $2, email = $3, updated_at = now()`, customer, p.Push, p.Email)
	return err
}

func (s *Store) Save(ctx context.Context, eventID string, ns []domain.Notification) error {
	return inbox.Handle(ctx, s.pool, consumer, outbox.Event{ID: eventID}, func(tx pgx.Tx) error {
		for _, n := range ns {
			channels := make([]string, len(n.Channels))
			for i, c := range n.Channels {
				channels[i] = string(c)
			}
			tag, err := tx.Exec(ctx, `INSERT INTO notifications (id, customer_id, account_id, event_id, category, title, body,
				important, channels, occurred_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (event_id, account_id) DO NOTHING`,
				n.ID, n.CustomerID, n.AccountID, n.EventID, n.Category, n.Title, n.Body, n.Important, channels, n.OccurredAt)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				continue
			}
			for _, c := range n.Channels {
				if c == domain.InApp {
					continue
				}
				if _, err := tx.Exec(ctx, `INSERT INTO deliveries (notification_id, channel, status) VALUES ($1, $2, 'PENDING')`, n.ID, c); err != nil {
					return err
				}
			}
			// Delivered to listeners when the transaction commits.
			if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, n.CustomerID); err != nil {
				return err
			}
		}
		return nil
	})
}

const cols = `id, customer_id, account_id, event_id, category, title, body, important, channels, occurred_at, created_at, read_at`

func scan(r pgx.Row) (domain.Notification, error) {
	var (
		n        domain.Notification
		category string
		channels []string
	)
	err := r.Scan(&n.ID, &n.CustomerID, &n.AccountID, &n.EventID, &category, &n.Title, &n.Body, &n.Important, &channels,
		&n.OccurredAt, &n.Created, &n.ReadAt)
	n.Category = domain.Category(category)
	for _, c := range channels {
		n.Channels = append(n.Channels, domain.Channel(c))
	}
	return n, err
}

func collect(rows pgx.Rows, err error) ([]domain.Notification, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Notification, error) { return scan(r) })
}

func (s *Store) List(ctx context.Context, customer string, unreadOnly bool, beforeID uuid.UUID, limit int) ([]domain.Notification, int, error) {
	before := &beforeID
	if beforeID == uuid.Nil {
		before = nil
	}
	list, err := collect(s.pool.Query(ctx, `SELECT `+cols+` FROM notifications
		WHERE customer_id = $1 AND (NOT $2 OR read_at IS NULL) AND ($3::uuid IS NULL OR id < $3)
		ORDER BY id DESC LIMIT $4`, customer, unreadOnly, before, limit))
	if err != nil {
		return nil, 0, err
	}
	var unread int
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE customer_id = $1 AND read_at IS NULL`, customer).Scan(&unread)
	return list, unread, err
}

func (s *Store) After(ctx context.Context, customer string, afterID uuid.UUID, limit int) ([]domain.Notification, error) {
	return collect(s.pool.Query(ctx, `SELECT `+cols+` FROM notifications WHERE customer_id = $1 AND id > $2
		ORDER BY id LIMIT $3`, customer, afterID, limit))
}

func (s *Store) Latest(ctx context.Context, customer string) (uuid.UUID, error) {
	var id *uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT max(id::text)::uuid FROM notifications WHERE customer_id = $1`, customer).Scan(&id)
	if err != nil || id == nil {
		return uuid.Nil, err
	}
	return *id, nil
}

func (s *Store) MarkRead(ctx context.Context, customer string, ids []uuid.UUID) (int, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at = now()
		WHERE customer_id = $1 AND read_at IS NULL AND (coalesce(cardinality($2::uuid[]), 0) = 0 OR id = ANY($2))`, customer, ids)
	return int(tag.RowsAffected()), err
}

func (s *Store) PendingDeliveries(ctx context.Context, limit int) ([]ports.Delivery, error) {
	rows, err := s.pool.Query(ctx, `SELECT d.channel, d.attempts, n.id, n.customer_id, n.account_id, n.event_id,
		n.category, n.title, n.body, n.important, n.channels, n.occurred_at, n.created_at, n.read_at FROM deliveries d
		JOIN notifications n ON n.id = d.notification_id
		WHERE d.status = 'PENDING' ORDER BY d.updated_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ports.Delivery, error) {
		var (
			d        ports.Delivery
			ch       string
			category string
			channels []string
		)
		n := &d.Notification
		err := r.Scan(&ch, &d.Attempts, &n.ID, &n.CustomerID, &n.AccountID, &n.EventID, &category, &n.Title, &n.Body,
			&n.Important, &channels, &n.OccurredAt, &n.Created, &n.ReadAt)
		d.Channel, n.Category = domain.Channel(ch), domain.Category(category)
		return d, err
	})
}

func (s *Store) SetDelivery(ctx context.Context, notification uuid.UUID, ch domain.Channel, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE deliveries SET status = $3, attempts = attempts + 1, updated_at = now()
		WHERE notification_id = $1 AND channel = $2`, notification, ch, status)
	return err
}

// Listen calls wake(customer) for every notification committed by any
// replica, reconnecting if the connection drops.
func (s *Store) Listen(ctx context.Context, log *slog.Logger, wake func(customer string)) {
	for ctx.Err() == nil {
		if err := s.listen(ctx, wake); err != nil && ctx.Err() == nil {
			log.WarnContext(ctx, "notification listener reconnecting", "error", err)
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
	}
}

func (s *Store) listen(ctx context.Context, wake func(string)) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
		return err
	}
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			// The connection may be mid-wait; do not return it to the pool.
			_ = conn.Conn().Close(context.Background())
			return err
		}
		wake(n.Payload)
	}
}
