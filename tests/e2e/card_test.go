//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDebitCard(t *testing.T) {
	ana := login(t, "ana-card-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	accID := acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", "/v1/accounts/"+accID+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "200.00"}, nil)

	status, card := ana.call("POST", "/v1/accounts/"+accID+"/cards", map[string]string{"kind": "VIRTUAL"}, idem())
	if status != 201 || card["status"] != "ACTIVE" || card["holder_name"] != "ANA SOUZA" {
		t.Fatalf("issue: %d %v", status, card)
	}
	base := "/v1/accounts/" + accID + "/cards/" + card["id"].(string)
	if status, body := ana.call("POST", base+"/reveal", nil, nil); status != 403 || body["error"].(map[string]any)["code"] != "STEP_UP_REQUIRED" {
		t.Fatalf("reveal without step-up: %d %v", status, body)
	}
	_, d := ana.call("POST", base+"/reveal", nil, ana.stepUp())
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

func TestCardTokens(t *testing.T) {
	ana := login(t, "ana-token-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	accID := acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", "/v1/accounts/"+accID+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "100.00"}, nil)
	_, card := ana.call("POST", "/v1/accounts/"+accID+"/cards", map[string]string{"kind": "VIRTUAL"}, idem())
	base := "/v1/accounts/" + accID + "/cards/" + card["id"].(string)

	wallet := map[string]string{"wallet": "APPLE_PAY", "device_name": "iPhone da Ana"}
	if status, _ := ana.call("POST", base+"/tokens", wallet, nil); status != 403 {
		t.Fatalf("tokenize without step-up: %d", status)
	}
	status, tok := ana.call("POST", base+"/tokens", wallet, ana.stepUp())
	tokenPAN, _ := tok["token_pan"].(string)
	if status != 201 || tok["status"] != "ACTIVE" || len(tokenPAN) != 16 || tokenPAN[:6] != "999998" {
		t.Fatalf("tokenize: %d %v", status, tok)
	}
	pay := func() map[string]any {
		_, a := ana.call("POST", "/v1/sandbox/card-network/authorizations", map[string]any{
			"pan": tokenPAN, "expiry_month": card["expiry_month"], "expiry_year": card["expiry_year"],
			"amount": "9.90", "merchant_name": "CAFE DO PONTO", "mcc": "5814", "country": "BR", "channel": "CONTACTLESS",
		}, nil)
		return a
	}
	if a := pay(); a["status"] != "APPROVED" || a["token_id"] != tok["id"] || a["card_id"] != card["id"] {
		t.Fatalf("wallet purchase: %v", a)
	}
	tokenPath := base + "/tokens/" + tok["id"].(string)
	if _, s := ana.call("POST", tokenPath+"/suspend", nil, nil); s["status"] != "SUSPENDED" {
		t.Fatalf("suspend: %v", s)
	}
	if a := pay(); a["response_code"] != "62" {
		t.Fatalf("suspended token: %v", a)
	}
	if _, l := ana.call("GET", base+"/tokens", nil, nil); len(l["tokens"].([]any)) != 1 {
		t.Fatalf("tokens = %v", l)
	}
	ana.call("DELETE", tokenPath, nil, nil)
	if _, l := ana.call("GET", base+"/tokens", nil, nil); len(l["tokens"].([]any)) != 0 {
		t.Fatalf("deleted token still listed: %v", l)
	}

	bruno := login(t, "bruno-token-"+uuid.NewString()[:8])
	if status, _ := bruno.call("GET", base+"/tokens", nil, nil); status != 404 {
		t.Fatalf("another customer listing tokens: %d", status)
	}
}

func TestTemporaryCard(t *testing.T) {
	ana := login(t, "ana-temporario-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "100.00"}, nil)
	status, card := ana.call("POST", base+"/cards", map[string]string{"kind": "TEMPORARY"}, idem())
	if status != 201 || card["kind"] != "TEMPORARY" || card["valid_until"] == nil {
		t.Fatalf("issue: %d %v", status, card)
	}
	_, d := ana.call("POST", base+"/cards/"+card["id"].(string)+"/reveal", nil, ana.stepUp())
	buy := func(amount string) map[string]any {
		_, a := ana.call("POST", "/v1/sandbox/card-network/authorizations", map[string]any{
			"pan": d["pan"], "expiry_month": d["expiry_month"], "expiry_year": d["expiry_year"], "cvv": d["cvv"],
			"amount": amount, "merchant_name": "LOJA ONLINE", "mcc": "5999", "country": "BR", "channel": "ONLINE",
		}, nil)
		return a
	}
	if a := buy("25.00"); a["status"] != "APPROVED" {
		t.Fatalf("first purchase: %v", a)
	}
	if a := buy("1.00"); a["status"] != "DECLINED" {
		t.Fatalf("second purchase: %v", a)
	}
	_, cards := ana.call("GET", base+"/cards", nil, nil)
	c := cards["cards"].([]any)[0].(map[string]any)
	if c["status"] != "CANCELLED" || c["status_reason"] != "USED" {
		t.Fatalf("card after use = %v", c)
	}
}
