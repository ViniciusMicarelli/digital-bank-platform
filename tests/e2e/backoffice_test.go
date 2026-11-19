//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The analysts bootstrapped in docker compose (email and TOTP secret).
const (
	analystEmail   = "analista@digitalbank.dev"
	analystSecret  = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	reviewerEmail  = "revisora@digitalbank.dev"
	reviewerSecret = "KRSXG5CTMVRXEZLUKRSXG5CTMVRXEZLU"
)

// analyst signs in to the back office with password and TOTP. A code is
// accepted once and refused codes count towards the lockout, so the last
// step used per analyst is kept in a temp file (shared with the Playwright
// tests) and a fresh step is always used: this one, the next (the server
// allows one step of drift), or the one after once it is close enough.
//
// Sessions are reused within a test run (tokens last 15 minutes), so each
// analyst signs in once.
func analyst(t *testing.T, email, secret string) *client {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	if token, ok := sessions[email]; ok {
		return &client{t: t, base: addr("GATEWAY_URL", "http://127.0.0.1:8000"), token: token}
	}
	c := signInAnalyst(t, email, secret)
	sessions[email] = c.token
	return c
}

var (
	sessionsMu sync.Mutex
	sessions   = map[string]string{}
)

func signInAnalyst(t *testing.T, email, secret string) *client {
	file := filepath.Join(os.TempDir(), "digital-bank-totp-"+email)
	var last int64
	if b, err := os.ReadFile(file); err == nil {
		last, _ = strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	}
	now := time.Now().Unix() / 30
	step := max(now, last+1)
	if step > now+1 {
		time.Sleep(time.Until(time.Unix((step-1)*30, 0)) + 500*time.Millisecond)
	}
	if err := os.WriteFile(file, []byte(strconv.FormatInt(step, 10)), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &client{t: t, base: addr("GATEWAY_URL", "http://127.0.0.1:8000")}
	status, body := c.call("POST", "/v1/auth/login", map[string]string{"email": email, "password": e2ePassword,
		"totp_code": totp(t, secret, time.Unix(step*30, 0))}, nil)
	if status != 200 {
		t.Fatalf("analyst login: %d %v", status, body)
	}
	c.token = body["access_token"].(string)
	return c
}

// actions lists the audit trail of a target, oldest first.
func actions(staff *client, target string) []string {
	_, trail := staff.call("GET", "/v1/backoffice/audit?target_id="+target, nil, nil)
	var out []string
	for _, e := range trail["events"].([]any) {
		out = append([]string{e.(map[string]any)["action"].(string)}, out...)
	}
	return out
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

	// Analysts need a second factor: the password alone does not open a session.
	if status, body := (&client{t: t, base: addr("GATEWAY_URL", "http://127.0.0.1:8000")}).call("POST", "/v1/auth/login",
		map[string]string{"email": analystEmail, "password": e2ePassword}, nil); status != 401 || errCode(body) != "MFA_REQUIRED" {
		t.Fatalf("analyst without the code: %d %v", status, body)
	}
	staff := analyst(t, analystEmail, analystSecret)
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

	// Unblocking needs a second analyst (four-eyes).
	status, req := staff.call("POST", "/v1/backoffice/accounts/"+accID+"/unblock", map[string]string{"reason": "Cliente confirmou as compras"}, nil)
	if status != 202 || req["status"] != "PENDING" || req["requested_by"] != staffMe["customer_id"] {
		t.Fatalf("unblock request: %d %v", status, req)
	}
	approval := req["id"].(string)
	if status, body := staff.call("POST", "/v1/backoffice/accounts/"+accID+"/unblock", map[string]string{"reason": "de novo"}, nil); status != 422 || errCode(body) != "APPROVAL_ALREADY_PENDING" {
		t.Fatalf("second request: %d %v", status, body)
	}
	if _, a := pep.call("GET", "/v1/accounts/"+accID, nil, nil); a["status"] != "BLOCKED" {
		t.Fatalf("still blocked while pending = %v", a)
	}
	if status, body := staff.call("POST", "/v1/backoffice/approvals/"+approval+"/approve", map[string]string{}, nil); status != 422 || errCode(body) != "SAME_ANALYST" {
		t.Fatalf("self approval: %d %v", status, body)
	}
	reviewer := analyst(t, reviewerEmail, reviewerSecret)
	_, pending := reviewer.call("GET", "/v1/backoffice/approvals", nil, nil)
	if !slices.ContainsFunc(pending["approvals"].([]any), func(a any) bool { return a.(map[string]any)["id"] == approval }) {
		t.Fatalf("pending approvals = %v", pending)
	}
	if status, done := reviewer.call("POST", "/v1/backoffice/approvals/"+approval+"/approve", map[string]string{}, nil); status != 200 || done["status"] != "EXECUTED" {
		t.Fatalf("approve: %d %v", status, done)
	}
	if _, a := pep.call("GET", "/v1/accounts/"+accID, nil, nil); a["status"] != "ACTIVE" {
		t.Fatalf("account after unblock = %v", a)
	}

	// Everything is in the audit trail, which is intact.
	if got, want := actions(staff, accID), []string{"ACCOUNT_BLOCK", "APPROVAL_REQUESTED", "APPROVAL_APPROVED", "APPROVAL_EXECUTED"}; !slices.Equal(got, want) {
		t.Fatalf("account trail = %v, want %v", got, want)
	}
	if got, want := actions(staff, customer), []string{"KYC_APPROVE", "CUSTOMER_FILE_VIEW"}; !slices.Equal(got, want) {
		t.Fatalf("customer trail = %v, want %v", got, want)
	}
	if _, v := staff.call("GET", "/v1/backoffice/audit/verify", nil, nil); v["intact"] != true {
		t.Fatalf("verify = %v", v)
	}
}
