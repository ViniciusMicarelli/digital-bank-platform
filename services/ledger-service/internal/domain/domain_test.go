package domain

import (
	"bytes"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

func brl(s string) money.Money { return money.MustParse(s, money.BRL) }

func TestNewEntryValidation(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	tests := []struct {
		name     string
		key      string
		postings []Posting
	}{
		{"missing key", "", []Posting{{a, brl("-1")}, {b, brl("1")}}},
		{"single posting", "k", []Posting{{a, brl("1")}}},
		{"unbalanced", "k", []Posting{{a, brl("-1")}, {b, brl("0.99")}}},
		{"zero posting", "k", []Posting{{a, brl("-1")}, {b, brl("1")}, {c, brl("0")}}},
		{"duplicate account", "k", []Posting{{a, brl("-1")}, {a, brl("1")}}},
		{"nil account", "k", []Posting{{uuid.Nil, brl("-1")}, {b, brl("1")}}},
		{"mixed currencies", "k", []Posting{{a, brl("-1")}, {b, money.MustParse("1", money.USD)}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewEntry(tt.key, tt.postings, "", nil); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err = %v, want ErrInvalidArgument", err)
			}
		})
	}

	e, err := NewEntry("k", []Posting{{a, brl("-10")}, {b, brl("7.5")}, {c, brl("2.5")}}, "split", nil)
	if err != nil {
		t.Fatalf("valid three-way entry: %v", err)
	}
	if e.Metadata == nil || e.ID == uuid.Nil {
		t.Error("entry should have an id and non-nil metadata")
	}
}

func TestFingerprint(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	e1, _ := NewEntry("k", []Posting{{a, brl("-1")}, {b, brl("1")}}, "d", map[string]string{"x": "1", "y": "2"})
	e2, _ := NewEntry("k", []Posting{{b, brl("1")}, {a, brl("-1")}}, "d", map[string]string{"y": "2", "x": "1"})
	if !bytes.Equal(e1.Fingerprint(), e2.Fingerprint()) {
		t.Error("posting and metadata order must not change the fingerprint")
	}
	e3, _ := NewEntry("k", []Posting{{a, brl("-2")}, {b, brl("2")}}, "d", map[string]string{"x": "1", "y": "2"})
	if bytes.Equal(e1.Fingerprint(), e3.Fingerprint()) {
		t.Error("different amounts must change the fingerprint")
	}
}

func setup(kind AccountKind, overdraft int64, posted int64) (Account, *Balance) {
	acc, _ := NewAccount(uuid.New(), kind, money.BRL, overdraft, "")
	return acc, &Balance{AccountID: acc.ID, Posted: posted}
}

func TestApplyEnforcesFloor(t *testing.T) {
	cust, custBal := setup(KindCustomer, 0, 1000)
	internal, intBal := setup(KindInternal, 0, 0)
	accounts := map[uuid.UUID]Account{cust.ID: cust, internal.ID: internal}

	over, _ := NewEntry("k", []Posting{{cust.ID, brl("-10.01")}, {internal.ID, brl("10.01")}}, "", nil)
	balances := map[uuid.UUID]*Balance{cust.ID: ptr(*custBal), internal.ID: ptr(*intBal)}
	if err := over.Apply(accounts, balances, true); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("overdraw err = %v", err)
	}

	exact, _ := NewEntry("k", []Posting{{cust.ID, brl("-10")}, {internal.ID, brl("10")}}, "", nil)
	balances = map[uuid.UUID]*Balance{cust.ID: ptr(*custBal), internal.ID: ptr(*intBal)}
	if err := exact.Apply(accounts, balances, true); err != nil {
		t.Fatalf("exact debit: %v", err)
	}
	if balances[cust.ID].Posted != 0 || balances[internal.ID].Posted != 1000 {
		t.Errorf("balances = %+v %+v", balances[cust.ID], balances[internal.ID])
	}

	// Internal accounts can go negative (e.g. the funding account).
	fund, _ := NewEntry("k", []Posting{{internal.ID, brl("-500")}, {cust.ID, brl("500")}}, "", nil)
	balances = map[uuid.UUID]*Balance{cust.ID: ptr(*custBal), internal.ID: ptr(*intBal)}
	if err := fund.Apply(accounts, balances, true); err != nil {
		t.Fatalf("internal debit: %v", err)
	}
}

func TestApplyOverdraft(t *testing.T) {
	cust, bal := setup(KindCustomer, 5000, 0) // R$ 50 overdraft
	other, otherBal := setup(KindInternal, 0, 0)
	accounts := map[uuid.UUID]Account{cust.ID: cust, other.ID: other}
	e, _ := NewEntry("k", []Posting{{cust.ID, brl("-50")}, {other.ID, brl("50")}}, "", nil)
	if err := e.Apply(accounts, map[uuid.UUID]*Balance{cust.ID: bal, other.ID: otherBal}, true); err != nil {
		t.Fatalf("within overdraft: %v", err)
	}
	e2, _ := NewEntry("k2", []Posting{{cust.ID, brl("-0.01")}, {other.ID, brl("0.01")}}, "", nil)
	if err := e2.Apply(accounts, map[uuid.UUID]*Balance{cust.ID: bal, other.ID: otherBal}, true); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("beyond overdraft err = %v", err)
	}
}

func TestReversal(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	e, _ := NewEntry("k", []Posting{{a, brl("-3")}, {b, brl("3")}}, "pay", nil)
	r, err := e.Reversal("k-rev", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.ReversesEntryID != e.ID || r.Postings[0].Amount != brl("3") || r.Postings[1].Amount != brl("-3") {
		t.Errorf("reversal = %+v", r)
	}
	if _, err := r.Reversal("k-rev-rev", ""); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("reversing a reversal err = %v", err)
	}
}

func TestHoldLifecycle(t *testing.T) {
	cust, bal := setup(KindCustomer, 0, 10000)
	counter := uuid.New()

	if _, err := NewHold("k", cust.ID, cust.ID, brl("1"), ""); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("same account err = %v", err)
	}
	if _, err := NewHold("k", cust.ID, counter, brl("-1"), ""); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("negative hold err = %v", err)
	}

	big, _ := NewHold("k", cust.ID, counter, brl("100.01"), "")
	if err := big.Reserve(cust, ptr(*bal)); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("over-reserve err = %v", err)
	}

	h, _ := NewHold("k", cust.ID, counter, brl("60"), "pix")
	if err := h.Reserve(cust, bal); err != nil {
		t.Fatal(err)
	}
	if bal.Available() != 4000 || bal.Pending != 6000 {
		t.Fatalf("after reserve: %+v", bal)
	}

	if _, err := h.Capture(brl("60.01"), "c", bal); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("over-capture err = %v", err)
	}
	e, err := h.Capture(brl("50"), "c", bal) // partial: R$ 10 returns to available
	if err != nil {
		t.Fatal(err)
	}
	if bal.Pending != 0 || h.Status != HoldCaptured || h.CaptureEntryID != e.ID {
		t.Errorf("after capture: bal=%+v hold=%+v", bal, h)
	}
	if e.Postings[0].Amount != brl("-50") || e.Postings[1].AccountID != counter {
		t.Errorf("capture entry = %+v", e.Postings)
	}
	if err := h.Release(bal); !errors.Is(err, ErrHoldNotActive) {
		t.Errorf("release after capture err = %v", err)
	}
}

func ptr[T any](v T) *T { return &v }
