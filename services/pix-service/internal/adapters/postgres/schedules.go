package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	pg "github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/ports"
)

const scheduleCols = `id, idempotency_key, account_id, key, amount, currency, description, start_date, frequency,
	occurrences, executed, next_date, status, mandate_id, created_at`

func scanSchedule(row pgx.Row) (domain.Schedule, error) {
	var (
		s                      domain.Schedule
		amount                 int64
		currency, freq, status string
		mandate                *uuid.UUID
	)
	err := row.Scan(&s.ID, &s.IdempotencyKey, &s.AccountID, &s.Key, &amount, &currency, &s.Description, &s.StartDate,
		&freq, &s.Occurrences, &s.Executed, &s.NextDate, &status, &mandate, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, fmt.Errorf("%w: schedule", domain.ErrNotFound)
	}
	if err != nil {
		return s, err
	}
	s.Frequency, s.Status = domain.Frequency(freq), domain.ScheduleStatus(status)
	if mandate != nil {
		s.MandateID = *mandate
	}
	s.Amount, err = money.New(amount, money.Currency(currency))
	return s, err
}

func insertSchedule(ctx context.Context, q querier, s domain.Schedule) (*domain.Schedule, error) {
	tag, err := q.Exec(ctx, `INSERT INTO pix_schedules (id, idempotency_key, account_id, key, amount, currency, description,
		start_date, frequency, occurrences, next_date, status, mandate_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) ON CONFLICT (idempotency_key) DO NOTHING`,
		s.ID, s.IdempotencyKey, s.AccountID, s.Key, s.Amount.Minor(), string(s.Amount.Currency()), s.Description,
		s.StartDate, s.Frequency, s.Occurrences, s.NextDate, s.Status, nullable(s.MandateID))
	if err != nil || tag.RowsAffected() == 1 {
		return nil, err
	}
	existing, err := scanSchedule(q.QueryRow(ctx, `SELECT `+scheduleCols+` FROM pix_schedules WHERE idempotency_key = $1`, s.IdempotencyKey))
	return &existing, err
}

func (s *Store) InsertSchedule(ctx context.Context, sc domain.Schedule) (*domain.Schedule, error) {
	return insertSchedule(ctx, s.pool, sc)
}

func (s *Store) Schedule(ctx context.Context, id uuid.UUID) (domain.Schedule, error) {
	return scanSchedule(s.pool.QueryRow(ctx, `SELECT `+scheduleCols+` FROM pix_schedules WHERE id = $1`, id))
}

func (s *Store) Schedules(ctx context.Context, account uuid.UUID) ([]domain.Schedule, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+scheduleCols+` FROM pix_schedules WHERE account_id = $1 ORDER BY created_at DESC LIMIT 200`, account)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Schedule, error) { return scanSchedule(r) })
}

func (s *Store) SaveScheduleStatus(ctx context.Context, sc domain.Schedule) error {
	_, err := s.pool.Exec(ctx, `UPDATE pix_schedules SET status = $2, updated_at = now() WHERE id = $1`, sc.ID, sc.Status)
	return err
}

func (s *Store) ProcessDueSchedules(ctx context.Context, today time.Time, limit int, run func(*domain.Schedule) (ports.ScheduleRun, error)) (int, error) {
	done := 0
	for done < limit {
		found := false
		err := pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
			found = false
			sc, err := scanSchedule(tx.QueryRow(ctx, `SELECT `+scheduleCols+` FROM pix_schedules
				WHERE status = 'ACTIVE' AND next_date <= $1 ORDER BY next_date LIMIT 1 FOR UPDATE SKIP LOCKED`, today))
			if errors.Is(err, domain.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			found = true
			r, err := run(&sc)
			if err != nil {
				return err // technical: nothing recorded, retried on the next tick
			}
			if _, err := tx.Exec(ctx, `INSERT INTO pix_schedule_runs (schedule_id, occurrence, run_date, transfer_id, status, reason)
				VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`,
				sc.ID, r.Occurrence, r.Date, nullable(r.TransferID), r.Status, r.Reason); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE pix_schedules SET executed = $2, next_date = $3, status = $4, updated_at = now() WHERE id = $1`,
				sc.ID, sc.Executed, sc.NextDate, sc.Status)
			return err
		})
		if err != nil {
			return done, err
		}
		if !found {
			break
		}
		done++
	}
	return done, nil
}

func (s *Store) ScheduleRuns(ctx context.Context, id uuid.UUID) ([]ports.ScheduleRun, error) {
	rows, err := s.pool.Query(ctx, `SELECT occurrence, run_date, transfer_id, status, reason FROM pix_schedule_runs
		WHERE schedule_id = $1 ORDER BY occurrence`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ports.ScheduleRun, error) {
		var run ports.ScheduleRun
		var tid *uuid.UUID
		err := r.Scan(&run.Occurrence, &run.Date, &tid, &run.Status, &run.Reason)
		if tid != nil {
			run.TransferID = *tid
		}
		return run, err
	})
}

const mandateCols = `id, receiver_account_id, receiver_key, receiver_name, payer_account_id, max_amount, frequency,
	description, status, last_charge_period, created_at`

func scanMandate(row pgx.Row) (domain.Mandate, error) {
	var (
		m            domain.Mandate
		max          int64
		freq, status string
	)
	err := row.Scan(&m.ID, &m.ReceiverAccount, &m.ReceiverKey, &m.ReceiverName, &m.PayerAccount, &max, &freq,
		&m.Description, &status, &m.LastChargePeriod, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, fmt.Errorf("%w: mandate", domain.ErrNotFound)
	}
	if err != nil {
		return m, err
	}
	m.Frequency, m.Status = domain.Frequency(freq), domain.MandateStatus(status)
	m.MaxAmount, err = money.New(max, money.BRL)
	return m, err
}

func (s *Store) InsertMandate(ctx context.Context, m domain.Mandate) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO pix_mandates (id, receiver_account_id, receiver_key, receiver_name, payer_account_id,
		max_amount, frequency, description, status) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		m.ID, m.ReceiverAccount, m.ReceiverKey, m.ReceiverName, m.PayerAccount, m.MaxAmount.Minor(), m.Frequency, m.Description, m.Status)
	return err
}

func (s *Store) Mandate(ctx context.Context, id uuid.UUID) (domain.Mandate, error) {
	return scanMandate(s.pool.QueryRow(ctx, `SELECT `+mandateCols+` FROM pix_mandates WHERE id = $1`, id))
}

func (s *Store) MandatesByAccount(ctx context.Context, account uuid.UUID) ([]domain.Mandate, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+mandateCols+` FROM pix_mandates
		WHERE payer_account_id = $1 OR receiver_account_id = $1 ORDER BY created_at DESC LIMIT 200`, account)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Mandate, error) { return scanMandate(r) })
}

func saveMandate(ctx context.Context, tx pgx.Tx, m domain.Mandate) error {
	_, err := tx.Exec(ctx, `UPDATE pix_mandates SET status = $2, last_charge_period = $3, updated_at = now() WHERE id = $1`,
		m.ID, m.Status, m.LastChargePeriod)
	return err
}

func (s *Store) UpdateMandate(ctx context.Context, id uuid.UUID, change func(*domain.Mandate) error) (domain.Mandate, error) {
	var m domain.Mandate
	err := pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if m, err = scanMandate(tx.QueryRow(ctx, `SELECT `+mandateCols+` FROM pix_mandates WHERE id = $1 FOR UPDATE`, id)); err != nil {
			return err
		}
		if err := change(&m); err != nil {
			return err
		}
		if err := saveMandate(ctx, tx, m); err != nil {
			return err
		}
		if m.Status == domain.MandateRevoked {
			_, err = tx.Exec(ctx, `UPDATE pix_schedules SET status = 'CANCELLED', updated_at = now()
				WHERE mandate_id = $1 AND status = 'ACTIVE'`, id)
		}
		return err
	})
	return m, err
}

func (s *Store) ChargeMandate(ctx context.Context, id uuid.UUID, scheduleKey string, charge func(*domain.Mandate) (domain.Schedule, error)) (domain.Schedule, error) {
	var out domain.Schedule
	err := pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		m, err := scanMandate(tx.QueryRow(ctx, `SELECT `+mandateCols+` FROM pix_mandates WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if existing, err := scanSchedule(tx.QueryRow(ctx, `SELECT `+scheduleCols+` FROM pix_schedules WHERE idempotency_key = $1`, scheduleKey)); err == nil {
			out = existing // retried charge
			return nil
		} else if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		sc, err := charge(&m)
		if err != nil {
			return err
		}
		if _, err := insertSchedule(ctx, tx, sc); err != nil {
			return err
		}
		out = sc
		return saveMandate(ctx, tx, m)
	})
	return out, err
}
