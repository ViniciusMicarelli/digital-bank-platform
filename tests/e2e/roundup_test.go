//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRoundUps(t *testing.T) {
	ana := login(t, "ana-troco-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "100.00"}, nil)
	_, box := ana.call("POST", base+"/savings", map[string]string{"name": "Troco"}, idem())
	boxBase := base + "/savings/" + box["id"].(string)
	if status, b := ana.call("PUT", boxBase+"/round-up", map[string]bool{"enabled": true}, nil); status != 200 || b["round_up"] != true {
		t.Fatalf("turn on: %d %v", status, b)
	}

	_, card := ana.call("POST", base+"/cards", map[string]string{"kind": "VIRTUAL"}, idem())
	_, d := ana.call("POST", base+"/cards/"+card["id"].(string)+"/reveal", nil, ana.stepUp())
	_, a := ana.call("POST", "/v1/sandbox/card-network/authorizations", map[string]any{
		"pan": d["pan"], "expiry_month": d["expiry_month"], "expiry_year": d["expiry_year"], "cvv": d["cvv"],
		"amount": "12.30", "merchant_name": "PADARIA", "mcc": "5462", "country": "BR", "channel": "ONLINE", "function": "DEBIT",
	}, nil)
	ana.call("POST", "/v1/sandbox/card-network/authorizations/"+a["id"].(string)+"/clear", map[string]any{}, nil)

	// The round-up follows the clearing event.
	deadline := time.Now().Add(20 * time.Second)
	for {
		_, b := ana.call("GET", boxBase, nil, nil)
		if amountOf(b["balance"]) == "0.70" {
			m := b["movements"].([]any)[0].(map[string]any)
			if m["source"] != "ROUND_UP" || amountOf(m["amount"]) != "0.70" {
				t.Fatalf("movement = %v", m)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("box = %v", b)
		}
		time.Sleep(300 * time.Millisecond)
	}
	_, bal := ana.call("GET", base+"/balance", nil, nil)
	if amountOf(bal["available"]) != "87.00" {
		t.Fatalf("checking after the purchase and the round-up = %v", bal)
	}
}
