package domain

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

func TestLuhn(t *testing.T) {
	for _, valid := range []string{"4111111111111111", "5555555555554444", "378282246310005"} {
		if !ValidPAN(valid) {
			t.Errorf("%s should be valid", valid)
		}
	}
	for _, invalid := range []string{"4111111111111112", "411111111111", "41111111111111a1"} {
		if ValidPAN(invalid) {
			t.Errorf("%s should be invalid", invalid)
		}
	}
	for range 100 {
		pan := NewPAN()
		if len(pan) != 16 || !strings.HasPrefix(pan, BIN) || !ValidPAN(pan) {
			t.Fatalf("generated PAN %s", pan)
		}
	}
}

func TestDeriveCVV(t *testing.T) {
	key := []byte("issuer-cvk")
	a := DeriveCVV(key, "9999990000000001", 12, 2030)
	if len(a) != 3 || a != DeriveCVV(key, "9999990000000001", 12, 2030) {
		t.Fatal("CVV must be 3 digits and deterministic")
	}
	if a == DeriveCVV([]byte("other-key"), "9999990000000001", 12, 2030) && a == DeriveCVV(key, "9999990000000001", 1, 2031) {
		t.Fatal("CVV must depend on the key and the expiry")
	}
}

func card(t *testing.T, now time.Time) Card {
	c, _, err := NewCard("k", uuid.New(), Physical, "ANA SOUZA", now)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func purchase(c Card, channel Channel, country, mcc, amount, cvv string) Purchase {
	return Purchase{NetworkReference: "ref", ExpiryMonth: c.ExpiryMonth, ExpiryYear: c.ExpiryYear, CVV: cvv,
		Amount: money.MustParse(amount, money.BRL), MerchantName: "LOJA", MCC: mcc, Country: country, Channel: channel}
}

func TestDecide(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	c := card(t, now)
	if c.ExpiryYear != 2031 || c.Last4 == "" {
		t.Fatalf("physical card expiry = %d", c.ExpiryYear)
	}
	cvv := "123"
	cases := []struct {
		name string
		mod  func(*Card, *Purchase)
		code string
	}{
		{"approved online", func(*Card, *Purchase) {}, CodeApproved},
		{"chip without CVV", func(_ *Card, p *Purchase) { p.Channel, p.CVV = Chip, "" }, CodeApproved},
		{"online without CVV", func(_ *Card, p *Purchase) { p.CVV = "" }, CodeWrongCVV},
		{"wrong CVV", func(_ *Card, p *Purchase) { p.CVV = "999" }, CodeWrongCVV},
		{"wrong expiry", func(_ *Card, p *Purchase) { p.ExpiryYear++ }, CodeExpiredCard},
		{"blocked", func(c *Card, _ *Purchase) { _ = c.Block("lost") }, CodeRestrictedCard},
		{"online disabled", func(c *Card, _ *Purchase) { c.Controls.Online = false }, CodeNotPermitted},
		{"international by default", func(_ *Card, p *Purchase) { p.Country = "US" }, CodeNotPermitted},
		{"international enabled", func(c *Card, p *Purchase) { p.Country, c.Controls.International = "US", true }, CodeApproved},
		{"contactless disabled", func(c *Card, p *Purchase) { p.Channel, p.CVV, c.Controls.Contactless = Contactless, "", false }, CodeNotPermitted},
		{"gambling", func(_ *Card, p *Purchase) { p.MCC = "7995" }, CodeNotPermitted},
		{"too large", func(_ *Card, p *Purchase) { p.Amount = money.MustParse("10000.01", money.BRL) }, CodeExceedsLimit},
	}
	for _, tc := range cases {
		cc := c
		p := purchase(cc, Online, "BR", "5411", "50", cvv)
		tc.mod(&cc, &p)
		if d := cc.Decide(p, cvv, now); d.Code != tc.code || d.Approve != (tc.code == CodeApproved) {
			t.Errorf("%s: %+v, want %s", tc.name, d, tc.code)
		}
	}
	if !c.Expired(time.Date(c.ExpiryYear, time.Month(c.ExpiryMonth)+1, 1, 0, 0, 0, 0, time.UTC)) || c.Expired(now) {
		t.Fatal("expiry is the end of the expiry month")
	}
}

func TestTokens(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	c := card(t, now)
	tok, pan, err := NewToken(c, ApplePay, "iPhone da Ana")
	if err != nil || !strings.HasPrefix(pan, TokenBIN) || !ValidPAN(pan) || tok.Last4 != pan[12:] {
		t.Fatalf("token = %+v %s %v", tok, pan, err)
	}
	p := purchase(c, Contactless, "BR", "5411", "30", "")
	if d := c.DecideToken(p, tok, now); !d.Approve {
		t.Fatalf("token purchase: %+v", d)
	}
	online := purchase(c, Online, "BR", "5411", "30", "")
	if d := c.DecideToken(online, tok, now); !d.Approve {
		t.Fatalf("in-app purchase with a token needs no CVV: %+v", d)
	}
	_ = tok.Transition("SUSPEND")
	if d := c.DecideToken(p, tok, now); d.Code != CodeRestrictedCard {
		t.Fatalf("suspended token: %+v", d)
	}
	_ = tok.Transition("RESUME")
	blocked := c
	_ = blocked.Block("lost")
	if d := blocked.DecideToken(p, tok, now); d.Code != CodeRestrictedCard {
		t.Fatalf("token of a blocked card: %+v", d)
	}
	_ = tok.Transition("DELETE")
	if err := tok.Transition("RESUME"); err == nil {
		t.Fatal("a deleted token cannot come back")
	}
	_ = c.Block("x")
	if _, _, err := NewToken(c, GooglePay, "Pixel"); err == nil {
		t.Fatal("tokenizing a blocked card")
	}
}

func TestDisputesAndRefunds(t *testing.T) {
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	a := Authorization{ID: uuid.New(), Status: Approved, Cleared: 0, CreatedAt: now.Add(-24 * time.Hour)}
	if _, err := NewDispute(a, DisputeNotReceived, 0, "", false, now); !refused(err, "NOT_CLEARED") {
		t.Fatalf("not cleared: %v", err)
	}
	a.Status, a.Cleared = Cleared, 100_00
	if _, err := NewDispute(a, "BORED", 0, "", false, now); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal("unknown reason")
	}
	if _, err := NewDispute(a, DisputeNotReceived, 0, "", false, a.CreatedAt.Add(DisputeWindow+time.Hour)); !refused(err, "DISPUTE_WINDOW_CLOSED") {
		t.Fatal("window")
	}
	if _, err := NewDispute(a, DisputeNotReceived, 0, "", true, now); !refused(err, "DISPUTE_ALREADY_OPEN") {
		t.Fatal("one open dispute")
	}
	d, err := NewDispute(a, DisputeWrongAmount, 0, "  cobraram a mais  ", false, now)
	if err != nil || d.Amount != 100_00 || d.Description != "cobraram a mais" {
		t.Fatalf("default amount = %+v %v", d, err)
	}

	// A refund cannot eat into an open dispute, nor go past what was cleared.
	if _, err := NewRefund(a, 1, RefundMerchant, "r1", d.Amount, now); !refused(err, "REFUND_TOO_LARGE") {
		t.Fatalf("refund under a full dispute: %v", err)
	}
	r, err := NewRefund(a, 40_00, RefundMerchant, "r1", 0, now)
	if err != nil || r.Status != RefundPending {
		t.Fatalf("refund = %+v %v", r, err)
	}
	a.Refunded = 40_00
	if a.Refundable(0) != 60_00 {
		t.Fatal(a.Refundable(0))
	}
	if _, err := NewDispute(a, DisputeWrongAmount, 70_00, "", false, now); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal("dispute more than what is left")
	}

	if err := d.Decide("a1", false, "", now); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal("a lost dispute needs a reason")
	}
	if err := d.Decide("a1", true, "", now); err != nil || d.Status != DisputeWon || d.DecidedBy != "a1" {
		t.Fatalf("won = %+v %v", d, err)
	}
	if err := d.Decide("a2", false, "x", now); !refused(err, "DISPUTE_NOT_OPEN") {
		t.Fatal("decided twice")
	}
}

func refused(err error, reason string) bool {
	r, ok := AsRefused(err)
	return ok && r.Reason == reason
}
