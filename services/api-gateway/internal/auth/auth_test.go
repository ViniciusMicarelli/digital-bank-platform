package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const secret = "0123456789abcdef0123456789abcdef"

func TestIssueAndVerify(t *testing.T) {
	a, err := New(secret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := a.Issue("cust-1")
	if err != nil {
		t.Fatal(err)
	}
	if sub, err := a.Verify(tok); err != nil || sub != "cust-1" {
		t.Fatalf("verify = %q, %v", sub, err)
	}

	a.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if _, err := a.Verify(tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token err = %v", err)
	}
}

func TestRejectsForgedTokens(t *testing.T) {
	a, _ := New(secret, time.Minute)
	other, _ := New(strings.Repeat("x", 32), time.Minute)
	forged, _, _ := other.Issue("cust-1")

	none := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		Subject: "cust-1", Issuer: issuer, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	unsigned, _ := none.SignedString(jwt.UnsafeAllowNoneSignatureType)

	noExp := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: "cust-1", Issuer: issuer})
	noExpTok, _ := noExp.SignedString([]byte(secret))

	for name, tok := range map[string]string{"other secret": forged, "alg none": unsigned, "no expiry": noExpTok, "garbage": "a.b.c"} {
		if _, err := a.Verify(tok); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := New("short", time.Minute); err == nil {
		t.Error("short secrets must be refused")
	}
}
