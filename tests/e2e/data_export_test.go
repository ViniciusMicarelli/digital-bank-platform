//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestDataExport(t *testing.T) {
	ana := login(t, "ana-dados-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "50.00"}, nil)
	_, card := ana.call("POST", base+"/cards", map[string]string{"kind": "VIRTUAL"}, idem())
	_, details := ana.call("POST", base+"/cards/"+card["id"].(string)+"/reveal", nil, ana.stepUp())

	if status, _ := ana.call("POST", "/v1/me/data-export", nil, nil); status != 401 && status != 403 {
		t.Fatalf("export without the password again: %d", status)
	}
	status, data := ana.call("POST", "/v1/me/data-export", nil, ana.stepUp())
	if status != 200 {
		t.Fatalf("export: %d %v", status, data)
	}
	profile := data["profile"].(map[string]any)
	if cpf, _ := profile["cpf"].(string); len(cpf) != 11 || profile["full_name"] != "Ana Souza" {
		t.Fatalf("profile = %v", profile)
	}
	accounts := data["accounts"].([]any)
	a := accounts[0].(map[string]any)
	if len(accounts) != 1 || len(a["pix_keys"].([]any)) != 1 || len(a["cards"].([]any)) != 1 || len(data["sessions"].([]any)) == 0 {
		t.Fatalf("export = %v", data)
	}
	raw, _ := json.Marshal(data)
	if strings.Contains(string(raw), details["pan"].(string)) || strings.Contains(string(raw), `"cvv"`) {
		t.Fatal("the card number or CVV leaked into the export")
	}
}
