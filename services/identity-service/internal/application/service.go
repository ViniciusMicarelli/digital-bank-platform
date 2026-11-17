// Package application implements registration, login, MFA, token refresh and
// signing-key management.
package application

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/secretbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/internal/ports"
)

// ErrTOTPAlreadyEnabled is returned when enrolling twice.
var ErrTOTPAlreadyEnabled = errors.New("TOTP is already enabled")

// TokenPair is what a successful login or refresh returns.
type TokenPair struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

type Service struct {
	store ports.Store
	box   *secretbox.Box
	log   *slog.Logger
	now   func() time.Time

	mu       sync.Mutex
	active   *domain.SigningKey
	activeAt time.Time
}

func New(store ports.Store, box *secretbox.Box, log *slog.Logger) *Service {
	return &Service{store: store, box: box, log: log, now: time.Now}
}

// Register creates credentials and returns the new customer id.
func (s *Service) Register(ctx context.Context, email, password, fullName string) (uuid.UUID, error) {
	email, err := domain.NormalizeEmail(email)
	if err != nil {
		return uuid.Nil, err
	}
	if err := domain.ValidatePassword(password); err != nil {
		return uuid.Nil, err
	}
	if len(fullName) < 3 || len(fullName) > 120 {
		return uuid.Nil, fmt.Errorf("%w: full name must have 3 to 120 characters", domain.ErrInvalidArgument)
	}
	c := domain.Credentials{
		CustomerID: uuid.Must(uuid.NewV7()), Email: email, FullName: fullName,
		PasswordHash: domain.HashPassword(password),
	}
	// No PII in events: consumers get the id and look up what they need.
	ev, err := outbox.NewEvent(ctx, "identity-service", "identity.customer.registered",
		map[string]string{"customer_id": c.CustomerID.String()})
	if err != nil {
		return uuid.Nil, err
	}
	msg := outbox.Message{Topic: "bank.identity.events", Key: c.CustomerID.String(), Event: ev}
	if err := s.store.InsertCredentials(ctx, c, msg); err != nil {
		return uuid.Nil, err
	}
	return c.CustomerID, nil
}

// Login checks the password (and the TOTP code when enabled) and starts a
// new session. Failures are counted even though an error is returned, so the
// transaction commits and the authentication error is reported afterwards.
func (s *Service) Login(ctx context.Context, email, password, code string) (uuid.UUID, TokenPair, error) {
	email, _ = domain.NormalizeEmail(email) // an invalid email simply does not match
	now := s.now()
	var (
		loginErr error
		customer uuid.UUID
		pair     TokenPair
	)
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		loginErr = nil
		c, err := tx.CredentialsByEmailForUpdate(ctx, email)
		if errors.Is(err, domain.ErrNotFound) {
			domain.VerifyAgainstDummy(password) // same cost as a wrong password
			loginErr = domain.ErrInvalidCredentials
			return nil
		}
		if err != nil {
			return err
		}
		switch {
		case c.Locked(now):
			loginErr = domain.ErrLocked
			return nil
		case !domain.VerifyPassword(password, c.PasswordHash):
			c.RegisterFailure(now)
			loginErr = domain.ErrInvalidCredentials
			return tx.SaveCredentials(ctx, c)
		case c.TOTPEnabled && code == "":
			loginErr = domain.ErrMFARequired
			return nil
		case c.TOTPEnabled:
			step, ok, err := s.checkTOTP(c, code, now)
			if err != nil {
				return err
			}
			if !ok {
				c.RegisterFailure(now)
				loginErr = domain.ErrInvalidMFACode
				return tx.SaveCredentials(ctx, c)
			}
			c.TOTPLastStep = step
		}
		c.RegisterSuccess()
		if err := tx.SaveCredentials(ctx, c); err != nil {
			return err
		}
		customer = c.CustomerID
		pair, _, err = s.issue(ctx, tx, c.CustomerID, uuid.Must(uuid.NewV7()), now)
		return err
	})
	if err != nil {
		return uuid.Nil, TokenPair{}, err
	}
	return customer, pair, loginErr
}

// Refresh rotates a refresh token. A token that was already rotated means it
// leaked (or was replayed): the whole family is revoked.
func (s *Service) Refresh(ctx context.Context, token string) (TokenPair, error) {
	now := s.now()
	var (
		pair       TokenPair
		refreshErr error
	)
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		refreshErr = nil
		rt, err := tx.RefreshTokenForUpdate(ctx, domain.HashRefreshToken(token))
		if errors.Is(err, domain.ErrNotFound) {
			refreshErr = domain.ErrInvalidRefresh
			return nil
		}
		if err != nil {
			return err
		}
		if rt.RevokedAt != nil {
			s.log.WarnContext(ctx, "refresh token reuse detected, revoking session", "customer_id", rt.CustomerID, "family_id", rt.FamilyID)
			refreshErr = domain.ErrRefreshReused
			return tx.RevokeFamily(ctx, rt.FamilyID) // committed on purpose
		}
		if !now.Before(rt.ExpiresAt) {
			refreshErr = domain.ErrInvalidRefresh
			return nil
		}
		var nextID uuid.UUID
		if pair, nextID, err = s.issue(ctx, tx, rt.CustomerID, rt.FamilyID, now); err != nil {
			return err
		}
		return tx.RevokeRefreshToken(ctx, rt.ID, &nextID)
	})
	if err != nil {
		return TokenPair{}, err
	}
	return pair, refreshErr
}

// Logout revokes the session the token belongs to. Unknown tokens are ignored.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.store.InTx(ctx, func(tx ports.Tx) error {
		rt, err := tx.RefreshTokenForUpdate(ctx, domain.HashRefreshToken(token))
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return tx.RevokeFamily(ctx, rt.FamilyID)
	})
}

// issue signs an access token and stores a new refresh token in family.
func (s *Service) issue(ctx context.Context, tx ports.Tx, customer, family uuid.UUID, now time.Time) (TokenPair, uuid.UUID, error) {
	key, err := s.signingKey(ctx)
	if err != nil {
		return TokenPair{}, uuid.Nil, err
	}
	access, accessExp, err := domain.SignAccessToken(key, customer, now)
	if err != nil {
		return TokenPair{}, uuid.Nil, err
	}
	refresh, hash := domain.NewRefreshToken()
	rt := ports.RefreshToken{
		ID: uuid.Must(uuid.NewV7()), FamilyID: family, CustomerID: customer,
		Hash: hash, ExpiresAt: now.Add(domain.RefreshTokenTTL),
	}
	if err := tx.InsertRefreshToken(ctx, rt); err != nil {
		return TokenPair{}, uuid.Nil, err
	}
	return TokenPair{AccessToken: access, AccessExpiresAt: accessExp, RefreshToken: refresh, RefreshExpiresAt: rt.ExpiresAt}, rt.ID, nil
}

func totpAAD(customer uuid.UUID) []byte { return []byte("totp:" + customer.String()) }

func (s *Service) checkTOTP(c domain.Credentials, code string, now time.Time) (int64, bool, error) {
	secret, err := s.box.Open(c.TOTPSecretEnc, totpAAD(c.CustomerID))
	if err != nil {
		return 0, false, fmt.Errorf("decrypt TOTP secret: %w", err)
	}
	step, ok := domain.VerifyTOTP(string(secret), code, now, c.TOTPLastStep)
	return step, ok, nil
}

// EnrollTOTP stores a new (not yet enabled) secret and returns it.
func (s *Service) EnrollTOTP(ctx context.Context, customer uuid.UUID) (secret, uri string, err error) {
	err = s.store.InTx(ctx, func(tx ports.Tx) error {
		c, err := tx.CredentialsForUpdate(ctx, customer)
		if err != nil {
			return err
		}
		if c.TOTPEnabled {
			return ErrTOTPAlreadyEnabled
		}
		secret = domain.NewTOTPSecret()
		uri = domain.TOTPURI(secret, c.Email)
		c.TOTPSecretEnc = s.box.Seal([]byte(secret), totpAAD(customer))
		return tx.SaveCredentials(ctx, c)
	})
	return secret, uri, err
}

// ConfirmTOTP enables TOTP once the customer proves the app is set up.
func (s *Service) ConfirmTOTP(ctx context.Context, customer uuid.UUID, code string) error {
	var confirmErr error
	err := s.store.InTx(ctx, func(tx ports.Tx) error {
		c, err := tx.CredentialsForUpdate(ctx, customer)
		if err != nil {
			return err
		}
		if c.TOTPEnabled {
			return ErrTOTPAlreadyEnabled
		}
		if c.TOTPSecretEnc == nil {
			return fmt.Errorf("%w: enroll first", domain.ErrInvalidArgument)
		}
		step, ok, err := s.checkTOTP(c, code, s.now())
		if err != nil {
			return err
		}
		if !ok {
			confirmErr = domain.ErrInvalidMFACode
			return nil
		}
		c.TOTPEnabled, c.TOTPLastStep = true, step
		return tx.SaveCredentials(ctx, c)
	})
	if err != nil {
		return err
	}
	return confirmErr
}

// ---- signing keys ----

func (s *Service) seal(k domain.SigningKey) ports.StoredKey {
	return ports.StoredKey{Kid: k.Kid, PublicKey: k.Public, PrivateKeyEnc: s.box.Seal(k.Private, []byte("signing-key:"+k.Kid))}
}

func (s *Service) open(k ports.StoredKey) (domain.SigningKey, error) {
	priv, err := s.box.Open(k.PrivateKeyEnc, []byte("signing-key:"+k.Kid))
	if err != nil {
		return domain.SigningKey{}, fmt.Errorf("decrypt signing key %s: %w", k.Kid, err)
	}
	return domain.SigningKey{Kid: k.Kid, Private: ed25519.PrivateKey(priv), Public: k.PublicKey, CreatedAt: k.CreatedAt}, nil
}

// EnsureSigningKey creates the first key on a fresh database.
func (s *Service) EnsureSigningKey(ctx context.Context) error {
	if _, err := s.store.ActiveKey(ctx); err == nil {
		return nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	_, err := s.store.InsertFirstKey(ctx, s.seal(domain.NewSigningKey()))
	return err
}

// signingKey returns the active key, cached briefly so a rotation done by
// another replica is picked up within a minute.
func (s *Service) signingKey(ctx context.Context) (domain.SigningKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil && s.now().Sub(s.activeAt) < time.Minute {
		return *s.active, nil
	}
	stored, err := s.store.ActiveKey(ctx)
	if err != nil {
		return domain.SigningKey{}, err
	}
	k, err := s.open(stored)
	if err != nil {
		return domain.SigningKey{}, err
	}
	s.active, s.activeAt = &k, s.now()
	return k, nil
}

// RotateSigningKey starts signing with a new key. Tokens signed by the old
// one stay verifiable for KeyGracePeriod.
func (s *Service) RotateSigningKey(ctx context.Context) (string, error) {
	k := domain.NewSigningKey()
	if err := s.store.RotateKey(ctx, s.seal(k), domain.KeyGracePeriod); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.active = nil
	s.mu.Unlock()
	s.log.InfoContext(ctx, "signing key rotated", "kid", k.Kid)
	return k.Kid, nil
}

// JWKS returns the public keys verifiers should accept.
func (s *Service) JWKS(ctx context.Context) ([]domain.JWK, error) {
	keys, err := s.store.PublishedKeys(ctx, s.now())
	if err != nil {
		return nil, err
	}
	out := make([]domain.JWK, len(keys))
	for i, k := range keys {
		out[i] = domain.SigningKey{Kid: k.Kid, Public: k.PublicKey}.JWK()
	}
	return out, nil
}

// RunKeyRotation rotates the active key once it is older than maxAge.
func (s *Service) RunKeyRotation(ctx context.Context, maxAge time.Duration) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if k, err := s.store.ActiveKey(ctx); err == nil && s.now().Sub(k.CreatedAt) > maxAge {
			if _, err := s.RotateSigningKey(ctx); err != nil {
				s.log.ErrorContext(ctx, "scheduled key rotation", "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
