// Package postgres stores customer profiles, with the CPF encrypted.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	pg "github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/secretbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/ports"
)

type Store struct {
	pool     *pgxpool.Pool
	box      *secretbox.Box
	indexKey []byte
}

func NewStore(pool *pgxpool.Pool, key []byte) (*Store, error) {
	box, err := secretbox.New(key)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool, box: box, indexKey: secretbox.DeriveKey(key, "user-cpf-index-v1")}, nil
}

var _ ports.Store = (*Store)(nil)

func aad(customer string) []byte { return []byte("user-cpf:" + customer) }

func (s *Store) Profile(ctx context.Context, customer string) (domain.Profile, error) {
	var (
		p      domain.Profile
		enc    []byte
		status string
	)
	a := &p.Address
	err := s.pool.QueryRow(ctx, `SELECT customer_id, full_name, cpf_enc, birth_date, phone, postal_code, street, number, complement,
		district, city, state, kyc_status, kyc_reason, decided_at, updated_at FROM profiles WHERE customer_id = $1`, customer).
		Scan(&p.CustomerID, &p.FullName, &enc, &p.BirthDate, &p.Phone, &a.PostalCode, &a.Street, &a.Number, &a.Complement,
			&a.District, &a.City, &a.State, &status, &p.KYCReason, &p.DecidedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, fmt.Errorf("%w: profile", domain.ErrNotFound)
	}
	if err != nil {
		return p, err
	}
	p.KYCStatus = domain.KYCStatus(status)
	cpf, err := s.box.Open(enc, aad(customer))
	p.CPF = string(cpf)
	return p, err
}

func (s *Store) Save(ctx context.Context, p domain.Profile, events ...outbox.Message) error {
	a := p.Address
	err := pg.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO profiles (customer_id, full_name, cpf_enc, cpf_index, birth_date, phone, postal_code,
			street, number, complement, district, city, state, kyc_status, kyc_reason, decided_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			ON CONFLICT (customer_id) DO UPDATE SET full_name = $2, cpf_enc = $3, cpf_index = $4, birth_date = $5, phone = $6,
			postal_code = $7, street = $8, number = $9, complement = $10, district = $11, city = $12, state = $13,
			kyc_status = $14, kyc_reason = $15, decided_at = $16, updated_at = now()`,
			p.CustomerID, p.FullName, s.box.Seal([]byte(p.CPF), aad(p.CustomerID)), secretbox.BlindIndex(s.indexKey, p.CPF),
			p.BirthDate, p.Phone, a.PostalCode, a.Street, a.Number, a.Complement, a.District, a.City, a.State,
			p.KYCStatus, p.KYCReason, p.DecidedAt)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if err := outbox.Insert(ctx, tx, ev); err != nil {
				return err
			}
		}
		return nil
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "profiles_cpf_in_use" {
		return domain.Refuse("DOCUMENT_IN_USE", "this CPF belongs to another customer")
	}
	return err
}

func (s *Store) Pending(ctx context.Context, before time.Time, limit int) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT customer_id FROM profiles WHERE kyc_status = 'PENDING' AND updated_at < $1
		ORDER BY updated_at LIMIT $2`, before, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
