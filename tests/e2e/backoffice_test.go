//go:build e2e

package e2e

import (
	"testing"

	"github.com/google/uuid"
)

// analyst logs in as the back-office analyst bootstrapped in docker compose.
func analyst(t *testing.T) *client {
	c := &client{t: t, base: addr("GATEWAY_URL", "http://127.0.0.1:8000")}
	status, body := c.call("POST", "/v1/auth/login", map[string]string{"email": "analista@digitalbank.dev", "password": e2ePassword}, nil)
	if status != 200 {
		t.Fatalf("analyst login: %d %v", status, body)
	}
	c.token = body["access_token"].(string)
	return c
}

func TestBackOffice(t *testing.T) {
	// A politically exposed person waits in the manual review queue.
	pep := signup(t, "carla-bo-"+uuid.NewString()[:8])
	pep.call("PUT", "/v1/me/profile", map[string]any{"full_name": "Carla Dias", "cpf": cpfWithDigits("222" + randomCPF()[3:9]), "birth_date": "1985-03-10",
		"address": map[string]string{"postal_code": "01310100", "street": "Av. Paulista", "number": "10", "city": "São Paulo", "state": "SP"}}, nil)
	if _, k := pep.call("POST", "/v1/me/kyc", nil, nil); k["kyc_status"] != "MANUAL_REVIEW" {
		t.Fatalf("kyc = %v", k)
	}
	_, me := pep.call("GET", "/v1/me", nil, nil)
	customer := me["customer_id"].(string)
	if me["role"] != "CUSTOMER" {
		t.Fatalf("me = %v", me)
	}

	// Customers cannot enter.
	if status, body := pep.call("GET", "/v1/backoffice/kyc/reviews", nil, nil); status != 403 || errCode(body) != "STAFF_ONLY" {
		t.Fatalf("customer in the back office: %d %v", status, body)
	}

	staff := analyst(t)
	_, q := staff.call("GET", "/v1/backoffice/kyc/reviews", nil, nil)
	found := false
	for _, r := range q["reviews"].([]any) {
		if r.(map[string]any)["customer_id"] == customer {
			found = true
		}
	}
	if !found {
		t.Fatalf("queue = %v", q)
	}
	if status, body := staff.call("POST", "/v1/backoffice/kyc/reviews/"+customer, map[string]any{"approve": false}, nil); status != 400 || errCode(body) != "REASON_REQUIRED" {
		t.Fatalf("rejection without reason: %d %v", status, body)
	}
	_, decided := staff.call("POST", "/v1/backoffice/kyc/reviews/"+customer, map[string]any{"approve": true}, nil)
	_, staffMe := staff.call("GET", "/v1/me", nil, nil)
	if decided["kyc_status"] != "APPROVED" || decided["reviewed_by"] != staffMe["customer_id"] {
		t.Fatalf("decision = %v", decided)
	}

	// The customer can now open an account; the analyst can block it.
	_, acc := pep.call("POST", "/v1/accounts", map[string]string{}, idem())
	accID := acc["id"].(string)
	_, file := staff.call("GET", "/v1/backoffice/customers/"+customer, nil, nil)
	if len(file["accounts"].([]any)) != 1 || file["profile"].(map[string]any)["full_name"] != "Carla Dias" {
		t.Fatalf("customer file = %v", file)
	}
	if status, _ := staff.call("POST", "/v1/backoffice/accounts/"+accID+"/block", map[string]string{"reason": "Suspeita de fraude"}, nil); status != 200 {
		t.Fatalf("block: %d", status)
	}
	if _, a := pep.call("GET", "/v1/accounts/"+accID, nil, nil); a["status"] != "BLOCKED" {
		t.Fatalf("account after block = %v", a)
	}
	staff.call("POST", "/v1/backoffice/accounts/"+accID+"/unblock", nil, nil)
	if _, a := pep.call("GET", "/v1/accounts/"+accID, nil, nil); a["status"] != "ACTIVE" {
		t.Fatalf("account after unblock = %v", a)
	}
}
