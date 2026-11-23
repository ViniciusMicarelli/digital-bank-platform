//go:build e2e

package e2e

import (
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestStatementSearch(t *testing.T) {
	ana := login(t, "ana-busca-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "100.00"}, nil)
	ana.call("POST", base+"/pix/transfers", map[string]string{"key": "maria@outrobanco.com.br", "amount": "20.00"}, idem())

	items := func(query string) []any {
		_, st := ana.call("GET", base+"/statement?"+query, nil, nil)
		list, _ := st["items"].([]any)
		return list
	}
	deadline := time.Now().Add(20 * time.Second)
	for len(items("")) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("statement = %v", items(""))
		}
		time.Sleep(300 * time.Millisecond)
	}
	if in := items("direction=credit"); len(in) != 1 || in[0].(map[string]any)["direction"] != "CREDIT" {
		t.Fatalf("money in = %v", in)
	}
	out := items("q=" + url.QueryEscape("MARIA"))
	if len(out) != 1 || out[0].(map[string]any)["direction"] != "DEBIT" {
		t.Fatalf("search = %v", out)
	}
	if none := items("q=ninguem&direction=debit"); len(none) != 0 {
		t.Fatalf("no match = %v", none)
	}
	if status, body := ana.call("GET", base+"/statement?direction=sideways", nil, nil); status != 400 {
		t.Fatalf("bad direction: %d %v", status, body)
	}
}
