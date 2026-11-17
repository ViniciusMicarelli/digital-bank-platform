// Package auth verifies access tokens issued by identity-service. The gateway
// holds no signing secret: it only fetches public keys (JWKS) and checks
// Ed25519 signatures, issuer, audience and expiry.
package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Must match identity-service's domain.Issuer / domain.Audience.
const (
	Issuer         = "digital-bank-identity"
	Audience       = "digital-bank-api"
	StepUpAudience = "digital-bank-step-up"
)

var ErrInvalidToken = errors.New("invalid or expired token")

// JWK is a public key as published by identity-service.
type JWK struct{ Kid, Kty, Crv, X string }

// KeySource fetches the current JWKS.
type KeySource func(ctx context.Context) ([]JWK, error)

// Verifier checks tokens against a cached JWKS. An unknown kid (e.g. right
// after a key rotation) triggers a refetch, at most once per minRefetch.
type Verifier struct {
	source     KeySource
	maxAge     time.Duration
	minRefetch time.Duration
	now        func() time.Time

	mu        sync.Mutex
	keys      map[string]ed25519.PublicKey
	fetchedAt time.Time
}

func NewVerifier(source KeySource) *Verifier {
	return &Verifier{source: source, maxAge: 5 * time.Minute, minRefetch: 10 * time.Second, now: time.Now}
}

func (v *Verifier) key(ctx context.Context, kid string) (ed25519.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	k, ok := v.keys[kid]
	age := v.now().Sub(v.fetchedAt)
	if ok && age < v.maxAge {
		return k, nil
	}
	if !ok && v.keys != nil && age < v.minRefetch {
		return nil, fmt.Errorf("unknown key %q", kid)
	}
	jwks, err := v.source(ctx)
	if err != nil {
		if ok { // identity is down: keep using the cached key
			return k, nil
		}
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	keys := make(map[string]ed25519.PublicKey, len(jwks))
	for _, j := range jwks {
		if j.Kty != "OKP" || j.Crv != "Ed25519" {
			continue
		}
		x, err := base64.RawURLEncoding.DecodeString(j.X)
		if err != nil || len(x) != ed25519.PublicKeySize {
			continue
		}
		keys[j.Kid] = ed25519.PublicKey(x)
	}
	v.keys, v.fetchedAt = keys, v.now()
	if k, ok = keys[kid]; !ok {
		return nil, fmt.Errorf("unknown key %q", kid)
	}
	return k, nil
}

// Verify returns the customer id (subject) of a valid access token.
func (v *Verifier) Verify(ctx context.Context, token string) (string, error) {
	return v.verify(ctx, token, Audience)
}

// VerifyStepUp returns the customer id of a valid step-up token (a fresh
// re-authentication, required for sensitive actions).
func (v *Verifier) VerifyStepUp(ctx context.Context, token string) (string, error) {
	return v.verify(ctx, token, StepUpAudience)
}

func (v *Verifier) verify(ctx context.Context, token, audience string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.key(ctx, kid)
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(Issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil || claims.Subject == "" {
		return "", ErrInvalidToken
	}
	return claims.Subject, nil
}
