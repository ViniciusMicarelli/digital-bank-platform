// Package secretbox encrypts small secrets at rest (signing keys, TOTP seeds)
// with AES-256-GCM under a key-encryption key (KEK) from the environment.
//
// The associated data binds a ciphertext to where it is stored (for example
// "totp:<customer id>"): copying it to another row makes decryption fail.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

// version prefixes every ciphertext so the format (or the KEK) can rotate later.
const version byte = 1

var ErrDecrypt = errors.New("secretbox: cannot decrypt")

type Box struct{ aead cipher.AEAD }

// New takes a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secretbox: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// FromBase64 builds a Box from a base64-encoded 32-byte key (e.g. an env var).
func FromBase64(s string) (*Box, error) {
	key, err := DecodeKey(s)
	if err != nil {
		return nil, err
	}
	return New(key)
}

// Seal returns version || nonce || ciphertext.
func (b *Box) Seal(plaintext, associated []byte) []byte {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	out := append([]byte{version}, nonce...)
	return b.aead.Seal(out, nonce, plaintext, associated)
}

// Open reverses Seal; it fails if the data or the associated data changed.
func (b *Box) Open(sealed, associated []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < 1+n || sealed[0] != version {
		return nil, ErrDecrypt
	}
	pt, err := b.aead.Open(nil, sealed[1:1+n], sealed[1+n:], associated)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

// DecodeKey decodes a base64 32-byte key.
func DecodeKey(s string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("secretbox: key is not base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("secretbox: key must be 32 bytes, got %d", len(key))
	}
	return key, nil
}

// DeriveKey derives an independent subkey (HMAC-SHA256) for another purpose,
// e.g. a blind-index key, so one master key does not serve two algorithms.
func DeriveKey(master []byte, label string) []byte {
	m := hmac.New(sha256.New, master)
	m.Write([]byte(label))
	return m.Sum(nil)
}

// BlindIndex is a keyed hash of a value: equal values give equal indexes,
// so encrypted columns stay searchable, but the value cannot be recovered
// or brute-forced without the key.
func BlindIndex(key []byte, value string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(value))
	return m.Sum(nil)
}
