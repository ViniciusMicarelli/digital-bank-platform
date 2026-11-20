//go:build e2e

package e2e

import (
	"testing"

	"github.com/google/uuid"
)

func TestCreditLimitRequests(t *testing.T) {
	ana := login(t, "ana-limite-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	ana.call("POST", base+"/credit", map[string]any{"closing_day": 10}, idem())
	limit := func() string {
		_, l := ana.call("GET", base+"/credit", nil, nil)
		return amountOf(l["limit"])
	}

	if status, r := ana.call("POST", base+"/credit/limit", map[string]string{"amount": "700.00"}, idem()); status != 200 || r["status"] != "APPROVED" || limit() != "700.00" {
		t.Fatalf("decrease: %d %v", status, r)
	}
	if status, r := ana.call("POST", base+"/credit/limit", map[string]string{"amount": "1000.00"}, idem()); status != 200 || r["decided_by"] != "automatic" || limit() != "1000.00" {
		t.Fatalf("automatic increase: %d %v", status, r)
	}
	status, review := ana.call("POST", base+"/credit/limit", map[string]string{"amount": "5000.00"}, idem())
	if status != 202 || review["status"] != "IN_REVIEW" || limit() != "1000.00" {
		t.Fatalf("large increase: %d %v", status, review)
	}

	staff := analyst(t, analystEmail, analystSecret)
	_, queue := staff.call("GET", "/v1/backoffice/limit-requests", nil, nil)
	found := false
	for _, r := range queue["requests"].([]any) {
		if r.(map[string]any)["id"] == review["id"] {
			found = true
		}
	}
	if !found {
		t.Fatalf("queue = %v", queue)
	}
	decide := "/v1/backoffice/limit-requests/" + review["id"].(string) + "/decide"
	if status, body := ana.call("POST", decide, map[string]any{"approve": true}, nil); status != 403 {
		t.Fatalf("customer deciding: %d %v", status, body)
	}
	if status, r := staff.call("POST", decide, map[string]any{"approve": true, "granted": "3000.00", "reason": "Renda compatível"}, nil); status != 200 || amountOf(r["granted"]) != "3000.00" {
		t.Fatalf("partial approval: %d %v", status, r)
	}
	if limit() != "3000.00" {
		t.Fatalf("limit after the analyst = %s", limit())
	}
	if got := actions(staff, acc["id"].(string)); len(got) == 0 || got[len(got)-1] != "LIMIT_APPROVE" {
		t.Fatalf("trail = %v", got)
	}
	_, mine := ana.call("GET", base+"/credit/limit-requests", nil, nil)
	if n := len(mine["requests"].([]any)); n != 3 {
		t.Fatalf("customer's requests = %v", mine)
	}
}
