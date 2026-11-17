//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDebitCard(t *testing.T) {
	ana := login(t, "ana-card-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza", "document": "529.982.247-25"}, idem())
	accID := acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", "/v1/accounts/"+accID+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "200.00"}, nil)

	status, card := ana.call("POST", "/v1/accounts/"+accID+"/cards", map[string]string{"kind": "VIRTUAL"}, idem())
	if status != 201 || card["status"] != "ACTIVE" || card["holder_name"] != "ANA SOUZA" {
		t.Fatalf("issue: %d %v", status, card)
	}
	base := "/v1/accounts/" + accID + "/cards/" + card["id"].(string)
	_, d := ana.call("POST", base+"/reveal", nil, nil)
	pan := d["pan"].(string)
	if pan[len(pan)-4:] != card["last4"] {
		t.Fatalf("reveal = %v", d)
	}

	buy := func(amount, cvv, country string) map[string]any {
		_, a := ana.call("POST", "/v1/sandbox/card-network/authorizations", map[string]any{
			"pan": pan, "expiry_month": d["expiry_month"], "expiry_year": d["expiry_year"], "cvv": cvv,
			"amount": amount, "merchant_name": "PADARIA SAO JOSE", "mcc": "5462", "country": country, "channel": "ONLINE",
		}, nil)
		return a
	}
	cvv := d["cvv"].(string)

	a := buy("80.00", cvv, "BR")
	if a["status"] != "APPROVED" || a["response_code"] != "00" {
		t.Fatalf("purchase: %v", a)
	}
	_, bal := ana.call("GET", "/v1/accounts/"+accID+"/balance", nil, nil)
	if amountOf(bal["available"]) != "120.00" || amountOf(bal["pending"]) != "80.00" {
		t.Fatalf("balance after authorization = %v", bal)
	}

	wrong := "000"
	if cvv == wrong {
		wrong = "001"
	}
	if r := buy("1.00", wrong, "BR"); r["response_code"] != "82" {
		t.Fatalf("wrong CVV: %v", r)
	}
	if r := buy("1.00", cvv, "US"); r["response_code"] != "57" {
		t.Fatalf("international while disabled: %v", r)
	}
	if r := buy("500.00", cvv, "BR"); r["response_code"] != "51" {
		t.Fatalf("insufficient funds: %v", r)
	}

	// The bakery clears less than authorized (e.g. a corrected bill).
	_, cleared := ana.call("POST", "/v1/sandbox/card-network/authorizations/"+a["id"].(string)+"/clear", map[string]string{"amount": "75.50"}, nil)
	if cleared["status"] != "CLEARED" || amountOf(cleared["cleared_amount"]) != "75.50" {
		t.Fatalf("clear: %v", cleared)
	}
	_, bal = ana.call("GET", "/v1/accounts/"+accID+"/balance", nil, nil)
	if amountOf(bal["available"]) != "124.50" || amountOf(bal["pending"]) != "0.00" {
		t.Fatalf("balance after clearing = %v", bal)
	}

	v := buy("30.00", cvv, "BR")
	_, rev := ana.call("POST", "/v1/sandbox/card-network/authorizations/"+v["id"].(string)+"/reverse", nil, nil)
	if rev["status"] != "REVERSED" {
		t.Fatalf("reverse: %v", rev)
	}

	ana.call("POST", base+"/block", map[string]string{"reason": "lost"}, nil)
	if r := buy("1.00", cvv, "BR"); r["response_code"] != "62" {
		t.Fatalf("blocked card: %v", r)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		_, st := ana.call("GET", "/v1/accounts/"+accID+"/statement", nil, nil)
		items, _ := st["items"].([]any)
		if len(items) > 0 {
			top := items[0].(map[string]any)
			if top["type"] == "DEBIT_CARD_PURCHASE" && top["counterparty_name"] == "PADARIA SAO JOSE" && amountOf(top["amount"]) == "75.50" {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("statement = %v", st)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
