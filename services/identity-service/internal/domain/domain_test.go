package domain

import (
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestPasswordHashing(t *testing.T) {
	h := HashPassword("correct horse battery")
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash format = %s", h)
	}
	if !VerifyPassword("correct horse battery", h) || VerifyPassword("correct horse batterY", h) {
		t.Fatal("verification is wrong")
	}
	if HashPassword("same password") == HashPassword("same password") {
		t.Fatal("hashes must be salted")
	}
	for _, bad := range []string{"", "$bcrypt$x", "$argon2id$v=19$m=1,t=1,p=1$!!$!!"} {
		if VerifyPassword("x", bad) {
			t.Errorf("malformed hash %q accepted", bad)
		}
	}
	if ValidatePassword("short") == nil || ValidatePassword(strings.Repeat("a", 129)) == nil || ValidatePassword("ten chars!") != nil {
		t.Fatal("password length policy")
	}
}

// RFC 6238 appendix B vectors (SHA-1, 8 digits) truncated to 6 digits.
func TestTOTPVectors(t *testing.T) {
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	vectors := map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"}
	for unix, want := range vectors {
		got, err := TOTPCode(secret, TOTPStep(time.Unix(unix, 0)))
		if err != nil || got != want {
			t.Errorf("t=%d: %s, want %s", unix, got, want)
		}
	}
}

func TestVerifyTOTP(t *testing.T) {
	secret := NewTOTPSecret()
	now := time.Unix(1_800_000_000, 0)
	code, _ := TOTPCode(secret, TOTPStep(now))
	step, ok := VerifyTOTP(secret, code, now, 0)
	if !ok {
		t.Fatal("current code rejected")
	}
	if _, ok := VerifyTOTP(secret, code, now, step); ok {
		t.Fatal("a code must not be accepted twice")
	}
	prev, _ := TOTPCode(secret, TOTPStep(now)-1)
	if _, ok := VerifyTOTP(secret, prev, now, 0); !ok {
		t.Fatal("previous step should be tolerated (clock drift)")
	}
	old, _ := TOTPCode(secret, TOTPStep(now)-3)
	if _, ok := VerifyTOTP(secret, old, now, 0); ok {
		t.Fatal("old code accepted")
	}
	if !strings.HasPrefix(TOTPURI(secret, "ana@example.com"), "otpauth://totp/Digital%20Bank:ana@example.com?") {
		t.Fatal(TOTPURI(secret, "ana@example.com"))
	}
}

func TestAccessToken(t *testing.T) {
	k := NewSigningKey()
	id := uuid.New()
	tok, exp, err := SignAccessToken(k, id, RoleCustomer, []string{"pwd"}, time.Now())
	if err != nil || time.Until(exp) > AccessTokenTTL {
		t.Fatal(err)
	}
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(tok, claims, func(tk *jwt.Token) (any, error) {
		if tk.Header["kid"] != k.Kid {
			t.Errorf("kid = %v", tk.Header["kid"])
		}
		return k.Public, nil
	}, jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithAudience(Audience), jwt.WithIssuer(Issuer))
	if err != nil || !parsed.Valid || claims.Subject != id.String() {
		t.Fatalf("parse: %v", err)
	}
	other := NewSigningKey()
	if _, err := jwt.Parse(tok, func(*jwt.Token) (any, error) { return ed25519.PublicKey(other.Public), nil }); err == nil {
		t.Fatal("another key must not verify the token")
	}
	if j := k.JWK(); j.Kty != "OKP" || j.Crv != "Ed25519" || j.Alg != "EdDSA" || len(j.X) != 43 {
		t.Fatalf("jwk = %+v", j)
	}
}

func TestLockout(t *testing.T) {
	now := time.Now()
	c := Credentials{}
	for range MaxFailedAttempts - 1 {
		c.RegisterFailure(now)
	}
	if c.Locked(now) {
		t.Fatal("locked too early")
	}
	c.RegisterFailure(now)
	if !c.Locked(now) || c.Locked(now.Add(LockDuration+time.Second)) {
		t.Fatal("lock window wrong")
	}
	c.RegisterSuccess()
	if c.Locked(now) || c.FailedAttempts != 0 {
		t.Fatal("success must unlock")
	}
}

func TestNormalizeEmail(t *testing.T) {
	if e, err := NormalizeEmail("  Ana@Example.COM "); err != nil || e != "ana@example.com" {
		t.Fatalf("%q %v", e, err)
	}
	for _, bad := range []string{"", "ana", "ana@x", "Ana <ana@x.com>"} {
		if _, err := NormalizeEmail(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
