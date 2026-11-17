package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeCPF(t *testing.T) {
	valid := map[string]string{
		"529.982.247-25": "52998224725",
		"52998224725":    "52998224725",
		"111.444.777-35": "11144477735",
	}
	for in, want := range valid {
		got, err := NormalizeCPF(in)
		if err != nil || got != want {
			t.Errorf("NormalizeCPF(%q) = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "529.982.247-26", "111.111.111-11", "5299822472", "52998224725a", "000.000.000-00"} {
		if _, err := NormalizeCPF(in); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("NormalizeCPF(%q) err = %v", in, err)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	if got := FormatNumber(1001); got != "00001001-7" { // 1*5 + 1*2 = 7
		t.Errorf("FormatNumber(1001) = %s", got)
	}
	if len(FormatNumber(99_999_999)) != 10 {
		t.Error("number must be 8 digits + check digit")
	}
}

func TestNewAccountDeterministicID(t *testing.T) {
	a, err := NewAccount("key-1", "cust-1", "  Ana   Silva ", "529.982.247-25", "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewAccount("key-1", "cust-1", "Ana Silva", "52998224725", "BRL")
	if a.ID != b.ID || a.HolderName != "Ana Silva" || a.Currency != "BRL" || !a.SameRequest(b) {
		t.Errorf("a=%+v b=%+v", a, b)
	}
	if _, err := NewAccount("", "c", "Ana", "52998224725", ""); !errors.Is(err, ErrInvalidArgument) {
		t.Error("missing key should fail")
	}
}

func TestTransitions(t *testing.T) {
	a := Account{Status: StatusActive}
	if err := a.Unblock(); !errors.Is(err, ErrInvalidTransition) {
		t.Error("unblock active should fail")
	}
	if err := a.Block("fraud"); err != nil || a.Status != StatusBlocked {
		t.Fatal(err)
	}
	if err := a.Close("", time.Now()); !errors.Is(err, ErrInvalidTransition) {
		t.Error("close blocked should fail")
	}
	if err := a.Unblock(); err != nil || a.StatusReason != "" {
		t.Fatal(err)
	}
	if err := a.Close("customer request", time.Now()); err != nil || a.ClosedAt == nil {
		t.Fatal(err)
	}
	if err := a.Block(""); !errors.Is(err, ErrInvalidTransition) {
		t.Error("block closed should fail")
	}
}

func TestLimitsValidateAndCheck(t *testing.T) {
	if err := DefaultLimits.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := []Limits{
		{PerTransaction: 0, Daily: 10, Nightly: 1},
		{PerTransaction: 20, Daily: 10, Nightly: 1},
		{PerTransaction: 1, Daily: 10, Nightly: 11},
		{PerTransaction: 1, Daily: MaxDailyLimit + 1, Nightly: 1},
	}
	for _, l := range bad {
		if err := l.Validate(); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("Validate(%+v) = %v", l, err)
		}
	}

	l := Limits{PerTransaction: 100, Daily: 250, Nightly: 120}
	cases := []struct {
		amount int64
		used   Usage
		night  bool
		ok     bool
	}{
		{100, Usage{}, false, true},
		{101, Usage{}, false, false},                     // per transaction
		{100, Usage{Daily: 150}, false, true},            // exactly the daily limit
		{100, Usage{Daily: 151}, false, false},           // daily
		{100, Usage{Daily: 0, Nightly: 30}, true, false}, // nightly
		{90, Usage{Daily: 0, Nightly: 30}, true, true},
		{100, Usage{Nightly: 999}, false, true}, // nightly ignored during the day
	}
	for _, c := range cases {
		err := l.Check(c.amount, c.used, c.night)
		if (err == nil) != c.ok {
			t.Errorf("Check(%d, %+v, night=%v) = %v", c.amount, c.used, c.night, err)
		}
	}
}

func TestWindow(t *testing.T) {
	at := func(s string) time.Time {
		tm, err := time.ParseInLocation("2006-01-02 15:04", s, Location)
		if err != nil {
			t.Fatal(err)
		}
		return tm
	}
	day, night := Window(at("2026-03-10 14:00"))
	if day.Format("2006-01-02") != "2026-03-10" || night != nil {
		t.Errorf("afternoon: %v %v", day, night)
	}
	_, night = Window(at("2026-03-10 21:30"))
	if night == nil || night.Format("2006-01-02") != "2026-03-10" {
		t.Errorf("evening night_of = %v", night)
	}
	day, night = Window(at("2026-03-11 05:59"))
	if day.Format("2006-01-02") != "2026-03-11" || night == nil || night.Format("2006-01-02") != "2026-03-10" {
		t.Errorf("early morning: %v %v", day, night)
	}
	// 23:30 UTC is 20:30 in São Paulo: already night.
	_, night = Window(time.Date(2026, 3, 10, 23, 30, 0, 0, time.UTC))
	if night == nil {
		t.Error("UTC input must be converted to local time")
	}
}
