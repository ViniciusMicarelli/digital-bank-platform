//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
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

func login(t *testing.T, customer string) *client {
	c := &client{t: t, base: addr("GATEWAY_URL", "http://127.0.0.1:8000")}
	status, body := c.call("POST", "/v1/auth/token", map[string]string{"customer_id": customer}, nil)
	if status != 200 {
		t.Fatalf("login: %d %v", status, body)
	}
	c.token = body["access_token"].(string)
	return c
}

func idem() map[string]string { return map[string]string{"Idempotency-Key": uuid.NewString()} }

func amountOf(v any) string { return v.(map[string]any)["amount"].(string) }

func TestPublicAPI(t *testing.T) {
	ana := login(t, "ana-"+uuid.NewString()[:8])
	bruno := login(t, "bruno-"+uuid.NewString()[:8])

	status, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza", "document": "529.982.247-25"}, idem())
	if status != 201 {
		t.Fatalf("open: %d %v", status, acc)
	}
	anaID := acc["id"].(string)
	_, accB := bruno.call("POST", "/v1/accounts", map[string]string{"holder_name": "Bruno Lima", "document": "111.444.777-35"}, idem())
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
		map[string]string{"to_account_id": brunoID, "amount": "1000.00"}, idem()); status != 422 || body["error"].(map[string]any)["code"] != "INSUFFICIENT_FUNDS" {
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
