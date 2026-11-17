package domain

import (
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
