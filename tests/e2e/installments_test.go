//go:build e2e

package e2e

import (
	"testing"

	"github.com/google/uuid"
)

func TestInstallmentPurchase(t *testing.T) {
	ana := login(t, "ana-parcelado-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	ana.call("POST", base+"/credit", map[string]any{"closing_day": 10}, idem())
	_, card := ana.call("POST", base+"/cards", map[string]string{"kind": "VIRTUAL"}, idem())
	_, d := ana.call("POST", base+"/cards/"+card["id"].(string)+"/reveal", nil, ana.stepUp())
	buy := func(amount, function string, installments int) (int, map[string]any) {
		return ana.call("POST", "/v1/sandbox/card-network/authorizations", map[string]any{
			"pan": d["pan"], "expiry_month": d["expiry_month"], "expiry_year": d["expiry_year"], "cvv": d["cvv"],
			"amount": amount, "merchant_name": "LOJA DE TV", "mcc": "5732", "country": "BR", "channel": "ONLINE",
			"function": function, "installments": installments,
		}, nil)
	}
	if status, body := buy("100.00", "DEBIT", 3); status != 400 {
		t.Fatalf("debit in installments: %d %v", status, body)
	}
	if status, body := buy("9.00", "CREDIT", 2); status != 400 {
		t.Fatalf("installments under R$ 5: %d %v", status, body)
	}
	_, a := buy("900.00", "CREDIT", 10)
	if a["status"] != "APPROVED" || a["installments"] != float64(10) {
		t.Fatalf("10x purchase: %v", a)
	}
	ana.call("POST", "/v1/sandbox/card-network/authorizations/"+a["id"].(string)+"/clear", map[string]any{}, nil)

	// The whole amount is on the limit, one installment on the invoice.
	_, line := ana.call("GET", base+"/credit", nil, nil)
	if amountOf(line["used"]) != "900.00" || amountOf(line["upcoming_installments"]) != "810.00" {
		t.Fatalf("line = %v", line)
	}
	_, invs := ana.call("GET", base+"/credit/invoices", nil, nil)
	if inv := invs["invoices"].([]any)[0].(map[string]any); amountOf(inv["total"]) != "90.00" {
		t.Fatalf("open invoice = %v", inv)
	}
	_, plans := ana.call("GET", base+"/credit/installments", nil, nil)
	p := plans["plans"].([]any)[0].(map[string]any)
	if p["billed"] != float64(1) || p["installments"] != float64(10) || amountOf(p["remaining"]) != "810.00" {
		t.Fatalf("plan = %v", p)
	}

	// The invoice closes: the next one starts with installment 2/10.
	ana.call("POST", "/v1/sandbox/accounts/"+acc["id"].(string)+"/credit/cycle", map[string]string{"action": "CLOSE"}, nil)
	_, invs = ana.call("GET", base+"/credit/invoices", nil, nil)
	open := invs["invoices"].([]any)[0].(map[string]any)
	_, detail := ana.call("GET", base+"/credit/invoices/"+open["id"].(string), nil, nil)
	items := detail["items"].([]any)
	if amountOf(open["total"]) != "90.00" || len(items) != 1 || items[0].(map[string]any)["description"] != "LOJA DE TV (2/10)" {
		t.Fatalf("next invoice = %v", detail)
	}
}

func TestInvoicePlan(t *testing.T) {
	ana := login(t, "ana-parcela-fatura-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	ana.call("POST", base+"/credit", map[string]any{"closing_day": 10}, idem())
	_, card := ana.call("POST", base+"/cards", map[string]string{"kind": "VIRTUAL"}, idem())
	_, d := ana.call("POST", base+"/cards/"+card["id"].(string)+"/reveal", nil, ana.stepUp())
	_, a := ana.call("POST", "/v1/sandbox/card-network/authorizations", map[string]any{
		"pan": d["pan"], "expiry_month": d["expiry_month"], "expiry_year": d["expiry_year"], "cvv": d["cvv"],
		"amount": "600.00", "merchant_name": "LOJA DE TV", "mcc": "5732", "country": "BR", "channel": "ONLINE", "function": "CREDIT",
	}, nil)
	ana.call("POST", "/v1/sandbox/card-network/authorizations/"+a["id"].(string)+"/clear", map[string]any{}, nil)
	ana.call("POST", "/v1/sandbox/accounts/"+acc["id"].(string)+"/credit/cycle", map[string]string{"action": "CLOSE"}, nil)

	_, invs := ana.call("GET", base+"/credit/invoices", nil, nil)
	closed := invs["invoices"].([]any)[1].(map[string]any)
	if closed["status"] != "CLOSED" || amountOf(closed["minimum_payment"]) != "90.00" {
		t.Fatalf("closed invoice = %v", closed)
	}
	plans := base + "/credit/invoices/" + closed["id"].(string) + "/plans"
	_, q := ana.call("GET", plans, nil, nil)
	quotes := q["quotes"].([]any)
	six := quotes[4].(map[string]any)
	if len(quotes) != 11 || six["installments"] != float64(6) || six["monthly_rate"] != 4.99 {
		t.Fatalf("quotes = %v", q)
	}
	key := idem()
	status, plan := ana.call("POST", plans, map[string]int{"installments": 6}, key)
	if status != 201 || amountOf(plan["installment"]) != amountOf(six["installment"]) {
		t.Fatalf("plan: %d %v", status, plan)
	}
	if status, again := ana.call("POST", plans, map[string]int{"installments": 6}, key); status != 201 || again["id"] != plan["id"] {
		t.Fatalf("retry: %d %v", status, again)
	}
	if status, body := ana.call("GET", plans, nil, nil); status != 422 || errCode(body) != "INVOICE_NOT_PLANNABLE" {
		t.Fatalf("quotes of a settled invoice: %d %v", status, body)
	}
	_, invs = ana.call("GET", base+"/credit/invoices", nil, nil)
	open, settled := invs["invoices"].([]any)[0].(map[string]any), invs["invoices"].([]any)[1].(map[string]any)
	if settled["status"] != "PAID" || amountOf(open["total"]) != amountOf(six["installment"]) {
		t.Fatalf("after the plan: open %v, settled %v", open, settled)
	}
	_, line := ana.call("GET", base+"/credit", nil, nil)
	if amountOf(line["upcoming_installments"]) == "0.00" {
		t.Fatalf("line = %v", line)
	}
}
