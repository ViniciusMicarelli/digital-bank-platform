package domain

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Token settings shared with the verifiers (api-gateway).
const (
	Issuer          = "digital-bank-identity"
	Audience        = "digital-bank-api"
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 30 * 24 * time.Hour
	// A retired key stays in the JWKS long enough to verify tokens it signed.
	KeyGracePeriod = AccessTokenTTL + 5*time.Minute
)

// SigningKey is an Ed25519 key pair identified by kid.
type SigningKey struct {
	Kid       string
	Private   ed25519.PrivateKey
	Public    ed25519.PublicKey
	CreatedAt time.Time
}

// NewSigningKey generates a key; its kid is a hash of the public key.
func NewSigningKey() SigningKey {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sum := sha256.Sum256(pub)
	return SigningKey{Kid: base64.RawURLEncoding.EncodeToString(sum[:12]), Private: priv, Public: pub}
}

// JWK is the public part of a key in RFC 7517 / RFC 8037 form.
type JWK struct {
	Kid, Kty, Crv, X, Alg, Use string
}

func (k SigningKey) JWK() JWK {
	return JWK{Kid: k.Kid, Kty: "OKP", Crv: "Ed25519", X: base64.RawURLEncoding.EncodeToString(k.Public), Alg: "EdDSA", Use: "sig"}
}

// SignAccessToken issues a short-lived token whose subject is the customer.
func SignAccessToken(k SigningKey, customerID uuid.UUID, now time.Time) (string, time.Time, error) {
	exp := now.Add(AccessTokenTTL)
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.RegisteredClaims{
		Issuer:    Issuer,
		Subject:   customerID.String(),
		Audience:  jwt.ClaimStrings{Audience},
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
		ID:        uuid.NewString(),
	})
	tok.Header["kid"] = k.Kid
	s, err := tok.SignedString(k.Private)
	return s, exp, err
}

// NewRefreshToken returns an opaque 256-bit token and the hash to store.
func NewRefreshToken() (token string, hash []byte) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashRefreshToken(token)
}

// HashRefreshToken is what the database stores; a leaked table cannot be replayed.
func HashRefreshToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Login policy.
const (
	MaxFailedAttempts = 5
	LockDuration      = 15 * time.Minute
)

// Credentials is a customer's login record.
type Credentials struct {
	CustomerID     uuid.UUID
	Email          string
	FullName       string
	PasswordHash   string
	TOTPSecretEnc  []byte
	TOTPEnabled    bool
	TOTPLastStep   int64
	FailedAttempts int
	LockedUntil    *time.Time
}

// Locked reports whether logins are refused at now.
func (c Credentials) Locked(now time.Time) bool {
	return c.LockedUntil != nil && now.Before(*c.LockedUntil)
}

// RegisterFailure counts a failed attempt and locks after too many.
func (c *Credentials) RegisterFailure(now time.Time) {
	c.FailedAttempts++
	if c.FailedAttempts >= MaxFailedAttempts {
		until := now.Add(LockDuration)
		c.LockedUntil, c.FailedAttempts = &until, 0
	}
}

// RegisterSuccess clears the failure counter.
func (c *Credentials) RegisterSuccess() { c.FailedAttempts, c.LockedUntil = 0, nil }
