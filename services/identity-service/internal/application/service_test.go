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

	got, pair, err := svc.Login(ctx, "ANA@example.com ", password, "", laptop)
	if err != nil || got != id {
		t.Fatalf("login = %v, %v", got, err)
	}
	if sub, err := verify(t, svc, pair.AccessToken); err != nil || sub != id.String() {
		t.Fatalf("access token: %q %v", sub, err)
	}
	if _, _, err := svc.Login(ctx, "ana@example.com", "wrong password!!", "", laptop); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("wrong password err = %v", err)
	}
	if _, _, err := svc.Login(ctx, "nobody@example.com", password, "", laptop); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("unknown email err = %v", err)
	}
}

func TestLockout(t *testing.T) {
	svc, _, now := setup(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "ana@example.com", password, "Ana Souza")
	for range domain.MaxFailedAttempts {
		_, _, _ = svc.Login(ctx, "ana@example.com", "wrong password!!", "", laptop)
	}
	if _, _, err := svc.Login(ctx, "ana@example.com", password, "", laptop); !errors.Is(err, domain.ErrLocked) {
		t.Fatalf("even the right password must be refused while locked: %v", err)
	}
	*now = now.Add(domain.LockDuration + time.Second)
	if _, _, err := svc.Login(ctx, "ana@example.com", password, "", laptop); err != nil {
		t.Fatalf("after the lock expires: %v", err)
	}
}

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	svc, _, _ := setup(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "ana@example.com", password, "Ana Souza")
	_, first, _ := svc.Login(ctx, "ana@example.com", password, "", laptop)

	second, err := svc.Refresh(ctx, first.RefreshToken, "")
	if err != nil || second.RefreshToken == first.RefreshToken {
		t.Fatalf("refresh = %v", err)
	}
	// The first token was rotated: presenting it again means it was stolen.
	if _, err := svc.Refresh(ctx, first.RefreshToken, ""); !errors.Is(err, domain.ErrRefreshReused) {
		t.Fatalf("reuse err = %v", err)
	}
	// ...and the legitimate holder's newer token is revoked too.
	if _, err := svc.Refresh(ctx, second.RefreshToken, ""); !errors.Is(err, domain.ErrRefreshReused) {
		t.Fatalf("family must be revoked: %v", err)
	}

	_, other, _ := svc.Login(ctx, "ana@example.com", password, "", laptop)
	if err := svc.Logout(ctx, other.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, other.RefreshToken, ""); err == nil {
		t.Fatal("refresh after logout must fail")
	}
	if _, err := svc.Refresh(ctx, "garbage", ""); !errors.Is(err, domain.ErrInvalidRefresh) {
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
	if _, _, err := svc.Login(ctx, "ana@example.com", password, "", laptop); err != nil {
		t.Fatal(err)
	}
	if err := svc.ConfirmTOTP(ctx, id, "000000"); !errors.Is(err, domain.ErrInvalidMFACode) {
		t.Fatalf("bad confirm err = %v", err)
	}
	code, _ := domain.TOTPCode(secret, domain.TOTPStep(*now))
	if err := svc.ConfirmTOTP(ctx, id, code); err != nil {
		t.Fatal(err)
	}

	if _, _, err := svc.Login(ctx, "ana@example.com", password, "", laptop); !errors.Is(err, domain.ErrMFARequired) {
		t.Fatalf("missing code err = %v", err)
	}
	// The confirmation code cannot be replayed to log in.
	if _, _, err := svc.Login(ctx, "ana@example.com", password, code, laptop); !errors.Is(err, domain.ErrInvalidMFACode) {
		t.Fatalf("replayed code err = %v", err)
	}
	*now = now.Add(30 * time.Second)
	next, _ := domain.TOTPCode(secret, domain.TOTPStep(*now))
	if _, _, err := svc.Login(ctx, "ana@example.com", password, next, laptop); err != nil {
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
	_, before, _ := svc.Login(ctx, "ana@example.com", password, "", laptop)

	kid, err := svc.RotateSigningKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, after, _ := svc.Login(ctx, "ana@example.com", password, "", laptop)
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
	svc, _, now := setup(t)
	ctx := context.Background()
	claimsOf := func(token string) *domain.AccessClaims {
		claims := &domain.AccessClaims{}
		_, _, _ = jwt.NewParser().ParseUnverified(token, claims)
		return claims
	}
	role := func(token string) domain.Role { return claimsOf(token).Role }
	_, _ = svc.Register(ctx, "cliente@example.com", "correct horse battery staple", "Cliente Um")
	_, pair, err := svc.Login(ctx, "cliente@example.com", "correct horse battery staple", "", laptop)
	if err != nil || role(pair.AccessToken) != domain.RoleCustomer {
		t.Fatalf("customer token role = %q %v", role(pair.AccessToken), err)
	}
	id, err := svc.EnsureAnalyst(ctx, "Analista@Bank.dev", "correct horse battery staple", "Analista", "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.EnsureAnalyst(ctx, "analista@bank.dev", "correct horse battery staple", "Analista", "")
	if err != nil || again != id {
		t.Fatalf("bootstrap is idempotent: %v %v", again, err)
	}
	_, pair, err = svc.Login(ctx, "analista@bank.dev", "correct horse battery staple", "", laptop)
	if err != nil || role(pair.AccessToken) != domain.RoleAnalyst {
		t.Fatalf("analyst token role = %q %v", role(pair.AccessToken), err)
	}
	refreshed, err := svc.Refresh(ctx, pair.RefreshToken, "")
	if err != nil || role(refreshed.AccessToken) != domain.RoleAnalyst {
		t.Fatalf("the role survives a refresh: %q %v", role(refreshed.AccessToken), err)
	}
	if amr := claimsOf(pair.AccessToken).AMR; len(amr) != 1 || amr[0] != "pwd" {
		t.Fatalf("password-only session amr = %v", amr)
	}

	// With a TOTP secret the analyst signs in with the code, and the token says so.
	secret := domain.NewTOTPSecret()
	if _, err := svc.EnsureAnalyst(ctx, "analista@bank.dev", "correct horse battery staple", "Analista", secret); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Login(ctx, "analista@bank.dev", "correct horse battery staple", "", laptop); !errors.Is(err, domain.ErrMFARequired) {
		t.Fatalf("login without the code: %v", err)
	}
	code, _ := domain.TOTPCode(secret, domain.TOTPStep(*now))
	_, pair, err = svc.Login(ctx, "analista@bank.dev", "correct horse battery staple", code, laptop)
	if err != nil {
		t.Fatal(err)
	}
	if amr := claimsOf(pair.AccessToken).AMR; len(amr) != 2 || amr[1] != "otp" {
		t.Fatalf("MFA session amr = %v", amr)
	}
	if _, err := svc.EnsureAnalyst(ctx, "outra@bank.dev", "correct horse battery staple", "Outra", "not base32!"); err == nil {
		t.Fatal("a bad TOTP secret must be refused")
	}
}

var (
	laptop = domain.Device{UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36", IP: "10.0.0.1"}
	phone  = domain.Device{UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1", IP: "10.0.0.2"}
)

func TestSessions(t *testing.T) {
	svc, pool, now := setup(t)
	ctx := context.Background()
	ana, _ := svc.Register(ctx, "ana@example.com", password, "Ana Souza")
	bob, _ := svc.Register(ctx, "bob@example.com", password, "Bob Lima")
	newDevice := func() (n int) {
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE payload->>'type' = 'identity.session.new_device'`).Scan(&n)
		return n
	}

	_, onLaptop, _ := svc.Login(ctx, "ana@example.com", password, "", laptop)
	_, _, _ = svc.Login(ctx, "ana@example.com", password, "", laptop) // the same device again
	if newDevice() != 0 {
		t.Fatal("the first device and a known one are not news")
	}
	_, onPhone, _ := svc.Login(ctx, "ana@example.com", password, "", phone)
	if newDevice() != 1 {
		t.Fatal("a new device should be announced")
	}
	list, err := svc.Sessions(ctx, ana)
	if err != nil || len(list) != 3 || list[0].Device != "Safari no iPhone" || list[2].Device != "Chrome no macOS" {
		t.Fatalf("sessions = %+v %v", list, err)
	}
	phoneSession := list[0].ID

	// A refresh keeps the session and moves its last use.
	*now = now.Add(time.Hour)
	if _, err := svc.Refresh(ctx, onLaptop.RefreshToken, "10.0.0.9"); err != nil {
		t.Fatal(err)
	}
	list, _ = svc.Sessions(ctx, ana)
	if list[0].Device != "Chrome no macOS" || list[0].IP != "10.0.0.9" {
		t.Fatalf("after refresh = %+v", list[0])
	}
	laptopSession := list[0].ID

	// Someone else's session is not found; ending one stops its refresh.
	if err := svc.RevokeSession(ctx, bob, phoneSession); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob ending ana's session: %v", err)
	}
	if err := svc.RevokeSession(ctx, ana, phoneSession); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, onPhone.RefreshToken, ""); err == nil {
		t.Fatal("an ended session must not refresh")
	}
	if err := svc.RevokeSession(ctx, ana, phoneSession); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ending it twice: %v", err)
	}
	// Ending all the others keeps the one asking.
	if n, err := svc.RevokeOtherSessions(ctx, ana, laptopSession); err != nil || n != 1 {
		t.Fatalf("others = %d %v", n, err)
	}
	if list, _ := svc.Sessions(ctx, ana); len(list) != 1 || list[0].ID != laptopSession {
		t.Fatalf("left = %+v", list)
	}
	// Sessions unused for longer than a refresh token lives are gone.
	*now = now.Add(domain.RefreshTokenTTL + time.Minute)
	if list, _ := svc.Sessions(ctx, ana); len(list) != 0 {
		t.Fatalf("expired = %+v", list)
	}
}
