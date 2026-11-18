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
	"slices"
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
	p, err := v.VerifyAccess(ctx, token)
	return p.Subject, err
}

// RoleAnalyst marks back-office staff in the access token's role claim.
const RoleAnalyst = "ANALYST"

// Principal is who an access token belongs to.
type Principal struct {
	Subject string
	// "CUSTOMER" or "ANALYST".
	Role string
	// MFA is true when the session was opened with a second factor
	// (the token's amr has "otp").
	MFA bool
}

// VerifyAccess returns the principal of a valid access token.
func (v *Verifier) VerifyAccess(ctx context.Context, token string) (Principal, error) {
	claims := &accessClaims{}
	if err := v.parse(ctx, token, Audience, claims); err != nil || claims.Subject == "" {
		return Principal{}, ErrInvalidToken
	}
	p := Principal{Subject: claims.Subject, Role: claims.Role, MFA: slices.Contains(claims.AMR, "otp")}
	if p.Role == "" {
		p.Role = "CUSTOMER"
	}
	return p, nil
}

type accessClaims struct {
	jwt.RegisteredClaims
	Role string   `json:"role"`
	AMR  []string `json:"amr"`
}

// VerifyStepUp returns the customer id of a valid step-up token (a fresh
// re-authentication, required for sensitive actions).
func (v *Verifier) VerifyStepUp(ctx context.Context, token string) (string, error) {
	return v.verify(ctx, token, StepUpAudience)
}

func (v *Verifier) verify(ctx context.Context, token, audience string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	if err := v.parse(ctx, token, audience, claims); err != nil || claims.Subject == "" {
		return "", ErrInvalidToken
	}
	return claims.Subject, nil
}

func (v *Verifier) parse(ctx context.Context, token, audience string, claims jwt.Claims) error {
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
	return err
}
