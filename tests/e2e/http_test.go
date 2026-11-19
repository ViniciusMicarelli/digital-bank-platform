//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// client is a customer using the public REST API.
type client struct {
	t     *testing.T
	base  string
	token string
}

func (c *client) call(method, path string, body any, headers map[string]string) (int, map[string]any) {
	c.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, c.base+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

const e2ePassword = "correct horse battery staple"

// randomCPF returns a valid CPF the bureau simulator approves (it rejects
// prefix 111 and reviews 222).
func randomCPF() string {
	nine := []byte{byte('3' + rand.IntN(7))}
	for len(nine) < 9 {
		nine = append(nine, byte('0'+rand.IntN(10)))
	}
	return cpfWithDigits(string(nine))
}

var surnames = map[string]string{"ana": "Souza", "bruno": "Lima", "carol": "Dias", "carla": "Dias"}

// fullName turns "ana-card-1a2b" or "ana.souza-1a2b" into "Ana Souza".
func fullName(login string) string {
	parts := strings.Split(strings.Split(login, "-")[0], ".")
	if len(parts) == 1 {
		s := surnames[parts[0]]
		if s == "" {
			s = "Silva"
		}
		parts = append(parts, s)
	}
	for i, p := range parts {
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// onboard fills in the profile and passes identity verification (KYC),
// which opening an account requires.
func (c *client) onboard(name string) {
	c.t.Helper()
	status, p := c.call("PUT", "/v1/me/profile", map[string]any{
		"full_name": name, "cpf": randomCPF(), "birth_date": "1990-05-20", "phone": "+5511912345678",
		"address": map[string]string{"postal_code": "01310-100", "street": "Av. Paulista", "number": "1000", "city": "São Paulo", "state": "SP"},
	}, nil)
	if status != 200 {
		c.t.Fatalf("profile: %d %v", status, p)
	}
	if status, k := c.call("POST", "/v1/me/kyc", nil, nil); status != 200 || k["kyc_status"] != "APPROVED" {
		c.t.Fatalf("kyc: %d %v", status, k)
	}
}

// signup registers and logs in a customer, without onboarding.
func signup(t *testing.T, name string) *client {
	c := &client{t: t, base: addr("GATEWAY_URL", "http://127.0.0.1:8000")}
	email := name + "@example.com"
	if status, body := c.call("POST", "/v1/auth/register", map[string]string{
		"email": email, "password": e2ePassword, "full_name": name,
	}, nil); status != 201 {
		t.Fatalf("register: %d %v", status, body)
	}
	status, body := c.call("POST", "/v1/auth/login", map[string]string{"email": email, "password": e2ePassword}, nil)
	if status != 200 {
		t.Fatalf("login: %d %v", status, body)
	}
	c.token = body["access_token"].(string)
	return c
}

// login signs up a customer, logs in and completes onboarding.
func login(t *testing.T, name string) *client {
	c := signup(t, name)
	c.onboard(fullName(name))
	return c
}

// stepUp re-confirms the password and returns the X-Step-Up-Token header.
func (c *client) stepUp() map[string]string {
	c.t.Helper()
	status, body := c.call("POST", "/v1/auth/step-up", map[string]string{"password": e2ePassword}, nil)
	if status != 200 {
		c.t.Fatalf("step-up: %d %v", status, body)
	}
	return map[string]string{"X-Step-Up-Token": body["step_up_token"].(string)}
}

func idem() map[string]string { return map[string]string{"Idempotency-Key": uuid.NewString()} }

func amountOf(v any) string { return v.(map[string]any)["amount"].(string) }

func TestPublicAPI(t *testing.T) {
	ana := login(t, "ana-"+uuid.NewString()[:8])
	bruno := login(t, "bruno-"+uuid.NewString()[:8])

	status, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	if status != 201 {
		t.Fatalf("open: %d %v", status, acc)
	}
	anaID := acc["id"].(string)
	_, accB := bruno.call("POST", "/v1/accounts", map[string]string{"holder_name": "Bruno Lima"}, idem())
	brunoID := accB["id"].(string)

	// Ana cannot see Bruno's account.
	if status, body := ana.call("GET", "/v1/accounts/"+brunoID+"/balance", nil, nil); status != 404 {
		t.Fatalf("cross-customer access: %d %v", status, body)
	}

	// Fund Ana through a (sandbox) Pix from another bank.
	anaKey := "ana." + uuid.NewString()[:8] + "@example.com"
	if status, body := ana.call("POST", "/v1/accounts/"+anaID+"/pix/keys", map[string]string{"type": "EMAIL", "value": anaKey}, nil); status != 201 {
		t.Fatalf("register key: %d %v", status, body)
	}
	if status, body := ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": anaKey, "amount": "250.00", "payer_name": "Empresa X"}, nil); status != 201 {
		t.Fatalf("sandbox pix-in: %d %v", status, body)
	}

	// Transfer to Bruno, retried with the same Idempotency-Key.
	key := idem()
	req := map[string]string{"to_account_id": brunoID, "amount": "100.00", "description": "Aluguel"}
	for range 2 {
		if status, body := ana.call("POST", "/v1/accounts/"+anaID+"/transfers", req, key); status != 201 {
			t.Fatalf("transfer: %d %v", status, body)
		}
	}
	if status, body := ana.call("POST", "/v1/accounts/"+anaID+"/transfers",
		// More than the balance, within the nightly limit (R$ 1.000 from 20h to 6h).
		map[string]string{"to_account_id": brunoID, "amount": "500.00"}, idem()); status != 422 || body["error"].(map[string]any)["code"] != "INSUFFICIENT_FUNDS" {
		t.Fatalf("overdraw: %d %v", status, body)
	}

	// Pix to another bank.
	_, look := ana.call("GET", "/v1/pix/keys/maria@outrobanco.com.br", nil, nil)
	if look["owner"].(map[string]any)["name"] != "Maria Oliveira" {
		t.Fatalf("lookup: %v", look)
	}
	if status, body := ana.call("POST", "/v1/accounts/"+anaID+"/pix/transfers",
		map[string]string{"key": "maria@outrobanco.com.br", "amount": "50.00", "description": "Presente"}, idem()); status != 201 {
		t.Fatalf("pix: %d %v", status, body)
	}

	_, bal := ana.call("GET", "/v1/accounts/"+anaID+"/balance", nil, nil)
	if amountOf(bal["available"]) != "100.00" {
		t.Fatalf("ana balance = %v (250 - 100 - 50)", bal)
	}
	_, bal = bruno.call("GET", "/v1/accounts/"+brunoID+"/balance", nil, nil)
	if amountOf(bal["available"]) != "100.00" {
		t.Fatalf("bruno balance = %v", bal)
	}

	// The statement converges (eventual consistency).
	deadline := time.Now().Add(15 * time.Second)
	for {
		_, st := ana.call("GET", "/v1/accounts/"+anaID+"/statement", nil, nil)
		items, _ := st["items"].([]any)
		if len(items) == 3 {
			newest := items[0].(map[string]any)
			if newest["type"] != "PIX_OUT" || newest["counterparty_name"] != "Maria Oliveira" || amountOf(newest["amount"]) != "50.00" {
				t.Fatalf("newest item = %v", newest)
			}
			oldest := items[2].(map[string]any)
			if oldest["direction"] != "CREDIT" || oldest["counterparty_name"] != "Empresa X" {
				t.Fatalf("oldest item = %v", oldest)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("statement = %v", st)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
