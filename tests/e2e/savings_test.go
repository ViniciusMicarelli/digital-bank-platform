//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSavingsBoxes(t *testing.T) {
	ana := login(t, "ana-caixinha-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "1000.00"}, nil)
	available := func() string {
		_, bal := ana.call("GET", base+"/balance", nil, nil)
		return amountOf(bal["available"])
	}

	create := idem()
	status, box := ana.call("POST", base+"/savings", map[string]string{"name": "Viagem", "goal": "3000.00"}, create)
	if status != 201 || box["name"] != "Viagem" || amountOf(box["goal"]) != "3000.00" || amountOf(box["balance"]) != "0.00" {
		t.Fatalf("create: %d %v", status, box)
	}
	if _, again := ana.call("POST", base+"/savings", map[string]string{"name": "Viagem", "goal": "3000.00"}, create); again["id"] != box["id"] {
		t.Fatal("same Idempotency-Key, same box")
	}
	boxBase := base + "/savings/" + box["id"].(string)

	// Deposits do not count against transfer limits and are idempotent.
	dep := idem()
	for range 2 {
		if status, r := ana.call("POST", boxBase+"/deposits", map[string]string{"amount": "600.00"}, dep); status != 200 || amountOf(r["box"].(map[string]any)["balance"]) != "600.00" {
			t.Fatalf("deposit: %d %v", status, r)
		}
	}
	if got := available(); got != "400.00" {
		t.Fatalf("checking after the deposit = %s", got)
	}
	if status, body := ana.call("POST", boxBase+"/deposits", map[string]string{"amount": "400.01"}, idem()); status != 422 || errCode(body) != "INSUFFICIENT_FUNDS" {
		t.Fatalf("deposit more than the balance: %d %v", status, body)
	}
	if status, body := ana.call("POST", boxBase+"/withdrawals", map[string]string{"amount": "600.01"}, idem()); status != 422 || errCode(body) != "INSUFFICIENT_FUNDS" {
		t.Fatalf("withdraw more than the box: %d %v", status, body)
	}
	if _, r := ana.call("POST", boxBase+"/withdrawals", map[string]string{"amount": "100.00"}, idem()); amountOf(r["box"].(map[string]any)["balance"]) != "500.00" {
		t.Fatalf("withdraw: %v", r)
	}

	// A simulated year of yield (10.65%): about R$ 53 on R$ 500.
	_, y := ana.call("POST", "/v1/sandbox/accounts/"+acc["id"].(string)+"/savings/"+box["id"].(string)+"/yield", map[string]int{"days": 365}, nil)
	if amountOf(y["movement"].(map[string]any)["amount"]) != "53.24" {
		t.Fatalf("a year of yield: %v", y)
	}
	_, list := ana.call("GET", base+"/savings", nil, nil)
	if amountOf(list["total"]) != "553.24" || amountOf(list["yielded"]) != "53.24" {
		t.Fatalf("boxes = %v", list)
	}
	_, detail := ana.call("GET", boxBase, nil, nil)
	if n := len(detail["movements"].([]any)); n != 3 {
		t.Fatalf("movements = %v", detail["movements"])
	}

	// Another customer cannot see the box.
	bruno := login(t, "bruno-caixinha-"+uuid.NewString()[:8])
	if status, _ := bruno.call("GET", boxBase, nil, nil); status != 404 {
		t.Fatalf("someone else's box: %d", status)
	}

	// Deposits and withdrawals are in the statement (asynchronous).
	deadline := time.Now().Add(15 * time.Second)
	for {
		_, st := ana.call("GET", base+"/statement", nil, nil)
		var in, out bool
		for _, it := range st["items"].([]any) {
			i := it.(map[string]any)
			in = in || i["type"] == "SAVINGS_WITHDRAWAL" && i["direction"] == "CREDIT"
			out = out || i["type"] == "SAVINGS_DEPOSIT" && i["direction"] == "DEBIT" && i["counterparty_name"] == "Viagem"
		}
		if in && out {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("statement = %v", st)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Closing brings everything back.
	if status, closed := ana.call("DELETE", boxBase, nil, idem()); status != 200 || closed["status"] != "CLOSED" {
		t.Fatalf("close: %d %v", status, closed)
	}
	if got := available(); got != "1053.24" {
		t.Fatalf("checking after closing = %s", got)
	}
}
