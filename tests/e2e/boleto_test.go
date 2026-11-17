//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBoletos(t *testing.T) {
	ana := login(t, "ana-boleto-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza", "document": "529.982.247-25"}, idem())
	accID := acc["id"].(string)
	base := "/v1/accounts/" + accID
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "500.00"}, nil)

	sp, _ := time.LoadLocation("America/Sao_Paulo")
	today := time.Now().In(sp)
	day := func(offset int) string { return today.AddDate(0, 0, offset).Format(time.DateOnly) }
	issue := func(amount, due, behavior string) string {
		status, b := ana.call("POST", "/v1/sandbox/boletos", map[string]string{
			"amount": amount, "due_date": due, "payee_name": "CIA DE ENERGIA", "behavior": behavior}, nil)
		if status != 201 {
			t.Fatalf("issue boleto: %d %v", status, b)
		}
		return b["formatted_line"].(string)
	}
	pay := func(body map[string]string) (int, map[string]any) {
		return ana.call("POST", base+"/boleto-payments", body, idem())
	}

	// Look up an overdue boleto: 2% fine plus 1% a month of interest.
	overdue := issue("100.00", day(-15), "")
	status, q := ana.call("POST", "/v1/boletos/lookup", map[string]string{"line": overdue}, nil)
	if status != 200 || q["payee"].(map[string]any)["name"] != "CIA DE ENERGIA" || amountOf(q["fine"]) != "2.00" || amountOf(q["total"]) != "102.50" {
		t.Fatalf("lookup: %d %v", status, q)
	}
	if status, p := pay(map[string]string{"line": overdue}); status != 201 || p["status"] != "COMPLETED" || amountOf(p["amount"]) != "102.50" {
		t.Fatalf("pay overdue: %d %v", status, p)
	}
	if status, body := pay(map[string]string{"line": overdue}); status != 422 || errCode(body) != "DUPLICATE_PAYMENT" {
		t.Fatalf("paying twice: %d %v", status, body)
	}
	if status, body := ana.call("POST", "/v1/boletos/lookup", map[string]string{"line": "23790.00000 00000.000000 00000.000000 1 00000000000000"}, nil); status != 400 {
		t.Fatalf("a typo in the line: %d %v", status, body)
	}

	// The payee's bank refuses: the money comes back.
	if status, body := pay(map[string]string{"line": issue("50.00", day(10), "REJECT")}); status != 422 || errCode(body) != "SETTLEMENT_REJECTED" {
		t.Fatalf("rejected: %d %v", status, body)
	}
	if _, bal := ana.call("GET", base+"/balance", nil, nil); amountOf(bal["available"]) != "397.50" {
		t.Fatalf("balance = %v", bal)
	}

	// Scheduled for the due date, then cancelled.
	status, sched := pay(map[string]string{"line": issue("80.00", day(10), ""), "scheduled_for": day(10)})
	if status != 201 || sched["status"] != "SCHEDULED" {
		t.Fatalf("schedule: %d %v", status, sched)
	}
	if _, c := ana.call("DELETE", base+"/boleto-payments/"+sched["id"].(string), nil, nil); c["status"] != "CANCELLED" {
		t.Fatalf("cancel: %v", c)
	}
	if _, list := ana.call("GET", base+"/boleto-payments", nil, nil); len(list["payments"].([]any)) != 3 {
		t.Fatalf("payments = %v", list)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		_, st := ana.call("GET", base+"/statement", nil, nil)
		found := false
		for _, it := range st["items"].([]any) {
			i := it.(map[string]any)
			if i["type"] == "BOLETO_PAYMENT" && i["counterparty_name"] == "CIA DE ENERGIA" && amountOf(i["amount"]) == "102.50" {
				found = true
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("statement = %v", st)
		}
		time.Sleep(100 * time.Millisecond)
	}

	bruno := login(t, "bruno-boleto-"+uuid.NewString()[:8])
	if status, _ := bruno.call("GET", base+"/boleto-payments/"+sched["id"].(string), nil, nil); status != 404 {
		t.Fatalf("another customer reading the payment: %d", status)
	}
}
