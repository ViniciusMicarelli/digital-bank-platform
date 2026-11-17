// Package keyring holds the KMS logic: versioned master keys, protected by a
// root key, that wrap and unwrap data keys. A wrapped key is
// version (4 bytes, big endian) || AES-256-GCM(master[version], key, aad=name).
package keyring

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/secretbox"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrDenied          = errors.New("caller may not use this key")
	ErrDecrypt         = errors.New("cannot unwrap key")
)

var nameRE = regexp.MustCompile(`^[a-z0-9-]+/[a-z0-9-]{1,64}$`)

// Authorize checks that caller owns key name ("<caller>/<key>").
func Authorize(caller, name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("%w: key names look like <service>/<key>", ErrInvalidArgument)
	}
	if caller == "" || !strings.HasPrefix(name, caller+"/") {
		return fmt.Errorf("%w: %q may not use %q", ErrDenied, caller, name)
	}
	return nil
}

type Keyring struct {
	pool *pgxpool.Pool
	root *secretbox.Box

	mu    sync.Mutex
	cache map[string]map[int]*secretbox.Box
}

// New uses root (32 bytes) to protect the master keys at rest.
func New(pool *pgxpool.Pool, root []byte) (*Keyring, error) {
	box, err := secretbox.New(root)
	if err != nil {
		return nil, err
	}
	return &Keyring{pool: pool, root: box, cache: map[string]map[int]*secretbox.Box{}}, nil
}

func masterAAD(name string, version int) []byte {
	return fmt.Appendf(nil, "master:%s:%d", name, version)
}

// current returns the newest version of a master key, creating version 1.
func (k *Keyring) current(ctx context.Context, name string) (int, *secretbox.Box, error) {
	var version int
	err := k.pool.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM master_keys WHERE name = $1`, name).Scan(&version)
	if err != nil {
		return 0, nil, err
	}
	if version == 0 {
		if version, err = k.rotate(ctx, name); err != nil {
			return 0, nil, err
		}
	}
	box, err := k.version(ctx, name, version)
	return version, box, err
}

func (k *Keyring) version(ctx context.Context, name string, version int) (*secretbox.Box, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if b, ok := k.cache[name][version]; ok {
		return b, nil
	}
	var enc []byte
	err := k.pool.QueryRow(ctx, `SELECT key_enc FROM master_keys WHERE name = $1 AND version = $2`, name, version).Scan(&enc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDecrypt
	}
	if err != nil {
		return nil, err
	}
	raw, err := k.root.Open(enc, masterAAD(name, version))
	if err != nil {
		return nil, fmt.Errorf("master key %s v%d: %w", name, version, err)
	}
	b, err := secretbox.New(raw)
	if err != nil {
		return nil, err
	}
	if k.cache[name] == nil {
		k.cache[name] = map[int]*secretbox.Box{}
	}
	k.cache[name][version] = b
	return b, nil
}

// rotate adds the next master key version (concurrent rotations serialize
// on an advisory lock).
func (k *Keyring) rotate(ctx context.Context, name string) (int, error) {
	var version int
	err := postgres.InTx(ctx, k.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, name); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM master_keys WHERE name = $1`, name).Scan(&version); err != nil {
			return err
		}
		raw := make([]byte, 32)
		_, _ = rand.Read(raw)
		_, err := tx.Exec(ctx, `INSERT INTO master_keys (name, version, key_enc) VALUES ($1, $2, $3)`,
			name, version, k.root.Seal(raw, masterAAD(name, version)))
		return err
	})
	return version, err
}

// Rotate starts a new master key version for name.
func (k *Keyring) Rotate(ctx context.Context, name string) (int, error) { return k.rotate(ctx, name) }

// Wrap encrypts a key under the current master version.
func (k *Keyring) Wrap(ctx context.Context, name string, key []byte) ([]byte, int, error) {
	if len(key) == 0 || len(key) > 64 {
		return nil, 0, fmt.Errorf("%w: keys must have 1-64 bytes", ErrInvalidArgument)
	}
	version, box, err := k.current(ctx, name)
	if err != nil {
		return nil, 0, err
	}
	out := binary.BigEndian.AppendUint32(nil, uint32(version))
	return append(out, box.Seal(key, []byte(name))...), version, nil
}

// Unwrap decrypts a wrapped key with whichever version wrapped it.
func (k *Keyring) Unwrap(ctx context.Context, name string, wrapped []byte) ([]byte, int, error) {
	if len(wrapped) < 5 {
		return nil, 0, ErrDecrypt
	}
	version := int(binary.BigEndian.Uint32(wrapped[:4]))
	box, err := k.version(ctx, name, version)
	if err != nil {
		return nil, 0, err
	}
	key, err := box.Open(wrapped[4:], []byte(name))
	if err != nil {
		return nil, 0, ErrDecrypt
	}
	return key, version, nil
}

// GenerateDataKey returns a new random 32-byte key, clear and wrapped.
func (k *Keyring) GenerateDataKey(ctx context.Context, name string) ([]byte, []byte, int, error) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	wrapped, version, err := k.Wrap(ctx, name, key)
	return key, wrapped, version, err
}

// ReEncrypt re-wraps under the current version; changed=false if it already was.
func (k *Keyring) ReEncrypt(ctx context.Context, name string, wrapped []byte) ([]byte, int, bool, error) {
	key, version, err := k.Unwrap(ctx, name, wrapped)
	if err != nil {
		return nil, 0, false, err
	}
	current, _, err := k.current(ctx, name)
	if err != nil {
		return nil, 0, false, err
	}
	if version == current {
		return wrapped, version, false, nil
	}
	out, v, err := k.Wrap(ctx, name, key)
	return out, v, true, err
}
