//go:build e2e

package e2e

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTaxReport(t *testing.T) {
	ana := login(t, "ana-informe-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "500.00"}, nil)
	_, box := ana.call("POST", base+"/savings", map[string]string{"name": "Reserva"}, idem())
	ana.call("POST", base+"/savings/"+box["id"].(string)+"/deposits", map[string]string{"amount": "200.00"}, idem())
	ana.call("POST", "/v1/sandbox/accounts/"+acc["id"].(string)+"/savings/"+box["id"].(string)+"/yield", map[string]int{"days": 30}, nil)

	year := time.Now().In(time.FixedZone("BRT", -3*60*60)).Year()
	status, r := ana.call("GET", base+"/tax-report?year="+strconv.Itoa(year), nil, nil)
	if status != 200 || r["complete"] != false || r["holder"].(map[string]any)["name"] != "Ana Souza" {
		t.Fatalf("this year: %d %v", status, r)
	}
	checking := r["checking"].(map[string]any)
	savings := r["savings"].(map[string]any)
	boxes := savings["boxes"].([]any)
	if amountOf(checking["balance_start"]) != "0.00" || amountOf(checking["balance_end"]) != "300.00" || len(boxes) != 1 {
		t.Fatalf("this year: %v", r)
	}
	b := boxes[0].(map[string]any)
	if b["name"] != "Reserva" || amountOf(b["yielded"]) == "0.00" || amountOf(savings["yielded"]) != amountOf(b["yielded"]) {
		t.Fatalf("box: %v", b)
	}

	// The year before: the account did not exist yet, nothing to declare.
	_, prev := ana.call("GET", base+"/tax-report?year="+strconv.Itoa(year-1), nil, nil)
	if amountOf(prev["checking"].(map[string]any)["balance_end"]) != "0.00" || len(prev["savings"].(map[string]any)["boxes"].([]any)) != 0 {
		t.Fatalf("last year: %v", prev)
	}
	if status, body := ana.call("GET", base+"/tax-report?year="+strconv.Itoa(year+1), nil, nil); status != 400 || errCode(body) != "INVALID_YEAR" {
		t.Fatalf("a future year: %d %v", status, body)
	}
	bruno := login(t, "bruno-informe-"+uuid.NewString()[:8])
	if status, _ := bruno.call("GET", base+"/tax-report", nil, nil); status != 404 {
		t.Fatalf("someone else's report: %d", status)
	}
}
