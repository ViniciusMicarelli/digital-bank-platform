package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

func TestNormalizeKey(t *testing.T) {
	ok := []struct {
		t    KeyType
		in   string
		want string
	}{
		{KeyCPF, "529.982.247-25", "52998224725"},
		{KeyEmail, "Ana.Silva@Example.com", "ana.silva@example.com"},
		{KeyPhone, "+55 (11) 98765-4321", "+5511987654321"},
		{KeyEVP, "8F14E45F-CEEA-467A-9A36-DEDD4BEA2543", "8f14e45f-ceea-467a-9a36-dedd4bea2543"},
	}
	for _, c := range ok {
		if got, err := NormalizeKey(c.t, c.in); err != nil || got != c.want {
			t.Errorf("NormalizeKey(%s, %q) = %q, %v", c.t, c.in, got, err)
		}
	}
	bad := []struct {
		t  KeyType
		in string
	}{
		{KeyCPF, "123"}, {KeyEmail, "no-at-sign"}, {KeyEmail, "a@b"}, {KeyEmail, "Ana <ana@x.com>"},
		{KeyPhone, "11987654321"}, {KeyPhone, "+5511887654321"}, {KeyEVP, "not-a-uuid"}, {"IBAN", "x"},
	}
	for _, c := range bad {
		if _, err := NormalizeKey(c.t, c.in); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("NormalizeKey(%s, %q) err = %v", c.t, c.in, err)
		}
	}
}

func TestDetectKeyType(t *testing.T) {
	cases := map[string]KeyType{
		"52998224725":                          KeyCPF,
		"ana@example.com":                      KeyEmail,
		"+5511987654321":                       KeyPhone,
		"8f14e45f-ceea-467a-9a36-dedd4bea2543": KeyEVP,
	}
	for in, want := range cases {
		if got, _, err := DetectKeyType(in); err != nil || got != want {
			t.Errorf("DetectKeyType(%q) = %s, %v", in, got, err)
		}
	}
}

func TestEndToEndID(t *testing.T) {
	id := NewEndToEndID(OurISPB, time.Date(2026, 11, 17, 13, 45, 0, 0, time.UTC))
	if len(id) != 32 || id[:21] != "E99999999202611171345" || !ValidEndToEndID(id) {
		t.Fatalf("e2e id = %s", id)
	}
	if ValidEndToEndID("E123") {
		t.Error("short id must be invalid")
	}
}

func TestNewOutgoing(t *testing.T) {
	from := uuid.New()
	brl := money.MustParse("10", money.BRL)
	if _, err := NewOutgoing("k", from, "x", Party{}, from, brl, "", time.Now()); !errors.Is(err, ErrInvalidArgument) {
		t.Error("sending to the same account must fail")
	}
	if _, err := NewOutgoing("k", from, "x", Party{}, uuid.Nil, money.MustParse("10", money.USD), "", time.Now()); !errors.Is(err, ErrInvalidArgument) {
		t.Error("non-BRL must fail")
	}
	tr, err := NewOutgoing("k", from, "x", Party{}, uuid.Nil, brl, "", time.Now())
	if err != nil || tr.Status != StatusCreated || tr.Internal() {
		t.Fatalf("transfer = %+v, %v", tr, err)
	}
}
