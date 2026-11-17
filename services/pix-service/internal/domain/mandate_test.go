package domain

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

func TestMandate(t *testing.T) {
	receiver, payer, other := uuid.New(), uuid.New(), uuid.New()
	m, err := NewMandate(receiver, "academia@x.com", "ACADEMIA", payer, money.MustParse("120", money.BRL), Monthly, "Mensalidade")
	if err != nil {
		t.Fatal(err)
	}
	today := date("2026-11-17")
	if err := m.Charge(receiver, money.MustParse("99", money.BRL), date("2026-11-20"), today); !errors.Is(err, ErrMandateNotActive) {
		t.Fatalf("charging before approval: %v", err)
	}
	if err := m.Decide(other, true); !errors.Is(err, ErrNotFound) {
		t.Fatal("only the payer approves")
	}
	if err := m.Decide(payer, true); err != nil || m.Status != MandateActive {
		t.Fatal(err)
	}
	if err := m.Charge(receiver, money.MustParse("120.01", money.BRL), date("2026-11-20"), today); !errors.Is(err, ErrChargeNotAllowed) {
		t.Fatal("above the maximum")
	}
	if err := m.Charge(receiver, money.MustParse("99", money.BRL), date("2026-11-20"), today); err != nil {
		t.Fatal(err)
	}
	if err := m.Charge(receiver, money.MustParse("99", money.BRL), date("2026-11-28"), today); !errors.Is(err, ErrChargeNotAllowed) {
		t.Fatal("two charges in one month")
	}
	if err := m.Charge(receiver, money.MustParse("99", money.BRL), date("2026-12-05"), today); err != nil {
		t.Fatalf("next month: %v", err)
	}
	if err := m.Charge(payer, money.MustParse("1", money.BRL), date("2027-01-05"), today); !errors.Is(err, ErrNotFound) {
		t.Fatal("only the receiver charges")
	}
	if err := m.Revoke(payer); err != nil || m.Status != MandateRevoked {
		t.Fatal(err)
	}
	if err := m.Charge(receiver, money.MustParse("1", money.BRL), date("2027-01-05"), today); !errors.Is(err, ErrMandateNotActive) {
		t.Fatal("charging a revoked mandate")
	}
	w := Mandate{Frequency: Weekly}
	if w.Period(date("2026-11-17")) != "2026-W47" {
		t.Fatal(w.Period(date("2026-11-17")))
	}
}
