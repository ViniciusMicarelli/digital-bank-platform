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

func TestReceiptsAndExport(t *testing.T) {
	ana := login(t, "ana-comprovante-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	base := "/v1/accounts/" + acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", base+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "100.00", "payer_name": "Empresa X"}, nil)
	ana.call("POST", base+"/pix/transfers", map[string]string{"key": "maria@outrobanco.com.br", "amount": "12.34", "description": "Presente"}, idem())

	var pix map[string]any
	deadline := time.Now().Add(15 * time.Second)
	for pix == nil {
		_, st := ana.call("GET", base+"/statement", nil, nil)
		for _, it := range st["items"].([]any) {
			if it.(map[string]any)["type"] == "PIX_OUT" {
				pix = it.(map[string]any)
			}
		}
		if pix == nil && time.Now().After(deadline) {
			t.Fatalf("statement = %v", st)
		}
		time.Sleep(200 * time.Millisecond)
	}

	status, rc := ana.call("GET", base+"/statement/"+pix["id"].(string)+"/receipt", nil, nil)
	code, _ := rc["authentication_code"].(string)
	if status != 200 || rc["holder_name"] != "Ana Souza" || code == "" || rc["item"].(map[string]any)["end_to_end_id"] == "" {
		t.Fatalf("receipt: %d %v", status, rc)
	}
	bruno := login(t, "bruno-comprovante-"+uuid.NewString()[:8])
	if status, _ := bruno.call("GET", base+"/statement/"+pix["id"].(string)+"/receipt", nil, nil); status != 404 {
		t.Fatalf("someone else's receipt: %d", status)
	}

	// Anyone can check the code; names come back masked.
	public := &client{t: t, base: addr("GATEWAY_URL", "http://127.0.0.1:8000")}
	status, v := public.call("GET", "/v1/receipts/"+code, nil, nil)
	if status != 200 || v["holder_name"] != "Ana S." || amountOf(v["item"].(map[string]any)["amount"]) != "12.34" {
		t.Fatalf("verify: %d %v", status, v)
	}
	forged := strings.Replace(code, code[:4], "AAAA", 1)
	if status, body := public.call("GET", "/v1/receipts/"+forged, nil, nil); status != 404 || errCode(body) != "RECEIPT_NOT_FOUND" {
		t.Fatalf("forged code: %d %v", status, body)
	}

	// CSV export of today.
	today := time.Now().In(time.FixedZone("BRT", -3*3600)).Format(time.DateOnly)
	req, _ := http.NewRequest("GET", ana.base+base+"/statement.csv?from="+today+"&to="+today, nil)
	req.Header.Set("Authorization", "Bearer "+ana.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	csv := string(body)
	if resp.StatusCode != 200 || !strings.HasPrefix(csv, "\xef\xbb\xbfData;Hora;Tipo") || !strings.Contains(csv, ";Pix enviado;Presente;") ||
		!strings.Contains(csv, ";-12,34;") || !strings.Contains(csv, ";100,00;") {
		t.Fatalf("csv: %d %q", resp.StatusCode, csv)
	}
}
