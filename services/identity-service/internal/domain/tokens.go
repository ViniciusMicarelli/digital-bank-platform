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
// amr (RFC 8176) says how the session was authenticated: "pwd", plus "otp"
// when the user has TOTP enabled (logins then require the code); sid is the
// session (the refresh token family), so a client can tell its own session.
func SignAccessToken(k SigningKey, customerID uuid.UUID, role Role, amr []string, sid uuid.UUID, now time.Time) (string, time.Time, error) {
	exp := now.Add(AccessTokenTTL)
	if role == "" {
		role = RoleCustomer
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, AccessClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    Issuer,
		Subject:   customerID.String(),
		Audience:  jwt.ClaimStrings{Audience},
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
		ID:        uuid.NewString(),
	}, Role: role, AMR: amr, SID: sid.String()})
	tok.Header["kid"] = k.Kid
	s, err := tok.SignedString(k.Private)
	return s, exp, err
}

// Step-up tokens prove a fresh re-authentication for sensitive actions
// (revealing card data). Their audience differs from access tokens, so one
// can never be used as the other.
const (
	StepUpAudience = "digital-bank-step-up"
	StepUpTTL      = 5 * time.Minute
)

// StepUpClaims carry how the customer re-authenticated (RFC 8176 "amr").
type StepUpClaims struct {
	jwt.RegisteredClaims
	AMR []string `json:"amr"`
}

// SignStepUpToken issues a short-lived token after a fresh password (+ TOTP) check.
func SignStepUpToken(k SigningKey, customerID uuid.UUID, amr []string, now time.Time) (string, time.Time, error) {
	exp := now.Add(StepUpTTL)
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, StepUpClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: Issuer, Subject: customerID.String(), Audience: jwt.ClaimStrings{StepUpAudience},
			IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp),
			ID: uuid.NewString(),
		},
		AMR: amr,
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
	Role           Role
}

// Role separates customers from bank staff.
type Role string

const (
	RoleCustomer Role = "CUSTOMER"
	// RoleAnalyst works in the back office (KYC reviews, account blocks).
	RoleAnalyst Role = "ANALYST"
)

// AccessClaims are the access token's claims: who, and with which role.
type AccessClaims struct {
	jwt.RegisteredClaims
	Role Role     `json:"role,omitempty"`
	AMR  []string `json:"amr,omitempty"`
	SID  string   `json:"sid,omitempty"`
}

// SessionAMR is how a user with these credentials signs in: TOTP enabled
// means every login carries the code. A session that enabled TOTP after
// logging in proved the code at confirmation, so it counts as well.
func (c Credentials) SessionAMR() []string {
	if c.TOTPEnabled {
		return []string{"pwd", "otp"}
	}
	return []string{"pwd"}
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
