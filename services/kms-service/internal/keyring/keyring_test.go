package keyring

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/kms-service/migrations"
)

func ring(t *testing.T) *Keyring {
	t.Helper()
	k, err := New(pgtest.NewDB(t, migrations.FS, "."), bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestEnvelopeAndRotation(t *testing.T) {
	k := ring(t)
	ctx := context.Background()
	name := "account-service/documents"

	key, wrapped, v1, err := k.GenerateDataKey(ctx, name)
	if err != nil || v1 != 1 || len(key) != 32 {
		t.Fatalf("generate = v%d, %v", v1, err)
	}
	if bytes.Contains(wrapped, key) {
		t.Fatal("wrapped key contains the key")
	}
	got, _, err := k.Unwrap(ctx, name, wrapped)
	if err != nil || !bytes.Equal(got, key) {
		t.Fatalf("unwrap = %v", err)
	}
	if _, _, err := k.Unwrap(ctx, "card-service/vault", wrapped); err == nil {
		t.Fatal("a key wrapped for one name must not unwrap under another")
	}

	if v, err := k.Rotate(ctx, name); err != nil || v != 2 {
		t.Fatalf("rotate = v%d, %v", v, err)
	}
	// Data wrapped by v1 still opens; re-encrypting moves it to v2.
	if got, _, err := k.Unwrap(ctx, name, wrapped); err != nil || !bytes.Equal(got, key) {
		t.Fatalf("old version after rotation: %v", err)
	}
	re, v, changed, err := k.ReEncrypt(ctx, name, wrapped)
	if err != nil || v != 2 || !changed {
		t.Fatalf("re-encrypt = v%d changed=%v %v", v, changed, err)
	}
	if got, ver, _ := k.Unwrap(ctx, name, re); !bytes.Equal(got, key) || ver != 2 {
		t.Fatal("re-encrypted key differs")
	}
	if _, _, changed, _ := k.ReEncrypt(ctx, name, re); changed {
		t.Fatal("already current: nothing to change")
	}
}

func TestAuthorize(t *testing.T) {
	if Authorize("account-service", "account-service/documents") != nil {
		t.Fatal("own key refused")
	}
	for caller, name := range map[string]string{
		"api-gateway":     "account-service/documents",
		"":                "account-service/documents",
		"account-service": "account-service-evil/x",
	} {
		if !errors.Is(Authorize(caller, name), ErrDenied) {
			t.Errorf("%q using %q allowed", caller, name)
		}
	}
	if !errors.Is(Authorize("a", "no-slash"), ErrInvalidArgument) {
		t.Error("bad name accepted")
	}
}

func TestRootKeyProtectsMasterKeys(t *testing.T) {
	pool := pgtest.NewDB(t, migrations.FS, ".")
	ctx := context.Background()
	a, _ := New(pool, bytes.Repeat([]byte{1}, 32))
	_, wrapped, _, err := a.GenerateDataKey(ctx, "card-service/vault")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := New(pool, bytes.Repeat([]byte{2}, 32)) // wrong root key
	if _, _, err := b.Unwrap(ctx, "card-service/vault", wrapped); err == nil {
		t.Fatal("master keys opened with the wrong root key")
	}
}
