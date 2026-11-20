//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSessions(t *testing.T) {
	name := "ana-sessoes-" + uuid.NewString()[:8]
	ana := signup(t, name) // the first login (Go client): "Dispositivo desconhecido"
	iphone := "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1"
	status, body := ana.call("POST", "/v1/auth/login", map[string]string{"email": name + "@example.com", "password": e2ePassword},
		map[string]string{"User-Agent": iphone})
	if status != 200 {
		t.Fatalf("login from the phone: %d %v", status, body)
	}
	phone := &client{t: t, base: ana.base, token: body["access_token"].(string)}
	phoneRefresh := body["refresh_token"].(string)

	_, list := ana.call("GET", "/v1/me/sessions", nil, nil)
	sessions := list["sessions"].([]any)
	if len(sessions) != 2 {
		t.Fatalf("sessions = %v", list)
	}
	newest, mine := sessions[0].(map[string]any), sessions[1].(map[string]any)
	if newest["device"] != "Safari no iPhone" || newest["current"] != false || mine["current"] != true || mine["ip"] == "" {
		t.Fatalf("sessions = %v", list)
	}

	// The phone is news: a notification says so.
	deadline := time.Now().Add(20 * time.Second)
	for {
		_, n := ana.call("GET", "/v1/notifications", nil, nil)
		var found bool
		for _, x := range n["notifications"].([]any) {
			found = found || x.(map[string]any)["title"] == "Novo acesso à sua conta"
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no new-device notification: %v", n)
		}
		time.Sleep(300 * time.Millisecond)
	}

	// Someone else cannot end it; the customer can, from the laptop.
	bruno := signup(t, "bruno-sessoes-"+uuid.NewString()[:8])
	if status, _ := bruno.call("DELETE", "/v1/me/sessions/"+newest["id"].(string), nil, nil); status != 404 {
		t.Fatalf("bruno ending ana's session: %d", status)
	}
	if status, _ := ana.call("POST", "/v1/me/sessions/end-others", nil, nil); status != 200 {
		t.Fatalf("end others: %d", status)
	}
	if status, _ := phone.call("POST", "/v1/auth/refresh", map[string]string{"refresh_token": phoneRefresh}, nil); status != 401 {
		t.Fatalf("the phone's session still refreshes: %d", status)
	}
	if _, list := ana.call("GET", "/v1/me/sessions", nil, nil); len(list["sessions"].([]any)) != 1 {
		t.Fatalf("after ending the others = %v", list)
	}
}
