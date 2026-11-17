package secretbox

import (
	"bytes"
	"errors"
	"testing"
)

func TestSealOpen(t *testing.T) {
	b, err := New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	sealed := b.Seal([]byte("seed"), []byte("totp:a"))
	if bytes.Contains(sealed, []byte("seed")) {
		t.Fatal("plaintext leaked")
	}
	if pt, err := b.Open(sealed, []byte("totp:a")); err != nil || string(pt) != "seed" {
		t.Fatalf("open = %q, %v", pt, err)
	}
	if _, err := b.Open(sealed, []byte("totp:b")); !errors.Is(err, ErrDecrypt) {
		t.Fatal("moving a ciphertext to another row must fail")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := b.Open(sealed, []byte("totp:a")); !errors.Is(err, ErrDecrypt) {
		t.Fatal("tampering must fail")
	}
	other, _ := New(bytes.Repeat([]byte{8}, 32))
	if _, err := other.Open(b.Seal([]byte("x"), nil), nil); !errors.Is(err, ErrDecrypt) {
		t.Fatal("wrong key must fail")
	}
	if _, err := New([]byte("short")); err == nil {
		t.Fatal("short key must fail")
	}
}
