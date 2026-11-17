//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type customer struct {
	c       *client
	account string
	key     string
}

func newCustomer(t *testing.T, name, cpf, fund string) customer {
	t.Helper()
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "."))
	c := login(t, slug+"-"+uuid.NewString()[:8])
	_, acc := c.call("POST", "/v1/accounts", map[string]string{"holder_name": name, "document": cpf}, idem())
	id := acc["id"].(string)
	key := slug + "." + uuid.NewString()[:8] + "@example.com"
	c.call("POST", "/v1/accounts/"+id+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	if fund != "" {
		c.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": fund}, nil)
	}
	return customer{c: c, account: id, key: key}
}

func (u customer) balance(t *testing.T) string {
	_, b := u.c.call("GET", "/v1/accounts/"+u.account+"/balance", nil, nil)
	return amountOf(b["available"])
}

// saoPaulo returns a date in the bank's calendar, days from today.
func saoPaulo(days int) string {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	return time.Now().In(loc).AddDate(0, 0, days).Format("2006-01-02")
}

func TestScheduledPixAndPixAutomatico(t *testing.T) {
	ana := newCustomer(t, "Ana Souza", "529.982.247-25", "300.00")
	gym := newCustomer(t, "Academia Forte", "111.444.777-35", "")

	status, sc := ana.c.call("POST", "/v1/accounts/"+ana.account+"/pix/schedules", map[string]any{
		"key": gym.key, "amount": "50.00", "description": "Plano anual (parcela)", "start_date": saoPaulo(1),
		"frequency": "MONTHLY", "occurrences": 2,
	}, idem())
	if status != 201 || sc["next_date"] != saoPaulo(1) {
		t.Fatalf("schedule: %d %v", status, sc)
	}
	ana.c.call("POST", "/v1/sandbox/pix/run-schedules", map[string]string{"as_of": saoPaulo(1)}, nil)
	if gym.balance(t) != "50.00" {
		t.Fatalf("scheduled Pix not executed: gym %s", gym.balance(t))
	}

	// Pix Automático: the gym asks, Ana approves, the gym charges.
	status, m := gym.c.call("POST", "/v1/accounts/"+gym.account+"/pix/mandates", map[string]string{
		"receiver_key": gym.key, "payer_key": ana.key, "max_amount": "120.00", "frequency": "MONTHLY", "description": "Mensalidade",
	}, nil)
	if status != 201 || m["status"] != "PENDING" {
		t.Fatalf("mandate: %d %v", status, m)
	}
	mid := m["id"].(string)
	if status, body := ana.c.call("POST", "/v1/accounts/"+ana.account+"/pix/mandates/"+mid+"/approve", nil, nil); status != 200 || body["status"] != "ACTIVE" {
		t.Fatalf("approve: %d %v", status, body)
	}
	if status, body := gym.c.call("POST", "/v1/accounts/"+gym.account+"/pix/mandates/"+mid+"/charges", map[string]string{"amount": "130.00", "due_date": saoPaulo(2)}, idem()); status != 422 || errorCode(body) != "CHARGE_NOT_ALLOWED" {
		t.Fatalf("charge above the maximum: %d %v", status, body)
	}
	if status, body := gym.c.call("POST", "/v1/accounts/"+gym.account+"/pix/mandates/"+mid+"/charges", map[string]string{"amount": "99.90", "due_date": saoPaulo(2)}, idem()); status != 201 {
		t.Fatalf("charge: %d %v", status, body)
	}
	ana.c.call("POST", "/v1/sandbox/pix/run-schedules", map[string]string{"as_of": saoPaulo(2)}, nil)
	if gym.balance(t) != "149.90" || ana.balance(t) != "150.10" {
		t.Fatalf("after the charge: gym %s ana %s", gym.balance(t), ana.balance(t))
	}
	if status, body := ana.c.call("POST", "/v1/accounts/"+ana.account+"/pix/mandates/"+mid+"/revoke", nil, nil); status != 200 || body["status"] != "REVOKED" {
		t.Fatalf("revoke: %d %v", status, body)
	}
}

func TestDynamicQRCodeAPI(t *testing.T) {
	ana := newCustomer(t, "Ana Souza", "529.982.247-25", "100.00")
	shop := newCustomer(t, "Loja Centro", "111.444.777-35", "")

	status, qr := shop.c.call("POST", "/v1/accounts/"+shop.account+"/pix/qr-codes", map[string]any{
		"key": shop.key, "amount": "25.00", "description": "Pedido 77", "one_time": true, "dynamic": true, "expires_in_seconds": 600,
	}, nil)
	if status != 201 {
		t.Fatalf("create: %d %v", status, qr)
	}
	payload := qr["payload"].(string)
	if strings.Contains(payload, shop.key) || !strings.Contains(payload, "/pix/qr/v2/"+qr["txid"].(string)) {
		t.Fatalf("dynamic payload must carry only the URL: %s", payload)
	}

	// Anyone (another bank) can fetch the signed payload and the keys, without auth.
	resp, err := http.Get(ana.c.base + "/pix/qr/v2/" + qr["txid"].(string))
	if err != nil {
		t.Fatal(err)
	}
	jws, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/jose" || strings.Count(string(jws), ".") != 2 {
		t.Fatalf("payload endpoint: %d %q", resp.StatusCode, jws)
	}

	_, dec := ana.c.call("POST", "/v1/pix/qr-codes/decode", map[string]string{"payload": payload}, nil)
	if dec["key"] != shop.key || amountOf(dec["amount"]) != "25.00" {
		t.Fatalf("decode: %v", dec)
	}
	if status, paid := ana.c.call("POST", "/v1/accounts/"+ana.account+"/pix/qr-payments", map[string]string{"payload": payload}, idem()); status != 201 {
		t.Fatalf("pay: %d %v", status, paid)
	}
	if status, body := ana.c.call("POST", "/v1/accounts/"+ana.account+"/pix/qr-payments", map[string]string{"payload": payload}, idem()); status != 422 || errorCode(body) != "QR_CODE_UNAVAILABLE" {
		t.Fatalf("second payment: %d %v", status, body)
	}
}
