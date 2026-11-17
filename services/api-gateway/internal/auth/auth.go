// Package auth issues and verifies the short-lived access tokens (JWT, HS256)
// that identify a customer at the edge. The subject is the customer id.
//
// identity-service (login, MFA, refresh tokens, key rotation) is not built
// yet; until then the gateway can issue tokens itself in development mode.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const issuer = "digital-bank"

var ErrInvalidToken = errors.New("invalid or expired token")

type Authenticator struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// New requires a secret of at least 32 bytes.
func New(secret string, ttl time.Duration) (*Authenticator, error) {
	if len(secret) < 32 {
		return nil, errors.New("auth: JWT secret must be at least 32 bytes")
	}
	return &Authenticator{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

// Issue returns a signed token for a customer and its expiry.
func (a *Authenticator) Issue(customerID string) (string, time.Time, error) {
	if customerID == "" {
		return "", time.Time{}, errors.New("auth: customer id is required")
	}
	now := a.now()
	exp := now.Add(a.ttl)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   customerID,
		Issuer:    issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	})
	signed, err := tok.SignedString(a.secret)
	return signed, exp, err
}

// Verify checks signature, algorithm, issuer and expiry, and returns the customer id.
func (a *Authenticator) Verify(token string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return a.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(a.now),
	)
	if err != nil || claims.Subject == "" {
		return "", fmt.Errorf("%w", ErrInvalidToken)
	}
	return claims.Subject, nil
}
