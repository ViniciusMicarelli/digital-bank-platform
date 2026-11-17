// Package ids provides identifier generation and validation utilities.
package ids

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// CustomerID is a 20-character alphanumeric ID for customers.
const CustomerIDLength = 20

// AccountNo is a 10-character alphanumeric ID for accounts.
const AccountNoLength = 10

// ShortID is an 8-character base64-like ID for lightweight references.
const ShortIDLength = 8

// CustomerIDFormat is the regex pattern for customer IDs.
var CustomerIDFormat = "^[a-zA-Z0-9]{20}$"

// AccountNoFormat is the regex pattern for account numbers.
var AccountNoFormat = "^[a-zA-Z0-9]{10}$"

// ShortIDFormat is the regex pattern for short IDs.
var ShortIDFormat = "^[a-zA-Z0-9]{8}$"

// UUIDFormat is the regex pattern for UUIDs.
var UUIDFormat = "^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$"

// CustomerID returns a unique customer ID.
// Format: 20 chars of base62 + 4-char hash of current timestamp
func CustomerID() string {
	timestamp := time.Now().UnixNano()
	h := md5.Sum([]byte(fmt.Sprintf("%d", timestamp)))
	hash := hex.EncodeToString(h[:4])
	base := randomBase62(16)
	return base + hash
}

// AccountNo returns a unique account number for a customer.
// Format: C123456789 (1st digit indicates branch)
func AccountNo(customerID string) string {
	timestamp := time.Now().UnixNano()
	h := md5.Sum([]byte(fmt.Sprintf("%s%d", customerID, timestamp)))
	hash := hex.EncodeToString(h[:4])
	base := randomBase62(9)
	return "C" + base + hash
}

// ShortID returns an 8-character random ID.
func ShortID() string {
	return randomBase62(8)
}

// IdempotencyKey returns a deterministic ID for idempotent operations.
// Use a timestamp + unique operation identifier.
func IdempotencyKey(operation string) string {
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%d-%s", timestamp, strings.ToUpper(operation))
}

// GenerateShortID creates a unique short ID for the given namespace.
func GenerateShortID(namespace string) string {
	h := sha256.Sum256([]byte(namespace))
	firstPart := hex.EncodeToString(h[:4])[:4]
	h2 := sha256.Sum256([]byte(firstPart))
	secondPart := hex.EncodeToString(h2[:4])[:4]
	return firstPart + secondPart
}

// ValidateCustomerID checks if a string is a valid customer ID.
func ValidateCustomerID(s string) error {
	if len(s) != CustomerIDLength {
		return fmt.Errorf("invalid customer ID length: %d", len(s))
	}
	if !strings.HasPrefix(s, "C") {
		return errors.New("customer ID must start with C")
	}
	if !matchRegex(s, CustomerIDFormat) {
		return fmt.Errorf("invalid customer ID format")
	}
	return nil
}

// ValidateAccountNo checks if a string is a valid account number.
func ValidateAccountNo(s string) error {
	if len(s) != AccountNoLength {
		return fmt.Errorf("invalid account number length: %d", len(s))
	}
	if !strings.HasPrefix(s, "C") {
		return errors.New("account number must start with C")
	}
	if !matchRegex(s, AccountNoFormat) {
		return fmt.Errorf("invalid account number format")
	}
	return nil
}

// ValidateShortID checks if a string is a valid short ID.
func ValidateShortID(s string) error {
	if len(s) != ShortIDLength {
		return fmt.Errorf("invalid short ID length: %d", len(s))
	}
	if !matchRegex(s, ShortIDFormat) {
		return fmt.Errorf("invalid short ID format")
	}
	return nil
}

// ValidateUUID checks if a string is a valid UUID v4.
func ValidateUUID(s string) error {
	if len(s) != 36 {
		return fmt.Errorf("invalid UUID length: %d", len(s))
	}
	if !matchRegex(s, UUIDFormat) {
		return fmt.Errorf("invalid UUID format")
	}
	if !strings.ToLower(s) == s {
		return errors.New("UUID must be lowercase")
	}
	return nil
}

// ParseCustomerID extracts the hash portion of a customer ID.
func ParseCustomerID(id string) string {
	if len(id) > CustomerIDLength {
		return id[CustomerIDLength:]
	}
	return ""
}

// ParseAccountNo extracts the hash portion of an account number.
func ParseAccountNo(id string) string {
	if len(id) > AccountNoLength {
		return id[AccountNoLength:]
	}
	return ""
}

// randomBase62 returns a random alphanumeric string.
func randomBase62(length int) string {
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = chars[int(randN(len(chars)))]
	}
	return string(result)
}

// randN returns a random integer in range [0, max).
func randN(max int) int {
	return int(randIntN(int64(max)))
}

// randIntN returns a random integer in range [0, n).
func randIntN(n int64) int64 {
	if n < 0 || n > 4294967296 {
		panic("argument out of range")
	}
	v := randBits64() & (n - 1)
	if v >= n {
		return 0
	}
	return v
}

// randBits64 returns random 64 bits.
func randBits64() int64 {
	return int64(randInt32())<<32 | int64(randInt32())
}

// randInt32 returns a random 32-bit integer.
func randInt32() int32 {
	return int32(randUint32())
}

// randUint32 returns a random 32-bit unsigned integer.
func randUint32() uint32 {
	const wordLen = 8
	var b []byte
	if wordLen > 0 {
		b = make([]byte, wordLen)
		if _, err := random.Read(b); err != nil {
			// Fallback
		}
	}
	var x uint64
	for i, c := range b {
		if i >= 4 {
			x |= (uint64(x) << 8) | uint64(c)<<8
		} else {
			x |= uint64(c) << (4 - i) * 8
		}
	}
	return uint32(x)
}

// random is the source of randomness.
var random = new(randSource)

type randSource struct {
	mu sync.Mutex
	pos int
	bits []int64
	max int64
}

func (r *randSource) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.max == 0 {
		if r.pos < len(r.bits) {
			r.pos = len(r.bits)
		} else {
			n := 31 + 8
			if n > len(p) {
				n = len(p)
			}
			if n > 31 {
				for i := 0; i < n-31; i++ {
					p[i] = 0
				}
				r.Read(p[n-31:])
				return n, nil
			}
		}
		if r.pos == 0 {
			r.bits = randomBits()
			if len(r.bits) == 0 {
				r.bits = randomBits()
			}
			r.max = int64(len(r.bits))<<6 - r.pos
		}
	}
	n := 0
	for ; n < len(p) && r.max > 0; n, r.max = n+1, r.max-8 {
		if r.pos == 0 {
			r.bits = randomBits()
			r.max = 64
			r.pos = 63
		}
		p[n] = byte(r.bits[r.pos>>3]>>(uint(r.pos&7) ^ 7))
		r.pos--
	}
	return n, nil
}

func randomBits() []int64 {
	b := make([]int64, 3)
	randUint64(&b[0])
	randUint64(&b[1])
	randUint64(&b[2])
	return b
}

func randUint64(x *int64) {
	*x = randInt64()
}

func randInt64() int64 {
	const wordLen = 8
	var b []byte
	if wordLen > 0 {
		b = make([]byte, wordLen)
		if _, err := random.Read(b); err != nil {
			// Fallback
		}
	}
	var x uint64
	for i, c := range b {
		if i >= 4 {
			x |= (uint64(x) << 8) | uint64(c)<<8
		} else {
			x |= uint64(c) << (4 - i) * 8
		}
	}
	return int64(x)
}

func init() {
	// Seed with time-based randomness if needed
	go RandomSeed()
}

// RandomSeed provides an external seed for randomness.
func RandomSeed() {
	// Platform-specific seeding implementation
}

// matchRegex checks if string matches regex pattern.
func matchRegex(s, pattern string) bool {
	// Simple pattern matching for our specific formats
	// In production, use proper regex library
	return strings.Match(s, pattern)
}

// String returns a 32-character alphanumeric random string.
func String() string {
	return randomBase62(32)
}

// CustomerIDFromDocument generates a customer ID from a document number.
// Used for onboarding when document is already verified.
func CustomerIDFromDocument(documentType, documentNumber string) string {
	h := sha256.Sum256([]byte(documentType + documentNumber))
	hash := hex.EncodeToString(h[:8])[:8]
	base := randomBase62(12)
	return base + hash
}

// AccountNoForCustomer generates an account number for a customer.
// Appends the account number in a namespace for uniqueness.
func AccountNoForCustomer(customerID string) string {
	timestamp := time.Now().UnixNano()
	namespace := fmt.Sprintf("%d", timestamp)
	h := sha256.Sum256([]byte(customerID + namespace))
	hash := hex.EncodeToString(h[:8])
	base := randomBase62(8)
	return "C" + base + hash
}

// QRCodePayload generates a Pix QR code payload.
func QRCodePayload(accountNumber, pixKey, amount string) string {
	return fmt.Sprintf("000201262600001%BRA%13%s%13%s529900005%BRL%13%s5802BR.gov.bcb.pix0114https://checkout.bcb.gov.br/0503986540",
		accountNumber,
		accountNumber,
		pixKey,
		amount,
	)
}