package money

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in    string
		minor int64
	}{
		{"0", 0},
		{"10", 1000},
		{"10.5", 1050},
		{"10.50", 1050},
		{"0.01", 1},
		{"-0.01", -1},
		{"+3.20", 320},
		{"92233720368547758.07", math.MaxInt64},
	}
	for _, tt := range tests {
		m, err := Parse(tt.in, BRL)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.in, err)
		}
		if m.Minor() != tt.minor {
			t.Errorf("Parse(%q) = %d, want %d", tt.in, m.Minor(), tt.minor)
		}
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]error{
		"":                     ErrInvalidAmount,
		"abc":                  ErrInvalidAmount,
		"1.":                   ErrInvalidAmount,
		".5":                   ErrInvalidAmount,
		"1.234":                ErrInvalidAmount, // more decimals than BRL allows
		"1,50":                 ErrInvalidAmount,
		"1e3":                  ErrInvalidAmount,
		"--1":                  ErrInvalidAmount,
		"92233720368547758.08": ErrOverflow,
	}
	for in, want := range cases {
		if _, err := Parse(in, BRL); !errors.Is(err, want) {
			t.Errorf("Parse(%q) error = %v, want %v", in, err, want)
		}
	}
	if _, err := Parse("1", "XXX"); !errors.Is(err, ErrUnsupportedCurrency) {
		t.Errorf("unsupported currency error = %v", err)
	}
}

func TestDecimalRoundTrip(t *testing.T) {
	for _, s := range []string{"0.00", "0.01", "-0.01", "10.50", "-1234.05", "92233720368547758.07"} {
		if got := MustParse(s, BRL).Decimal(); got != s {
			t.Errorf("Decimal() = %q, want %q", got, s)
		}
	}
	if got := (Money{amount: math.MinInt64, currency: BRL}).Decimal(); got != "-92233720368547758.08" {
		t.Errorf("MinInt64 Decimal() = %q", got)
	}
}

func TestAddSub(t *testing.T) {
	a, b := MustParse("10.50", BRL), MustParse("0.75", BRL)
	sum, err := a.Add(b)
	if err != nil || sum.Decimal() != "11.25" {
		t.Fatalf("Add = %v, %v", sum, err)
	}
	diff, err := b.Sub(a)
	if err != nil || diff.Decimal() != "-9.75" {
		t.Fatalf("Sub = %v, %v", diff, err)
	}
}

func TestOverflow(t *testing.T) {
	maxM, _ := New(math.MaxInt64, BRL)
	minM, _ := New(math.MinInt64, BRL)
	one, _ := New(1, BRL)

	if _, err := maxM.Add(one); !errors.Is(err, ErrOverflow) {
		t.Errorf("max+1 error = %v", err)
	}
	if _, err := minM.Sub(one); !errors.Is(err, ErrOverflow) {
		t.Errorf("min-1 error = %v", err)
	}
	if _, err := minM.Neg(); !errors.Is(err, ErrOverflow) {
		t.Errorf("-min error = %v", err)
	}
	if _, err := one.Sub(minM); !errors.Is(err, ErrOverflow) {
		t.Errorf("1-min error = %v", err)
	}
}

func TestCurrencyMismatch(t *testing.T) {
	brl, usd := MustParse("1", BRL), MustParse("1", USD)
	if _, err := brl.Add(usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Add error = %v", err)
	}
	if _, err := brl.Cmp(usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Cmp error = %v", err)
	}
	if _, err := Sum(BRL, brl, usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Sum error = %v", err)
	}
}

func TestSumAndCmp(t *testing.T) {
	total, err := Sum(BRL, MustParse("1.10", BRL), MustParse("2.20", BRL), MustParse("-0.30", BRL))
	if err != nil || total.Decimal() != "3.00" {
		t.Fatalf("Sum = %v, %v", total, err)
	}
	if c, _ := MustParse("1", BRL).Cmp(MustParse("2", BRL)); c != -1 {
		t.Errorf("Cmp = %d, want -1", c)
	}
}

func TestSplit(t *testing.T) {
	tests := []struct {
		amount string
		n      int
		want   []string
	}{
		{"10.00", 3, []string{"3.34", "3.33", "3.33"}},
		{"-10.00", 3, []string{"-3.34", "-3.33", "-3.33"}},
		{"0.01", 3, []string{"0.01", "0.00", "0.00"}},
		{"9.00", 3, []string{"3.00", "3.00", "3.00"}},
	}
	for _, tt := range tests {
		m := MustParse(tt.amount, BRL)
		parts, err := m.Split(tt.n)
		if err != nil {
			t.Fatal(err)
		}
		total, _ := Sum(BRL, parts...)
		if total != m {
			t.Errorf("Split(%s) sums to %s", tt.amount, total.Decimal())
		}
		for i, p := range parts {
			if p.Decimal() != tt.want[i] {
				t.Errorf("Split(%s)[%d] = %s, want %s", tt.amount, i, p.Decimal(), tt.want[i])
			}
		}
	}
	if _, err := MustParse("1", BRL).Split(0); err == nil {
		t.Error("Split(0) should fail")
	}
}

func TestJSON(t *testing.T) {
	m := MustParse("1234.56", BRL)
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"amount":"1234.56","currency":"BRL"}` {
		t.Errorf("Marshal = %s", b)
	}
	var back Money
	if err := json.Unmarshal(b, &back); err != nil || back != m {
		t.Errorf("Unmarshal = %v, %v", back, err)
	}
	if err := json.Unmarshal([]byte(`{"amount":"1.234","currency":"BRL"}`), &back); err == nil {
		t.Error("expected error for too many decimals")
	}
}

func TestString(t *testing.T) {
	if s := MustParse("-0.05", BRL).String(); s != "BRL -0.05" {
		t.Errorf("String() = %q", s)
	}
}
