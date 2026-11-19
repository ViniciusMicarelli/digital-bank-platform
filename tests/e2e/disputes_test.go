//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCardRefundsAndDisputes(t *testing.T) {
	ana := login(t, "ana-disputa-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	accID := acc["id"].(string)
	base := "/v1/accounts/" + accID
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "200.00"}, nil)
	_, card := ana.call("POST", base+"/cards", map[string]string{"kind": "VIRTUAL"}, idem())
	cardBase := base + "/cards/" + card["id"].(string)
	_, d := ana.call("POST", cardBase+"/reveal", nil, ana.stepUp())
	buy := func(amount, merchant, function string) string {
		_, a := ana.call("POST", "/v1/sandbox/card-network/authorizations", map[string]any{
			"pan": d["pan"], "expiry_month": d["expiry_month"], "expiry_year": d["expiry_year"], "cvv": d["cvv"],
			"amount": amount, "merchant_name": merchant, "mcc": "5732", "country": "BR", "channel": "ONLINE", "function": function,
		}, nil)
		if a["status"] != "APPROVED" {
			t.Fatalf("purchase: %v", a)
		}
		id := a["id"].(string)
		ana.call("POST", "/v1/sandbox/card-network/authorizations/"+id+"/clear", map[string]any{}, nil)
		return id
	}
	available := func() string {
		_, bal := ana.call("GET", base+"/balance", nil, nil)
		return amountOf(bal["available"])
	}

	// The merchant refunds part of a purchase (network retries are harmless).
	tv := buy("100.00", "LOJA DE TV", "DEBIT")
	refund := map[string]string{"amount": "30.00", "refund_reference": "rf-" + uuid.NewString()}
	for range 2 {
		if status, r := ana.call("POST", "/v1/sandbox/card-network/authorizations/"+tv+"/refund", refund, nil); status != 200 || r["status"] != "COMPLETED" {
			t.Fatalf("merchant refund: %d %v", status, r)
		}
	}
	if got := available(); got != "130.00" {
		t.Fatalf("balance after the refund = %s", got)
	}
	if status, body := ana.call("POST", "/v1/sandbox/card-network/authorizations/"+tv+"/refund", map[string]string{"amount": "70.01"}, nil); status != 422 || errCode(body) != "REFUND_TOO_LARGE" {
		t.Fatalf("refund more than left: %d %v", status, body)
	}

	// The customer does not recognize a purchase: dispute, and the card is blocked.
	fraud := buy("50.00", "LOJA ESTRANHA", "DEBIT")
	status, dispute := ana.call("POST", cardBase+"/purchases/"+fraud+"/disputes", map[string]string{"reason": "NOT_RECOGNIZED", "description": "Não fui eu"}, nil)
	if status != 201 || dispute["status"] != "OPEN" || amountOf(dispute["amount"]) != "50.00" || dispute["merchant_name"] != "LOJA ESTRANHA" {
		t.Fatalf("open dispute: %d %v", status, dispute)
	}
	if status, body := ana.call("POST", cardBase+"/purchases/"+fraud+"/disputes", map[string]string{"reason": "DUPLICATE"}, nil); status != 422 || errCode(body) != "DISPUTE_ALREADY_OPEN" {
		t.Fatalf("second dispute: %d %v", status, body)
	}
	_, cards := ana.call("GET", base+"/cards", nil, nil)
	if c := cards["cards"].([]any)[0].(map[string]any); c["status"] != "BLOCKED" {
		t.Fatalf("card after a fraud dispute = %v", c)
	}
	_, purchases := ana.call("GET", cardBase+"/purchases", nil, nil)
	var seen bool
	for _, p := range purchases["purchases"].([]any) {
		p := p.(map[string]any)
		if p["id"] == fraud {
			seen = p["dispute"].(map[string]any)["status"] == "OPEN"
		}
		if p["id"] == tv && amountOf(p["refunded_amount"]) != "30.00" {
			t.Fatalf("refunded purchase = %v", p)
		}
	}
	if !seen {
		t.Fatalf("purchases = %v", purchases)
	}

	// Customers cannot decide; an analyst accepts and the money comes back.
	disputeID := dispute["id"].(string)
	if status, _ := ana.call("POST", "/v1/backoffice/disputes/"+disputeID+"/decide", map[string]any{"won": true}, nil); status != 403 {
		t.Fatalf("customer deciding: %d", status)
	}
	staff := analyst(t, analystEmail, analystSecret)
	_, open := staff.call("GET", "/v1/backoffice/disputes", nil, nil)
	found := false
	for _, x := range open["disputes"].([]any) {
		if x.(map[string]any)["id"] == disputeID {
			found = true
		}
	}
	if !found {
		t.Fatalf("open disputes = %v", open)
	}
	if status, body := staff.call("POST", "/v1/backoffice/disputes/"+disputeID+"/decide", map[string]any{"won": false}, nil); status != 400 || errCode(body) != "REASON_REQUIRED" {
		t.Fatalf("lost without a reason: %d %v", status, body)
	}
	if status, won := staff.call("POST", "/v1/backoffice/disputes/"+disputeID+"/decide", map[string]any{"won": true}, nil); status != 200 || won["status"] != "WON" {
		t.Fatalf("accept: %d %v", status, won)
	}
	if got := available(); got != "130.00" { // 200 - 100 + 30 - 50 + 50
		t.Fatalf("balance after the won dispute = %s", got)
	}
	if got := actions(staff, accID); len(got) == 0 || got[len(got)-1] != "DISPUTE_ACCEPT" {
		t.Fatalf("account trail = %v", got)
	}
	_, mine := ana.call("GET", base+"/disputes", nil, nil)
	if list := mine["disputes"].([]any); len(list) != 1 || list[0].(map[string]any)["status"] != "WON" {
		t.Fatalf("customer disputes = %v", mine)
	}

	// The refund shows in the feed (the projection is asynchronous).
	deadline := time.Now().Add(15 * time.Second)
	for {
		_, feed := ana.call("GET", base+"/statement", nil, nil)
		refunds := 0
		for _, it := range feed["items"].([]any) {
			if it.(map[string]any)["type"] == "CARD_REFUND" {
				refunds++
			}
		}
		if refunds == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("statement refunds = %d: %v", refunds, feed)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestCreditCardDispute(t *testing.T) {
	ana := login(t, "ana-disputa-credito-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	ana.call("POST", base+"/credit", map[string]any{"closing_day": 10}, idem())
	_, card := ana.call("POST", base+"/cards", map[string]string{"kind": "VIRTUAL"}, idem())
	cardBase := base + "/cards/" + card["id"].(string)
	_, d := ana.call("POST", cardBase+"/reveal", nil, ana.stepUp())
	_, a := ana.call("POST", "/v1/sandbox/card-network/authorizations", map[string]any{
		"pan": d["pan"], "expiry_month": d["expiry_month"], "expiry_year": d["expiry_year"], "cvv": d["cvv"],
		"amount": "120.00", "merchant_name": "ACADEMIA", "mcc": "7997", "country": "BR", "channel": "ONLINE", "function": "CREDIT",
	}, nil)
	ana.call("POST", "/v1/sandbox/card-network/authorizations/"+a["id"].(string)+"/clear", map[string]any{}, nil)

	_, dispute := ana.call("POST", cardBase+"/purchases/"+a["id"].(string)+"/disputes",
		map[string]string{"reason": "CANCELLED", "amount": "40.00", "description": "Cancelei o plano"}, nil)
	staff := analyst(t, analystEmail, analystSecret)
	if status, won := staff.call("POST", "/v1/backoffice/disputes/"+dispute["id"].(string)+"/decide", map[string]any{"won": true}, nil); status != 200 || won["status"] != "WON" {
		t.Fatalf("accept: %d %v", status, won)
	}
	_, line := ana.call("GET", base+"/credit", nil, nil)
	if amountOf(line["used"]) != "80.00" {
		t.Fatalf("credit line after the refund = %v", line)
	}
	_, invoices := ana.call("GET", base+"/credit/invoices", nil, nil)
	inv := invoices["invoices"].([]any)[0].(map[string]any)
	if amountOf(inv["total"]) != "80.00" {
		t.Fatalf("invoice after the refund = %v", inv)
	}
}
