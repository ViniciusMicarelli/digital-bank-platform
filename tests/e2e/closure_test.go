//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAccountClosure(t *testing.T) {
	ana := login(t, "ana-encerra-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "300.00"}, nil)
	_, box := ana.call("POST", base+"/savings", map[string]string{"name": "Viagem"}, idem())
	ana.call("POST", base+"/savings/"+box["id"].(string)+"/deposits", map[string]string{"amount": "100.00"}, idem())
	ana.call("POST", base+"/savings", map[string]string{"name": "Vazia"}, idem())
	ana.call("POST", base+"/pix/schedules", map[string]any{"key": "maria@outrobanco.com.br", "amount": "10.00",
		"start_date": saoPaulo(5), "frequency": "MONTHLY", "occurrences": 3}, idem())
	_, card := ana.call("POST", base+"/cards", map[string]string{"kind": "VIRTUAL"}, idem())

	codes := func(c map[string]any) map[string]bool {
		out := map[string]bool{}
		for _, b := range c["blockers"].([]any) {
			out[b.(map[string]any)["code"].(string)] = true
		}
		return out
	}
	_, check := ana.call("GET", base+"/closure", nil, nil)
	if got := codes(check); check["can_close"] != false || !got["BALANCE"] || !got["SAVINGS"] || len(got) != 2 {
		t.Fatalf("check = %v", check)
	}
	if wc := check["will_cancel"].(map[string]any); wc["pix_schedules"] != float64(1) || wc["empty_savings_boxes"] != float64(1) {
		t.Fatalf("will cancel = %v", wc)
	}
	if status, body := ana.call("POST", base+"/close", map[string]string{}, ana.stepUp()); status != 409 || errCode(body) != "CLOSURE_BLOCKED" {
		t.Fatalf("closing with money in it: %d %v", status, body)
	}

	// Take everything out: close the box, send the rest by Pix.
	ana.call("DELETE", base+"/savings/"+box["id"].(string), nil, idem())
	ana.call("POST", base+"/pix/transfers", map[string]string{"key": "maria@outrobanco.com.br", "amount": "300.00"}, idem())
	deadline := time.Now().Add(20 * time.Second)
	for {
		_, check = ana.call("GET", base+"/closure", nil, nil)
		if check["can_close"] == true {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("still blocked: %v", check)
		}
		time.Sleep(300 * time.Millisecond)
	}

	if status, _ := ana.call("POST", base+"/close", map[string]string{}, nil); status != 401 && status != 403 {
		t.Fatalf("closing without the password again: %d", status)
	}
	bruno := login(t, "bruno-encerra-"+uuid.NewString()[:8])
	if status, _ := bruno.call("POST", base+"/close", map[string]string{}, bruno.stepUp()); status != 404 {
		t.Fatalf("someone else closing it: %d", status)
	}
	status, closed := ana.call("POST", base+"/close", map[string]string{"reason": "Mudei de banco"}, ana.stepUp())
	if status != 200 || closed["status"] != "CLOSED" {
		t.Fatalf("close: %d %v", status, closed)
	}
	if _, s := ana.call("GET", base+"/pix/schedules", nil, nil); len(s["schedules"].([]any)) > 0 {
		for _, x := range s["schedules"].([]any) {
			if x.(map[string]any)["status"] == "ACTIVE" {
				t.Fatalf("a schedule survived: %v", s)
			}
		}
	}

	// The event cancels cards and removes Pix keys; the customer is told.
	deadline = time.Now().Add(20 * time.Second)
	for {
		_, cards := ana.call("GET", base+"/cards", nil, nil)
		var cardStatus any
		for _, c := range cards["cards"].([]any) {
			if c.(map[string]any)["id"] == card["id"] {
				cardStatus = c.(map[string]any)["status"]
			}
		}
		_, keys := ana.call("GET", base+"/pix/keys", nil, nil)
		list, _ := keys["keys"].([]any)
		if cardStatus == "CANCELLED" && len(list) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("card %v, keys %v", cardStatus, keys)
		}
		time.Sleep(300 * time.Millisecond)
	}
	if status, body := ana.call("POST", base+"/close", map[string]string{}, ana.stepUp()); status != 409 || errCode(body) != "ACCOUNT_NOT_ACTIVE" {
		t.Fatalf("closing twice: %d %v", status, body)
	}
}
