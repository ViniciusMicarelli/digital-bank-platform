//go:build e2e

package e2e

import (
	"testing"

	"github.com/google/uuid"
)

func TestCardMonthlyLimit(t *testing.T) {
	ana := login(t, "ana-limitecartao-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "200.00"}, nil)
	_, card := ana.call("POST", base+"/cards", map[string]string{"kind": "VIRTUAL"}, idem())
	cardBase := base + "/cards/" + card["id"].(string)
	_, d := ana.call("POST", cardBase+"/reveal", nil, ana.stepUp())
	buy := func(amount string) map[string]any {
		_, a := ana.call("POST", "/v1/sandbox/card-network/authorizations", map[string]any{
			"pan": d["pan"], "expiry_month": d["expiry_month"], "expiry_year": d["expiry_year"], "cvv": d["cvv"],
			"amount": amount, "merchant_name": "STREAMING", "mcc": "4899", "country": "BR", "channel": "ONLINE", "function": "DEBIT",
		}, nil)
		return a
	}

	if status, body := ana.call("PUT", cardBase+"/monthly-limit", map[string]string{"amount": "50.50"}, nil); status != 400 {
		t.Fatalf("not whole reais: %d %v", status, body)
	}
	status, c := ana.call("PUT", cardBase+"/monthly-limit", map[string]string{"amount": "50.00"}, nil)
	if ml, _ := c["monthly_limit"].(map[string]any); status != 200 || amountOf(ml["amount"]) != "50.00" || amountOf(ml["used"]) != "0.00" {
		t.Fatalf("set: %d %v", status, c)
	}
	if a := buy("30.00"); a["response_code"] != "00" {
		t.Fatalf("within the limit: %v", a)
	}
	if a := buy("25.00"); a["response_code"] != "61" {
		t.Fatalf("over the limit: %v", a)
	}
	_, cards := ana.call("GET", base+"/cards", nil, nil)
	for _, x := range cards["cards"].([]any) {
		if x.(map[string]any)["id"] == card["id"] {
			if used := amountOf(x.(map[string]any)["monthly_limit"].(map[string]any)["used"]); used != "30.00" {
				t.Fatalf("used = %s", used)
			}
		}
	}
	if status, c := ana.call("DELETE", cardBase+"/monthly-limit", nil, nil); status != 200 || c["monthly_limit"] != nil {
		t.Fatalf("remove: %d %v", status, c)
	}
	if a := buy("25.00"); a["response_code"] != "00" {
		t.Fatalf("without a limit: %v", a)
	}
}
