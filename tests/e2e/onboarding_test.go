//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOnboarding(t *testing.T) {
	ana := signup(t, "ana-kyc-"+uuid.NewString()[:8])
	if status, body := ana.call("POST", "/v1/accounts", map[string]string{}, idem()); status != 422 || errCode(body) != "KYC_REQUIRED" {
		t.Fatalf("account before KYC: %d %v", status, body)
	}
	if status, _ := ana.call("GET", "/v1/me/profile", nil, nil); status != 404 {
		t.Fatalf("no profile yet: %d", status)
	}

	// CPF prefix 222: a politically exposed person goes to manual review.
	cpf := "222" + randomCPF()[3:9]
	cpf = cpfWithDigits(cpf)
	profile := map[string]any{"full_name": "Ana Souza", "cpf": cpf, "birth_date": "1990-05-20",
		"address": map[string]string{"postal_code": "01310-100", "street": "Av. Paulista", "number": "1000", "city": "São Paulo", "state": "SP"}}
	if status, body := ana.call("PUT", "/v1/me/profile", map[string]any{"full_name": "Ana", "cpf": cpf, "birth_date": "1990-05-20"}, nil); status != 400 {
		t.Fatalf("incomplete profile: %d %v", status, body)
	}
	if status, p := ana.call("PUT", "/v1/me/profile", profile, nil); status != 200 || p["kyc_status"] != "NOT_STARTED" || p["cpf"] == cpf {
		t.Fatalf("profile: %d %v (the CPF must come back masked)", status, p)
	}
	if _, k := ana.call("POST", "/v1/me/kyc", nil, nil); k["kyc_status"] != "MANUAL_REVIEW" {
		t.Fatalf("kyc = %v", k)
	}
	profile["full_name"] = "Ana Souza Lima"
	if status, body := ana.call("PUT", "/v1/me/profile", profile, nil); status != 422 || errCode(body) != "IDENTITY_LOCKED" {
		t.Fatalf("changing the name under review: %d %v", status, body)
	}
	if _, k := ana.call("POST", "/v1/sandbox/kyc/review", map[string]any{"approve": true}, nil); k["kyc_status"] != "APPROVED" {
		t.Fatalf("review = %v", k)
	}

	// Accounts take the verified identity; someone else's CPF is refused.
	if status, body := ana.call("POST", "/v1/accounts", map[string]string{"document": "529.982.247-25"}, idem()); status != 422 || errCode(body) != "DOCUMENT_MISMATCH" {
		t.Fatalf("other CPF: %d %v", status, body)
	}
	status, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Someone Else"}, idem())
	if status != 201 || acc["holder_name"] != "Ana Souza" {
		t.Fatalf("account: %d %v", status, acc)
	}

	// One person, one customer.
	impostor := signup(t, "eve-kyc-"+uuid.NewString()[:8])
	profile["full_name"] = "Eve Souza"
	impostor.call("PUT", "/v1/me/profile", profile, nil)
	if status, body := impostor.call("POST", "/v1/me/kyc", nil, nil); status != 422 || errCode(body) != "DOCUMENT_IN_USE" {
		t.Fatalf("same CPF, another customer: %d %v", status, body)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		_, l := ana.call("GET", "/v1/notifications", nil, nil)
		found := false
		for _, n := range l["notifications"].([]any) {
			if n.(map[string]any)["title"] == "Cadastro aprovado" {
				found = true
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("notifications = %v", l)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// cpfWithDigits completes 9 digits with the two CPF check digits.
func cpfWithDigits(nine string) string {
	d := make([]int, 11)
	for i := 0; i < 9; i++ {
		d[i] = int(nine[i] - '0')
	}
	for _, n := range []int{9, 10} {
		sum := 0
		for i := 0; i < n; i++ {
			sum += d[i] * (n + 1 - i)
		}
		if d[n] = sum * 10 % 11; d[n] == 10 {
			d[n] = 0
		}
	}
	out := make([]byte, 11)
	for i, v := range d {
		out[i] = byte('0' + v)
	}
	return string(out)
}
