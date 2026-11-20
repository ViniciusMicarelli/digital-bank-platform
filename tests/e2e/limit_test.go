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

	var review map[string]any
	// A new card: the base score, no usage, explained factor by factor.
	status, sc := ana.call("GET", base+"/credit/score", nil, nil)
	if factors, _ := sc["factors"].([]any); status != 200 || sc["score"] != float64(580) || sc["band"] != "GOOD" || len(factors) != 6 ||
		sc["auto_increase_bp"] != float64(5000) {
		t.Fatalf("score: %d %v", status, sc)
	}

	if status, r := ana.call("POST", base+"/credit/limit", map[string]string{"amount": "700.00"}, idem()); status != 200 || r["status"] != "APPROVED" || limit() != "700.00" {
		t.Fatalf("decrease: %d %v", status, r)
	}
	if status, r := ana.call("POST", base+"/credit/limit", map[string]string{"amount": "1000.00"}, idem()); status != 200 || r["decided_by"] != "automatic" ||
		r["score"] != float64(580) || limit() != "1000.00" {
		t.Fatalf("automatic increase: %d %v", status, r)
	}
	status, review = ana.call("POST", base+"/credit/limit", map[string]string{"amount": "5000.00"}, idem())
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

	// Above R$ 10.000: the analyst's approval waits for a second analyst.
	_, big := ana.call("POST", base+"/credit/limit", map[string]string{"amount": "15000.00"}, idem())
	decide = "/v1/backoffice/limit-requests/" + big["id"].(string) + "/decide"
	if status, body := staff.call("POST", decide, map[string]any{"approve": true}, nil); status != 400 || errCode(body) != "REASON_REQUIRED" {
		t.Fatalf("four-eyes without a reason: %d %v", status, body)
	}
	status, pending := staff.call("POST", decide, map[string]any{"approve": true, "reason": "Renda de R$ 40 mil comprovada"}, nil)
	if status != 202 || pending["status"] != "AWAITING_SECOND_ANALYST" || limit() != "3000.00" {
		t.Fatalf("large approval: %d %v", status, pending)
	}
	approval := pending["approval"].(map[string]any)
	if approval["kind"] != "LIMIT_INCREASE" || approval["params"].(map[string]any)["granted_minor"] != "1500000" {
		t.Fatalf("approval = %v", approval)
	}
	approve := "/v1/backoffice/approvals/" + approval["id"].(string) + "/approve"
	if status, body := staff.call("POST", approve, map[string]string{}, nil); status != 422 || errCode(body) != "SAME_ANALYST" {
		t.Fatalf("the same analyst confirming: %d %v", status, body)
	}
	reviewer := analyst(t, reviewerEmail, reviewerSecret)
	_, list := reviewer.call("GET", "/v1/backoffice/approvals", nil, nil)
	var seen map[string]any
	for _, x := range list["approvals"].([]any) {
		if x.(map[string]any)["id"] == approval["id"] {
			seen = x.(map[string]any)
		}
	}
	if seen == nil || amountOf(seen["current_limit"]) != "3000.00" || seen["account"] == nil {
		t.Fatalf("the reviewer's queue = %v", seen)
	}
	if status, done := reviewer.call("POST", approve, map[string]string{}, nil); status != 200 || done["status"] != "EXECUTED" {
		t.Fatalf("second analyst: %d %v", status, done)
	}
	if limit() != "15000.00" {
		t.Fatalf("limit after four-eyes = %s", limit())
	}
}
