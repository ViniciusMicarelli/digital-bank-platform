//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBudgets(t *testing.T) {
	ana := login(t, "ana-orcamento-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "500.00"}, nil)

	if status, body := ana.call("PUT", base+"/budgets/pets", map[string]string{"amount": "100.00"}, nil); status != 400 || errCode(body) != "INVALID_BUDGET" {
		t.Fatalf("unknown category: %d %v", status, body)
	}
	if status, _ := ana.call("PUT", base+"/budgets/transfers", map[string]string{"amount": "100.00"}, nil); status != 204 {
		t.Fatalf("set: %d", status)
	}
	ana.call("PUT", base+"/budgets/total", map[string]string{"amount": "1000.00"}, nil)

	// 85% of the transfers budget: one alert, as a notification.
	ana.call("POST", base+"/pix/transfers", map[string]string{"key": "maria@outrobanco.com.br", "amount": "85.00"}, idem())
	deadline := time.Now().Add(20 * time.Second)
	for {
		_, list := ana.call("GET", "/v1/notifications", nil, nil)
		var found bool
		for _, n := range list["notifications"].([]any) {
			found = found || n.(map[string]any)["title"] == "80% do orçamento: Pix e transferências"
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no budget alert: %v", list)
		}
		time.Sleep(300 * time.Millisecond)
	}
	_, b := ana.call("GET", base+"/budgets", nil, nil)
	list := b["budgets"].([]any)
	if first := list[0].(map[string]any); len(list) != 2 || first["category"] != "TOTAL" || amountOf(first["spent"]) != "85.00" {
		t.Fatalf("budgets = %v", b)
	}

	if status, _ := ana.call("DELETE", base+"/budgets/transfers", nil, nil); status != 204 {
		t.Fatalf("remove: %d", status)
	}
	if _, b := ana.call("GET", base+"/budgets", nil, nil); len(b["budgets"].([]any)) != 1 {
		t.Fatalf("after removing = %v", b)
	}
}
