//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSpendingSummary(t *testing.T) {
	ana := login(t, "ana-gastos-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "500.00"}, nil)
	ana.call("POST", base+"/credit", map[string]any{"closing_day": 10}, idem())
	_, card := ana.call("POST", base+"/cards", map[string]string{"kind": "VIRTUAL"}, idem())
	_, d := ana.call("POST", base+"/cards/"+card["id"].(string)+"/reveal", nil, ana.stepUp())
	buy := func(amount, merchant, mcc, function string) string {
		_, a := ana.call("POST", "/v1/sandbox/card-network/authorizations", map[string]any{
			"pan": d["pan"], "expiry_month": d["expiry_month"], "expiry_year": d["expiry_year"], "cvv": d["cvv"],
			"amount": amount, "merchant_name": merchant, "mcc": mcc, "country": "BR", "channel": "ONLINE", "function": function,
		}, nil)
		ana.call("POST", "/v1/sandbox/card-network/authorizations/"+a["id"].(string)+"/clear", map[string]any{}, nil)
		return a["id"].(string)
	}
	bakery := buy("50.00", "PADARIA", "5462", "DEBIT")
	buy("200.00", "LOJA DE TV", "5732", "CREDIT")
	ana.call("POST", "/v1/sandbox/card-network/authorizations/"+bakery+"/refund", map[string]string{"amount": "20.00"}, nil)
	ana.call("POST", base+"/pix/transfers", map[string]string{"key": "maria@outrobanco.com.br", "amount": "30.00"}, idem())

	// The summary is projected from events: wait for it.
	deadline := time.Now().Add(15 * time.Second)
	for {
		_, s := ana.call("GET", base+"/spending", nil, nil)
		if amountOf(s["total"]) == "260.00" {
			cats := map[string]string{}
			for _, c := range s["categories"].([]any) {
				c := c.(map[string]any)
				cats[c["category"].(string)] = amountOf(c["amount"])
			}
			if cats["SHOPPING"] != "200.00" || cats["FOOD"] != "30.00" || cats["TRANSFERS"] != "30.00" {
				t.Fatalf("categories = %v", cats)
			}
			if m := s["merchants"].([]any)[0].(map[string]any); m["name"] != "LOJA DE TV" {
				t.Fatalf("top merchant = %v", m)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("spending = %v", s)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if status, body := ana.call("GET", base+"/spending?month=11-2026", nil, nil); status != 400 || errCode(body) != "INVALID_MONTH" {
		t.Fatalf("bad month: %d %v", status, body)
	}
}
