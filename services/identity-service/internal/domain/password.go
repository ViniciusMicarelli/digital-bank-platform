// Package domain holds identity rules: password hashing, TOTP, tokens and
// login policy. It does no I/O.
package domain

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrMFARequired        = errors.New("MFA code required")
	ErrInvalidMFACode     = errors.New("invalid MFA code")
	ErrLocked             = errors.New("too many failed attempts, try again later")
	ErrInvalidRefresh     = errors.New("invalid or expired refresh token")
	ErrRefreshReused      = errors.New("refresh token reused: session revoked")
	ErrNotFound           = errors.New("not found")
)

// argon2id parameters (OWASP 2024 minimum: m=19 MiB, t=2, p=1).
const (
	argonTime    = 2
	argonMemory  = 19 * 1024
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16
)

// ValidatePassword enforces length; long passphrases beat composition rules
// (NIST SP 800-63B).
func ValidatePassword(p string) error {
	n := utf8.RuneCountInString(p)
	if n < 10 || n > 128 {
		return fmt.Errorf("%w: password must have 10 to 128 characters", ErrInvalidArgument)
	}
	return nil
}

// HashPassword returns a PHC-formatted argon2id hash.
func HashPassword(p string) string {
	salt := make([]byte, saltLen)
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(p), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	enc := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, enc(salt), enc(key))
}

// VerifyPassword checks p against a hash produced by HashPassword, in constant time.
func VerifyPassword(p, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var (
		version      int
		memory, time uint32
		threads      uint8
	)
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(p), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash is verified when an email is unknown, so a login for a missing
// account costs the same time as one with a wrong password.
var dummyHash = HashPassword("not-a-real-password-0000")

// VerifyAgainstDummy burns the same CPU as a real verification.
func VerifyAgainstDummy(p string) { VerifyPassword(p, dummyHash) }

// NormalizeEmail lower-cases and validates an email address.
func NormalizeEmail(e string) (string, error) {
	e = strings.ToLower(strings.TrimSpace(e))
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e || len(e) > 254 || !strings.Contains(e[strings.LastIndex(e, "@"):], ".") {
		return "", fmt.Errorf("%w: invalid email", ErrInvalidArgument)
	}
	return e, nil
}
