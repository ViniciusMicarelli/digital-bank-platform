//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func errorCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func TestQRCodeAndRefund(t *testing.T) {
	ana := login(t, "ana-qr-"+uuid.NewString()[:8])
	bruno := login(t, "bruno-qr-"+uuid.NewString()[:8])
	_, a := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza", "document": "529.982.247-25"}, idem())
	_, b := bruno.call("POST", "/v1/accounts", map[string]string{"holder_name": "Bruno Lima", "document": "111.444.777-35"}, idem())
	anaID, brunoID := a["id"].(string), b["id"].(string)

	anaKey := "ana." + uuid.NewString()[:8] + "@example.com"
	brunoKey := "bruno." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", "/v1/accounts/"+anaID+"/pix/keys", map[string]string{"type": "EMAIL", "value": anaKey}, nil)
	bruno.call("POST", "/v1/accounts/"+brunoID+"/pix/keys", map[string]string{"type": "EMAIL", "value": brunoKey}, nil)

	// Ana receives R$ 100 from another bank.
	status, in := ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": anaKey, "amount": "100.00", "payer_name": "Loja X"}, nil)
	if status != 201 {
		t.Fatalf("pix-in: %d %v", status, in)
	}

	t.Run("one-time QR code", func(t *testing.T) {
		status, qr := bruno.call("POST", "/v1/accounts/"+brunoID+"/pix/qr-codes", map[string]any{
			"key": brunoKey, "amount": "25.00", "description": "Pedido 42", "one_time": true, "expires_in_seconds": 600,
		}, nil)
		if status != 201 {
			t.Fatalf("create: %d %v", status, qr)
		}
		payload := qr["payload"].(string)

		status, dec := ana.call("POST", "/v1/pix/qr-codes/decode", map[string]string{"payload": payload}, nil)
		if status != 200 || dec["receiver"].(map[string]any)["name"] != "Bruno Lima" || dec["issued_here"] != true ||
			amountOf(dec["amount"]) != "25.00" {
			t.Fatalf("decode: %d %v", status, dec)
		}
		if status, paid := ana.call("POST", "/v1/accounts/"+anaID+"/pix/qr-payments", map[string]string{"payload": payload}, idem()); status != 201 || paid["qr_txid"] != qr["txid"] {
			t.Fatalf("pay: %d %v", status, paid)
		}
		if status, again := ana.call("POST", "/v1/accounts/"+anaID+"/pix/qr-payments", map[string]string{"payload": payload}, idem()); status != 422 || errorCode(again) != "QR_CODE_UNAVAILABLE" {
			t.Fatalf("paying twice: %d %v", status, again)
		}
		tampered := payload[:len(payload)-5] + "0" + payload[len(payload)-4:]
		if status, _ := ana.call("POST", "/v1/pix/qr-codes/decode", map[string]string{"payload": tampered}, nil); status != 400 {
			t.Fatalf("tampered payload: %d", status)
		}
	})

	t.Run("refund a received Pix", func(t *testing.T) {
		path := "/v1/accounts/" + anaID + "/pix/transfers/" + in["id"].(string) + "/refunds"
		status, r := ana.call("POST", path, map[string]string{"amount": "40.00", "reason": "Produto devolvido"}, idem())
		if status != 201 || r["refund_of"] != in["id"] || r["counterparty"].(map[string]any)["name"] != "Loja X" {
			t.Fatalf("refund: %d %v", status, r)
		}
		if status, body := ana.call("POST", path, map[string]string{"amount": "60.01"}, idem()); status != 422 || errorCode(body) != "REFUND_TOO_LARGE" {
			t.Fatalf("over-refund: %d %v", status, body)
		}
		// Bruno cannot refund a Pix that Ana received.
		if status, _ := bruno.call("POST", "/v1/accounts/"+brunoID+"/pix/transfers/"+in["id"].(string)+"/refunds", map[string]string{"amount": "1.00"}, idem()); status != 404 {
			t.Fatalf("refund by another customer: %d", status)
		}
	})

	_, bal := ana.call("GET", "/v1/accounts/"+anaID+"/balance", nil, nil)
	if amountOf(bal["available"]) != "35.00" { // 100 - 25 (QR) - 40 (refund)
		t.Fatalf("ana balance = %v", bal)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		_, st := ana.call("GET", "/v1/accounts/"+anaID+"/statement", nil, nil)
		items, _ := st["items"].([]any)
		if len(items) == 3 && items[0].(map[string]any)["type"] == "PIX_REFUND" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("statement = %v", st)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
