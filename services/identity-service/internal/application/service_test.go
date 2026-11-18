package application

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/secretbox"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/migrations"
)

const password = "correct horse battery staple"

func setup(t *testing.T) (*Service, *pgxpool.Pool, *time.Time) {
	t.Helper()
	pool := pgtest.NewDB(t, migrations.FS, ".")
	box, _ := secretbox.New(bytes.Repeat([]byte{1}, 32))
	svc := New(pgstore.NewStore(pool), box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Now()
	svc.now = func() time.Time { return now }
	if err := svc.EnsureSigningKey(context.Background()); err != nil {
		t.Fatal(err)
	}
	return svc, pool, &now
}

// verify checks an access token the way the gateway does: with the JWKS only.
func verify(t *testing.T, svc *Service, token string) (string, error) {
	t.Helper()
	keys, err := svc.JWKS(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	claims := &jwt.RegisteredClaims{}
	_, err = jwt.ParseWithClaims(token, claims, func(tk *jwt.Token) (any, error) {
		for _, k := range keys {
			if k.Kid == tk.Header["kid"] {
				x, _ := base64.RawURLEncoding.DecodeString(k.X)
				return ed25519.PublicKey(x), nil
			}
		}
		return nil, errors.New("unknown kid")
	}, jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithIssuer(domain.Issuer), jwt.WithAudience(domain.Audience))
	return claims.Subject, err
}

func TestRegisterAndLogin(t *testing.T) {
	svc, _, _ := setup(t)
	ctx := context.Background()
	id, err := svc.Register(ctx, "Ana@Example.com", password, "Ana Souza")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Register(ctx, "ana@example.com", password, "Other"); !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("duplicate email err = %v", err)
	}
	if _, err := svc.Register(ctx, "b@example.com", "short", "Bob Lima"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("weak password err = %v", err)
	}

	got, pair, err := svc.Login(ctx, "ANA@example.com ", password, "")
	if err != nil || got != id {
		t.Fatalf("login = %v, %v", got, err)
	}
	if sub, err := verify(t, svc, pair.AccessToken); err != nil || sub != id.String() {
		t.Fatalf("access token: %q %v", sub, err)
	}
	if _, _, err := svc.Login(ctx, "ana@example.com", "wrong password!!", ""); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("wrong password err = %v", err)
	}
	if _, _, err := svc.Login(ctx, "nobody@example.com", password, ""); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("unknown email err = %v", err)
	}
}

func TestLockout(t *testing.T) {
	svc, _, now := setup(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "ana@example.com", password, "Ana Souza")
	for range domain.MaxFailedAttempts {
		_, _, _ = svc.Login(ctx, "ana@example.com", "wrong password!!", "")
	}
	if _, _, err := svc.Login(ctx, "ana@example.com", password, ""); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("even the right password must be refused while locked: %v", err)
	}
	*now = now.Add(domain.LockDuration + time.Second)
	if _, _, err := svc.Login(ctx, "ana@example.com", password, ""); err != nil {
		t.Fatalf("after the lock expires: %v", err)
	}
}

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	svc, _, _ := setup(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "ana@example.com", password, "Ana Souza")
	_, first, _ := svc.Login(ctx, "ana@example.com", password, "")

	second, err := svc.Refresh(ctx, first.RefreshToken)
	if err != nil || second.RefreshToken == first.RefreshToken {
		t.Fatalf("refresh = %v", err)
	}
	// The first token was rotated: presenting it again means it was stolen.
	if _, err := svc.Refresh(ctx, first.RefreshToken); !errors.Is(err, domain.ErrRefreshReused) {
		t.Fatalf("reuse err = %v", err)
	}
	// ...and the legitimate holder's newer token is revoked too.
	if _, err := svc.Refresh(ctx, second.RefreshToken); !errors.Is(err, domain.ErrRefreshReused) {
		t.Fatalf("family must be revoked: %v", err)
	}

	_, other, _ := svc.Login(ctx, "ana@example.com", password, "")
	if err := svc.Logout(ctx, other.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, other.RefreshToken); err == nil {
		t.Fatal("refresh after logout must fail")
	}
	if _, err := svc.Refresh(ctx, "garbage"); !errors.Is(err, domain.ErrInvalidRefresh) {
		t.Fatalf("unknown token err = %v", err)
	}
}

func TestTOTP(t *testing.T) {
	svc, pool, now := setup(t)
	ctx := context.Background()
	id, _ := svc.Register(ctx, "ana@example.com", password, "Ana Souza")

	secret, uri, err := svc.EnrollTOTP(ctx, id)
	if err != nil || !strings.Contains(uri, "secret="+secret) {
		t.Fatalf("enroll = %q, %v", uri, err)
	}
	// Not enabled until confirmed: login still works without a code.
	if _, _, err := svc.Login(ctx, "ana@example.com", password, ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfirmTOTP(ctx, id, "000000"); !errors.Is(err, domain.ErrInvalidMFACode) {
		t.Fatalf("bad confirm err = %v", err)
	}
	code, _ := domain.TOTPCode(secret, domain.TOTPStep(*now))
	if err := svc.ConfirmTOTP(ctx, id, code); err != nil {
		t.Fatal(err)
	}

	if _, _, err := svc.Login(ctx, "ana@example.com", password, ""); !errors.Is(err, domain.ErrMFARequired) {
		t.Fatalf("missing code err = %v", err)
	}
	// The confirmation code cannot be replayed to log in.
	if _, _, err := svc.Login(ctx, "ana@example.com", password, code); !errors.Is(err, domain.ErrInvalidMFACode) {
		t.Fatalf("replayed code err = %v", err)
	}
	*now = now.Add(30 * time.Second)
	next, _ := domain.TOTPCode(secret, domain.TOTPStep(*now))
	if _, _, err := svc.Login(ctx, "ana@example.com", password, next); err != nil {
		t.Fatalf("login with code: %v", err)
	}
	if _, _, err := svc.EnrollTOTP(ctx, id); !errors.Is(err, ErrTOTPAlreadyEnabled) {
		t.Fatalf("re-enroll err = %v", err)
	}

	// Secrets are encrypted at rest.
	var enc []byte
	_ = pool.QueryRow(ctx, `SELECT totp_secret_enc FROM credentials WHERE customer_id = $1`, id).Scan(&enc)
	if bytes.Contains(enc, []byte(secret)) {
		t.Fatal("TOTP secret stored in clear text")
	}
}

func TestKeyRotation(t *testing.T) {
	svc, pool, _ := setup(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "ana@example.com", password, "Ana Souza")
	_, before, _ := svc.Login(ctx, "ana@example.com", password, "")

	kid, err := svc.RotateSigningKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, after, _ := svc.Login(ctx, "ana@example.com", password, "")
	tok, _, _ := jwt.NewParser().ParseUnverified(after.AccessToken, &jwt.RegisteredClaims{})
	if tok.Header["kid"] != kid {
		t.Fatalf("new tokens must use the new key, kid = %v", tok.Header["kid"])
	}
	// Tokens signed before the rotation stay valid: the old key is still published.
	if _, err := verify(t, svc, before.AccessToken); err != nil {
		t.Fatalf("old token after rotation: %v", err)
	}
	keys, _ := svc.JWKS(ctx)
	if len(keys) != 2 {
		t.Fatalf("jwks has %d keys", len(keys))
	}
	var raw []byte
	_ = pool.QueryRow(ctx, `SELECT private_key_enc FROM signing_keys WHERE kid = $1`, kid).Scan(&raw)
	if len(raw) == ed25519.PrivateKeySize {
		t.Fatal("private key looks unencrypted")
	}
	// Once the grace period is over, the old key leaves the JWKS.
	svc.now = func() time.Time { return time.Now().Add(domain.KeyGracePeriod + time.Minute) }
	if keys, _ := svc.JWKS(ctx); len(keys) != 1 {
		t.Fatalf("expired key still published: %d keys", len(keys))
	}
}

func TestStepUp(t *testing.T) {
	svc, _, now := setup(t)
	ctx := context.Background()
	id, _ := svc.Register(ctx, "ana@example.com", password, "Ana Souza")

	tok, _, err := svc.StepUp(ctx, id, password, "")
	if err != nil {
		t.Fatal(err)
	}
	claims := &domain.StepUpClaims{}
	keys, _ := svc.JWKS(ctx)
	x, _ := base64.RawURLEncoding.DecodeString(keys[0].X)
	if _, err := jwt.ParseWithClaims(tok, claims, func(*jwt.Token) (any, error) { return ed25519.PublicKey(x), nil },
		jwt.WithAudience(domain.StepUpAudience)); err != nil || claims.Subject != id.String() || claims.AMR[0] != "pwd" {
		t.Fatalf("step-up token: %+v %v", claims, err)
	}
	// It is not an access token.
	if _, err := verify(t, svc, tok); err == nil {
		t.Fatal("a step-up token was accepted as an access token")
	}
	if _, _, err := svc.StepUp(ctx, id, "wrong password!!", ""); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("wrong password err = %v", err)
	}

	secret, _, _ := svc.EnrollTOTP(ctx, id)
	code, _ := domain.TOTPCode(secret, domain.TOTPStep(*now))
	_ = svc.ConfirmTOTP(ctx, id, code)
	if _, _, err := svc.StepUp(ctx, id, password, ""); !errors.Is(err, domain.ErrMFARequired) {
		t.Fatalf("TOTP enabled, no code: %v", err)
	}
	*now = now.Add(30 * time.Second)
	next, _ := domain.TOTPCode(secret, domain.TOTPStep(*now))
	tok, _, err = svc.StepUp(ctx, id, password, next)
	claims = &domain.StepUpClaims{}
	_, _ = jwt.ParseWithClaims(tok, claims, func(*jwt.Token) (any, error) { return ed25519.PublicKey(x), nil })
	if err != nil || len(claims.AMR) != 2 || claims.AMR[1] != "otp" {
		t.Fatalf("step-up with TOTP: %+v %v", claims, err)
	}
}

func TestAnalystRole(t *testing.T) {
	svc, _, _ := setup(t)
	ctx := context.Background()
	role := func(token string) domain.Role {
		claims := &domain.AccessClaims{}
		_, _, _ = jwt.NewParser().ParseUnverified(token, claims)
		return claims.Role
	}
	_, _ = svc.Register(ctx, "cliente@example.com", "correct horse battery staple", "Cliente Um")
	_, pair, err := svc.Login(ctx, "cliente@example.com", "correct horse battery staple", "")
	if err != nil || role(pair.AccessToken) != domain.RoleCustomer {
		t.Fatalf("customer token role = %q %v", role(pair.AccessToken), err)
	}
	id, err := svc.EnsureAnalyst(ctx, "Analista@Bank.dev", "correct horse battery staple", "Analista")
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.EnsureAnalyst(ctx, "analista@bank.dev", "correct horse battery staple", "Analista")
	if err != nil || again != id {
		t.Fatalf("bootstrap is idempotent: %v %v", again, err)
	}
	_, pair, err = svc.Login(ctx, "analista@bank.dev", "correct horse battery staple", "")
	if err != nil || role(pair.AccessToken) != domain.RoleAnalyst {
		t.Fatalf("analyst token role = %q %v", role(pair.AccessToken), err)
	}
	refreshed, err := svc.Refresh(ctx, pair.RefreshToken)
	if err != nil || role(refreshed.AccessToken) != domain.RoleAnalyst {
		t.Fatalf("the role survives a refresh: %q %v", role(refreshed.AccessToken), err)
	}
}
