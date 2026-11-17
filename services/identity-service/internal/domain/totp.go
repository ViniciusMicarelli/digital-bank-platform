package domain

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP uses HMAC-SHA1; authenticator apps expect it
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"time"
)

// TOTP parameters (RFC 6238 defaults, what authenticator apps support).
const (
	totpPeriod = 30
	totpDigits = 6
	// Accept the previous and next step to tolerate clock drift.
	totpSkew = 1
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a random 160-bit secret, base32-encoded.
func NewTOTPSecret() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return b32.EncodeToString(b)
}

// TOTPURI builds the otpauth:// URI shown as a QR code.
func TOTPURI(secret, account string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", "Digital Bank")
	v.Set("algorithm", "SHA1")
	v.Set("digits", fmt.Sprint(totpDigits))
	v.Set("period", fmt.Sprint(totpPeriod))
	return "otpauth://totp/" + url.PathEscape("Digital Bank:"+account) + "?" + v.Encode()
}

// TOTPCode computes the code for a time step (exported for tests and tools).
func TOTPCode(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(secret)
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, n%1_000_000), nil
}

// TOTPStep is the time step of t.
func TOTPStep(t time.Time) int64 { return t.Unix() / totpPeriod }

// VerifyTOTP checks code at now (±1 step) and returns the matched step. A
// step at or before lastStep is refused, so each code works only once.
func VerifyTOTP(secret, code string, now time.Time, lastStep int64) (int64, bool) {
	if len(code) != totpDigits {
		return 0, false
	}
	current := TOTPStep(now)
	for s := current - totpSkew; s <= current+totpSkew; s++ {
		want, err := TOTPCode(secret, s)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 && s > lastStep {
			return s, true
		}
	}
	return 0, false
}
