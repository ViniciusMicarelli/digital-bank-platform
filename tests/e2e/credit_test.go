//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func errCode(body map[string]any) any {
	e, _ := body["error"].(map[string]any)
	return e["code"]
}

func TestCreditCard(t *testing.T) {
	ana := login(t, "ana-credit-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	accID := acc["id"].(string)
	base := "/v1/accounts/" + accID
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "100.00"}, nil)
	_, card := ana.call("POST", base+"/cards", map[string]string{"kind": "VIRTUAL"}, idem())
	_, d := ana.call("POST", base+"/cards/"+card["id"].(string)+"/reveal", nil, ana.stepUp())
	buy := func(amount string) map[string]any {
		_, a := ana.call("POST", "/v1/sandbox/card-network/authorizations", map[string]any{
			"pan": d["pan"], "expiry_month": d["expiry_month"], "expiry_year": d["expiry_year"], "cvv": d["cvv"],
			"amount": amount, "merchant_name": "LOJA DE TV", "mcc": "5732", "country": "BR", "channel": "ONLINE", "function": "CREDIT",
		}, nil)
		return a
	}

	if a := buy("10.00"); a["response_code"] != "57" {
		t.Fatalf("credit purchase without a credit line: %v", a)
	}
	status, line := ana.call("POST", base+"/credit", map[string]any{"closing_day": 10}, idem())
	if status != 201 || amountOf(line["available"]) != "1000.00" {
		t.Fatalf("open credit line: %d %v", status, line)
	}
	if status, body := ana.call("POST", base+"/credit", map[string]any{"closing_day": 10}, idem()); status != 422 || errCode(body) != "CREDIT_LINE_EXISTS" {
		t.Fatalf("second credit line: %d %v", status, body)
	}

	a := buy("250.00")
	if a["status"] != "APPROVED" || a["function"] != "CREDIT" {
		t.Fatalf("credit purchase: %v", a)
	}
	if _, c := ana.call("POST", "/v1/sandbox/card-network/authorizations/"+a["id"].(string)+"/clear", map[string]any{}, nil); c["status"] != "CLEARED" {
		t.Fatalf("clear: %v", c)
	}
	if _, bal := ana.call("GET", base+"/balance", nil, nil); amountOf(bal["available"]) != "100.00" {
		t.Fatalf("a credit purchase must not touch the checking account: %v", bal)
	}
	if a := buy("750.01"); a["response_code"] != "51" {
		t.Fatalf("over the credit limit: %v", a)
	}

	// The invoice closes (sandbox: now) with the purchase.
	if status, _ := ana.call("POST", "/v1/sandbox/accounts/"+accID+"/credit/cycle", map[string]string{"action": "CLOSE"}, nil); status != 204 {
		t.Fatalf("close cycle: %d", status)
	}
	_, list := ana.call("GET", base+"/credit/invoices", nil, nil)
	invoices := list["invoices"].([]any)
	closed := invoices[1].(map[string]any)
	if len(invoices) != 2 || closed["status"] != "CLOSED" || amountOf(closed["total"]) != "250.00" {
		t.Fatalf("invoices = %v", list)
	}
	invPath := base + "/credit/invoices/" + closed["id"].(string)
	if _, inv := ana.call("GET", invPath, nil, nil); len(inv["items"].([]any)) != 1 {
		t.Fatalf("invoice items = %v", inv)
	}

	pay := func(amount string) (int, map[string]any) {
		return ana.call("POST", invPath+"/payments", map[string]string{"amount": amount}, idem())
	}
	if status, body := pay("250.01"); status != 422 || errCode(body) != "PAYMENT_TOO_LARGE" {
		t.Fatalf("overpaying: %d %v", status, body)
	}
	if status, body := pay("150.00"); status != 422 || errCode(body) != "INSUFFICIENT_FUNDS" {
		t.Fatalf("paying without money: %d %v", status, body)
	}
	if status, p := pay("80.00"); status != 201 || p["status"] != "COMPLETED" {
		t.Fatalf("partial payment: %d %v", status, p)
	}
	if _, bal := ana.call("GET", base+"/balance", nil, nil); amountOf(bal["available"]) != "20.00" {
		t.Fatalf("balance after paying = %v", bal)
	}

	// Past the due date: 170.00 unpaid + 2% fine + 8% interest go to the next invoice.
	ana.call("POST", "/v1/sandbox/accounts/"+accID+"/credit/cycle", map[string]string{"action": "EXPIRE"}, nil)
	_, list = ana.call("GET", base+"/credit/invoices", nil, nil)
	invoices = list["invoices"].([]any)
	if next := invoices[0].(map[string]any); amountOf(next["total"]) != "187.00" || invoices[1].(map[string]any)["status"] != "OVERDUE" {
		t.Fatalf("after the due date = %v", list)
	}
	if _, l := ana.call("GET", base+"/credit", nil, nil); amountOf(l["used"]) != "187.00" {
		t.Fatalf("line = %v", l)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		_, st := ana.call("GET", base+"/statement", nil, nil)
		items, _ := st["items"].([]any)
		if len(items) > 0 && items[0].(map[string]any)["type"] == "CREDIT_INVOICE_PAYMENT" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("statement = %v", st)
		}
		time.Sleep(100 * time.Millisecond)
	}

	bruno := login(t, "bruno-credit-"+uuid.NewString()[:8])
	if status, _ := bruno.call("GET", invPath, nil, nil); status != 404 {
		t.Fatalf("another customer reading the invoice: %d", status)
	}
}
