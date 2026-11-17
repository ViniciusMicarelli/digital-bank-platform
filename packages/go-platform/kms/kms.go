// Package kms gives a service its data key through kms-service (envelope
// encryption): the service stores only the wrapped key, in its own database,
// and asks the KMS to unwrap it at startup. No master key in the environment.
package kms

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	kmsv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/kms/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/secretbox"
)

// LoadDataKey returns the clear data key called name ("<service>/<purpose>").
// On first use it creates one, or wraps legacy (a key that predates the KMS,
// so existing ciphertexts stay readable). Replicas starting together agree
// on one key: the first insert wins. A key wrapped by an old master key
// version is re-wrapped under the current one.
func LoadDataKey(ctx context.Context, pool *pgxpool.Pool, c kmsv1.KmsServiceClient, name string, legacy []byte) ([]byte, error) {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS data_keys (
		name TEXT PRIMARY KEY, wrapped BYTEA NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return nil, err
	}
	var wrapped []byte
	err := pool.QueryRow(ctx, `SELECT wrapped FROM data_keys WHERE name = $1`, name).Scan(&wrapped)
	if errors.Is(err, pgx.ErrNoRows) {
		if legacy != nil {
			resp, err := c.Encrypt(ctx, &kmsv1.EncryptRequest{KeyName: name, Plaintext: legacy})
			if err != nil {
				return nil, fmt.Errorf("kms: import key: %w", err)
			}
			wrapped = resp.GetWrapped()
		} else {
			resp, err := c.GenerateDataKey(ctx, &kmsv1.GenerateDataKeyRequest{KeyName: name})
			if err != nil {
				return nil, fmt.Errorf("kms: generate key: %w", err)
			}
			wrapped = resp.GetWrapped()
		}
		if _, err := pool.Exec(ctx, `INSERT INTO data_keys (name, wrapped) VALUES ($1, $2) ON CONFLICT DO NOTHING`, name, wrapped); err != nil {
			return nil, err
		}
		if err := pool.QueryRow(ctx, `SELECT wrapped FROM data_keys WHERE name = $1`, name).Scan(&wrapped); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	if re, err := c.ReEncrypt(ctx, &kmsv1.ReEncryptRequest{KeyName: name, Wrapped: wrapped}); err != nil {
		return nil, fmt.Errorf("kms: re-encrypt: %w", err)
	} else if re.GetChanged() {
		if _, err := pool.Exec(ctx, `UPDATE data_keys SET wrapped = $2, updated_at = now() WHERE name = $1 AND wrapped = $3`,
			name, re.GetWrapped(), wrapped); err != nil {
			return nil, err
		}
		wrapped = re.GetWrapped()
	}
	resp, err := c.Decrypt(ctx, &kmsv1.DecryptRequest{KeyName: name, Wrapped: wrapped})
	if err != nil {
		return nil, fmt.Errorf("kms: decrypt: %w", err)
	}
	return resp.GetPlaintext(), nil
}

// MasterKey resolves a service's 32-byte key: through kms-service when
// KMS_ADDR is set (importing legacyEnv's value on first use, if present),
// otherwise straight from legacyEnv (local `go run`, tests).
func MasterKey(ctx context.Context, log *slog.Logger, pool *pgxpool.Pool, name, legacyEnv string) ([]byte, error) {
	var legacy []byte
	if v := os.Getenv(legacyEnv); v != "" {
		k, err := secretbox.DecodeKey(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", legacyEnv, err)
		}
		legacy = k
	}
	addr := os.Getenv("KMS_ADDR")
	if addr == "" {
		if legacy == nil {
			return nil, fmt.Errorf("set KMS_ADDR (or %s for local development)", legacyEnv)
		}
		log.Warn("using a key from the environment; set KMS_ADDR to use kms-service", "env", legacyEnv)
		return legacy, nil
	}
	conn, err := grpcserver.Dial(addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	key, err := LoadDataKey(ctx, pool, kmsv1.NewKmsServiceClient(conn), name, legacy)
	if err != nil {
		return nil, err
	}
	if legacy != nil {
		log.Warn("key imported into kms-service; remove it from the environment", "env", legacyEnv)
	}
	log.Info("data key loaded from kms-service", "key", name)
	return key, nil
}
