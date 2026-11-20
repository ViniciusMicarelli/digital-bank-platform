//go:build e2e

package e2e

import (
	"testing"

	"github.com/google/uuid"
)

func TestPixContacts(t *testing.T) {
	ana := login(t, "ana-contatos-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "100.00"}, nil)
	for range 2 {
		ana.call("POST", base+"/pix/transfers", map[string]string{"key": "maria@outrobanco.com.br", "amount": "1.00"}, idem())
	}
	_, list := ana.call("GET", base+"/pix/contacts", nil, nil)
	contacts := list["contacts"].([]any)
	if len(contacts) != 1 || contacts[0].(map[string]any)["name"] != "Maria Oliveira" || contacts[0].(map[string]any)["times_sent"] != float64(2) {
		t.Fatalf("contacts = %v", list)
	}
	if status, c := ana.call("PUT", base+"/pix/contacts/favorites", map[string]string{"key": "+5511987654321", "nickname": "Pai"}, nil); status != 200 || c["name"] != "João Pereira" {
		t.Fatalf("favorite: %d %v", status, c)
	}
	if status, body := ana.call("PUT", base+"/pix/contacts/favorites", map[string]string{"key": key}, nil); status != 422 || errCode(body) != "OWN_KEY" {
		t.Fatalf("own key: %d %v", status, body)
	}
	_, list = ana.call("GET", base+"/pix/contacts", nil, nil)
	if first := list["contacts"].([]any)[0].(map[string]any); first["nickname"] != "Pai" || first["favorite"] != true {
		t.Fatalf("favorites first = %v", list)
	}
	if status, _ := ana.call("DELETE", base+"/pix/contacts/favorites?key=%2B5511987654321", nil, nil); status != 204 {
		t.Fatalf("remove: %d", status)
	}
	if _, list = ana.call("GET", base+"/pix/contacts", nil, nil); len(list["contacts"].([]any)) != 1 {
		t.Fatalf("after removing = %v", list)
	}
}
