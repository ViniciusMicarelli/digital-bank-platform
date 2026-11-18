package domain

import (
	"errors"
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestCheckDigits(t *testing.T) {
	// Worked examples from the FEBRABAN layout.
	if got := mod10("001905009"); got != "5" {
		t.Fatalf("mod10 = %s", got)
	}
	if got := barcodeDV("0019373700000001000500940144816060680935031"); got != "3" {
		t.Fatalf("mod11 = %s", got)
	}
}

func TestBoletoRoundTrip(t *testing.T) {
	for range 200 { // random free fields: cover many check digits
		roundTrip(t)
	}
}

func roundTrip(t *testing.T) {
	t.Helper()
	today := date(2026, 11, 17)
	b, err := NewBoleto("341", date(2026, 12, 1), 150_75, RandomFreeField("12345"))
	if err != nil {
		t.Fatal(err)
	}
	formatted := FormatLine(b.Line)
	p, err := ParseBoleto(formatted, today)
	if err != nil || p.Barcode != b.Barcode || p.Amount != 150_75 || !p.DueDate.Equal(date(2026, 12, 1)) || p.BankCode != "341" {
		t.Fatalf("parse(%s) = %+v %v", formatted, p, err)
	}
	if p2, err := ParseBoleto(b.Barcode, today); err != nil || p2.Line != b.Line {
		t.Fatalf("barcode = %+v %v", p2, err)
	}
	for i := range b.Line {
		// Fields 1-3 have their own modulo 10 digits: every single typo is
		// caught. Field 5 (due factor, amount) only has the general modulo 11
		// digit, which maps remainders 0, 10 and 11 to 1, so when it is 1 a
		// few typos collide (a FEBRABAN limitation, not ours).
		if i > 32 && b.Line[32] == '1' {
			continue
		}
		wrong := []byte(b.Line)
		wrong[i] = '0' + (wrong[i]-'0'+1)%10
		if _, err := ParseBoleto(string(wrong), today); err == nil {
			t.Fatalf("a typo at position %d went unnoticed in %s", i, b.Line)
		}
	}
	if _, err := ParseBoleto("8"+b.Line, today); err == nil {
		t.Fatal("48 digits")
	}
}

func TestDueFactor(t *testing.T) {
	if f := factorOf(date(2025, 2, 21)); f != 9999 {
		t.Fatalf("last factor of the first cycle = %d", f)
	}
	if f := factorOf(date(2025, 2, 22)); f != 1000 {
		t.Fatalf("restart = %d", f)
	}
	// Factor 1000 means 2000-07-03 or 2025-02-22: the one near today.
	if d := dateOfFactor(1000, date(2025, 3, 1)); !d.Equal(date(2025, 2, 22)) {
		t.Fatalf("factor 1000 = %v", d)
	}
	b, _ := NewBoleto("001", time.Time{}, 0, RandomFreeField("1"))
	if !b.DueDate.IsZero() || b.Amount != 0 {
		t.Fatalf("no due date, open amount: %+v", b)
	}
}

func TestCharges(t *testing.T) {
	due := date(2026, 11, 10) // Tuesday
	if f, i, _ := Charges(100_00, due, due); f != 0 || i != 0 {
		t.Fatal("on the due date")
	}
	f, i, err := Charges(100_00, due, date(2026, 11, 25)) // 15 days late
	if err != nil || f != 2_00 || i != 50 {
		t.Fatalf("15 days late: fine %d interest %d %v", f, i, err)
	}
	saturday := date(2026, 11, 14)
	if f, _, _ := Charges(100_00, saturday, date(2026, 11, 16)); f != 0 {
		t.Fatal("due on Saturday, paid on Monday: no charges")
	}
	if _, _, err := Charges(100_00, due, due.AddDate(0, 0, 60)); err == nil {
		t.Fatal("past the payment limit")
	}
	var r *RefusedError
	_, _, err = Charges(100_00, due, due.AddDate(0, 0, 60))
	if !errors.As(err, &r) || r.Reason != "BOLETO_EXPIRED" {
		t.Fatalf("expired: %v", err)
	}
}
