package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type key struct {
	kid  string
	priv ed25519.PrivateKey
	jwk  JWK
}

func newKey(kid string) key {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	return key{kid: kid, priv: priv, jwk: JWK{Kid: kid, Kty: "OKP", Crv: "Ed25519", X: base64.RawURLEncoding.EncodeToString(pub)}}
}

func (k key) sign(t *testing.T, claims jwt.RegisteredClaims, method jwt.SigningMethod) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	tok.Header["kid"] = k.kid
	var signKey any = k.priv
	if method == jwt.SigningMethodNone {
		signKey = jwt.UnsafeAllowNoneSignatureType
	}
	s, err := tok.SignedString(signKey)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func valid(sub string) jwt.RegisteredClaims {
	return jwt.RegisteredClaims{Subject: sub, Issuer: Issuer, Audience: jwt.ClaimStrings{Audience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}
}

func TestVerify(t *testing.T) {
	k1 := newKey("k1")
	fetches := 0
	published := []JWK{k1.jwk}
	v := NewVerifier(func(context.Context) ([]JWK, error) { fetches++; return published, nil })
	ctx := context.Background()

	if sub, err := v.Verify(ctx, k1.sign(t, valid("cust-1"), jwt.SigningMethodEdDSA)); err != nil || sub != "cust-1" {
		t.Fatalf("valid token: %q %v", sub, err)
	}

	bad := map[string]string{
		"expired": k1.sign(t, jwt.RegisteredClaims{Subject: "c", Issuer: Issuer, Audience: jwt.ClaimStrings{Audience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))}, jwt.SigningMethodEdDSA),
		"no expiry":      k1.sign(t, jwt.RegisteredClaims{Subject: "c", Issuer: Issuer, Audience: jwt.ClaimStrings{Audience}}, jwt.SigningMethodEdDSA),
		"wrong audience": k1.sign(t, jwt.RegisteredClaims{Subject: "c", Issuer: Issuer, Audience: jwt.ClaimStrings{"other"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}, jwt.SigningMethodEdDSA),
		"wrong issuer":   k1.sign(t, jwt.RegisteredClaims{Subject: "c", Issuer: "evil", Audience: jwt.ClaimStrings{Audience}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}, jwt.SigningMethodEdDSA),
		"alg none":       k1.sign(t, valid("c"), jwt.SigningMethodNone),
		"unknown key":    newKey("k9").sign(t, valid("c"), jwt.SigningMethodEdDSA),
		"forged with k1 kid": func() string {
			attacker := newKey("k1")
			return attacker.sign(t, valid("c"), jwt.SigningMethodEdDSA)
		}(),
		"garbage": "a.b.c",
	}
	for name, tok := range bad {
		if _, err := v.Verify(ctx, tok); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestKeyRotationAndCaching(t *testing.T) {
	k1, k2 := newKey("k1"), newKey("k2")
	fetches := 0
	published := []JWK{k1.jwk}
	now := time.Now()
	v := NewVerifier(func(context.Context) ([]JWK, error) { fetches++; return published, nil })
	v.now = func() time.Time { return now }
	ctx := context.Background()

	for range 3 {
		if _, err := v.Verify(ctx, k1.sign(t, valid("c"), jwt.SigningMethodEdDSA)); err != nil {
			t.Fatal(err)
		}
	}
	if fetches != 1 {
		t.Fatalf("jwks must be cached, fetched %d times", fetches)
	}

	// identity-service rotated to k2: the first k2 token triggers a refetch...
	published = []JWK{k2.jwk, k1.jwk}
	now = now.Add(20 * time.Second) // past minRefetch, before the tokens expire
	if _, err := v.Verify(ctx, k2.sign(t, valid("c"), jwt.SigningMethodEdDSA)); err != nil {
		t.Fatalf("token from the rotated key: %v", err)
	}
	// ...but a flood of unknown kids does not hammer identity-service.
	before := fetches
	for range 10 {
		_, _ = v.Verify(ctx, newKey("bogus").sign(t, valid("c"), jwt.SigningMethodEdDSA))
	}
	if fetches != before {
		t.Fatalf("unknown kids refetched %d times", fetches-before)
	}
}
