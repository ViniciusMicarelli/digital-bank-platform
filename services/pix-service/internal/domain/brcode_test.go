package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
)

func TestCRC16CCITTFalse(t *testing.T) {
	if got := CRC16("123456789"); got != 0x29B1 {
		t.Fatalf("CRC16 check value = %04X, want 29B1", got)
	}
}

func TestBRCodeRoundTrip(t *testing.T) {
	amount := money.MustParse("123.45", money.BRL)
	in := BRCode{
		Key: "ana@example.com", Amount: &amount, MerchantName: "Ana Conceição Souza da Silva Oliveira",
		MerchantCity: "São Paulo", TxID: "PEDIDO123", Description: "Pedido #123", OneTime: true,
	}
	payload, err := in.Encode()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"000201", "010212", "0014br.gov.bcb.pix", "0115ana@example.com", "5303986", "5406123.45", "5802BR", "6009SAO PAULO", "0509PEDIDO123", "6304"} {
		if !strings.Contains(payload, want) {
			t.Errorf("payload lacks %q: %s", want, payload)
		}
	}
	if strings.ContainsAny(payload, "ãçÃÇ") {
		t.Error("accents must be stripped")
	}
	out, err := DecodeBRCode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if out.Key != in.Key || *out.Amount != amount || out.TxID != "PEDIDO123" || !out.OneTime ||
		out.MerchantName != "ANA CONCEICAO SOUZA DA SI" || out.MerchantCity != "SAO PAULO" {
		t.Fatalf("decoded = %+v", out)
	}
	if len(out.MerchantName) > 25 {
		t.Fatal("merchant name must be truncated to 25 characters")
	}
}

func TestStaticBRCodeWithoutAmount(t *testing.T) {
	payload, err := BRCode{Key: "+5511987654321", MerchantName: "Bruno", MerchantCity: "Rio"}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	c, err := DecodeBRCode(payload)
	if err != nil || c.Amount != nil || c.HasTxID() || c.OneTime {
		t.Fatalf("static code = %+v, %v", c, err)
	}
}

func TestDecodeRejects(t *testing.T) {
	good, _ := BRCode{Key: "ana@example.com", MerchantName: "Ana", MerchantCity: "SP"}.Encode()
	tampered := strings.Replace(good, "ana@example.com", "eve@example.com", 1)
	cases := map[string]string{
		"tampered key": tampered,
		"bad crc":      good[:len(good)-4] + "0000",
		"garbage":      "hello",
		"truncated":    good[:20],
	}
	for name, p := range cases {
		if _, err := DecodeBRCode(p); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := (BRCode{Key: "k", TxID: "not valid!"}).Encode(); err == nil {
		t.Error("invalid txid accepted")
	}
}

func TestDynamicBRCode(t *testing.T) {
	payload, err := BRCode{URL: "pix.digitalbank.example/qr/v2/abc123", MerchantName: "Loja", MerchantCity: "SP", OneTime: true}.Encode()
	if err != nil || !strings.Contains(payload, "2536pix.digitalbank.example/qr/v2/abc123") {
		t.Fatalf("dynamic payload = %s, %v", payload, err)
	}
	c, err := DecodeBRCode(payload)
	if err != nil || c.URL != "pix.digitalbank.example/qr/v2/abc123" || c.Key != "" {
		t.Fatalf("decoded = %+v, %v", c, err)
	}
	if _, err := (BRCode{Key: "k", URL: "u"}).Encode(); err == nil {
		t.Fatal("key and URL together must fail")
	}
}
