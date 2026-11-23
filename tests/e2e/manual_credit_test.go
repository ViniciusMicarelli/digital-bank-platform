//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
)

func TestManualCredit(t *testing.T) {
	ana := login(t, "ana-ajuste-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	id := acc["id"].(string)
	staff := analyst(t, analystEmail, analystSecret)
	credits := "/v1/backoffice/accounts/" + id + "/credits"

	if status, body := ana.call("POST", credits, map[string]string{"amount": "25.00", "reason": "x"}, nil); status != 403 {
		t.Fatalf("a customer crediting herself: %d %v", status, body)
	}
	if status, body := staff.call("POST", credits, map[string]string{"amount": "25.00"}, nil); status != 400 || errCode(body) != "REASON_REQUIRED" {
		t.Fatalf("no reason: %d %v", status, body)
	}
	if status, body := staff.call("POST", credits, map[string]string{"amount": "50000.01", "reason": "demais"}, nil); status != 400 {
		t.Fatalf("over the cap: %d %v", status, body)
	}
	status, approval := staff.call("POST", credits, map[string]string{"amount": "25.00", "reason": "Estorno da tarifa cobrada por engano."}, nil)
	if status != 202 || approval["kind"] != "MANUAL_CREDIT" || approval["status"] != "PENDING" {
		t.Fatalf("request: %d %v", status, approval)
	}
	approve := "/v1/backoffice/approvals/" + approval["id"].(string) + "/approve"
	if status, body := staff.call("POST", approve, map[string]string{}, nil); status != 422 || errCode(body) != "SAME_ANALYST" {
		t.Fatalf("the same analyst approving: %d %v", status, body)
	}
	_, bal := ana.call("GET", "/v1/accounts/"+id+"/balance", nil, nil)
	if amountOf(bal["available"]) != "0.00" {
		t.Fatalf("money before the second analyst: %v", bal)
	}
	reviewer := analyst(t, reviewerEmail, reviewerSecret)
	if status, done := reviewer.call("POST", approve, map[string]string{}, nil); status != 200 || done["status"] != "EXECUTED" {
		t.Fatalf("second analyst: %d %v", status, done)
	}
	_, bal = ana.call("GET", "/v1/accounts/"+id+"/balance", nil, nil)
	if amountOf(bal["available"]) != "25.00" {
		t.Fatalf("balance after the credit = %v", bal)
	}

	// The customer sees it in the statement and is told.
	deadline := time.Now().Add(20 * time.Second)
	for {
		_, st := ana.call("GET", "/v1/accounts/"+id+"/statement", nil, nil)
		_, n := ana.call("GET", "/v1/notifications", nil, nil)
		var inStatement, notified bool
		for _, it := range st["items"].([]any) {
			inStatement = inStatement || it.(map[string]any)["type"] == "MANUAL_CREDIT"
		}
		for _, x := range n["notifications"].([]any) {
			notified = notified || x.(map[string]any)["title"] == "Crédito do Digital Bank"
		}
		if inStatement && notified {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("statement %v / notifications %v", st, n)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// Being allowed to call transaction-service is not enough to create money:
// only savings-service may pay yields and only backoffice-service may post
// manual credits.
func TestBankFundedTypesNeedTheirService(t *testing.T) {
	s := connect(t)
	alice := openAccount(t, s, "Alice Souza", "529.982.247-25")
	for _, typ := range []txv1.TransactionType{txv1.TransactionType_TRANSACTION_TYPE_MANUAL_CREDIT, txv1.TransactionType_TRANSACTION_TYPE_SAVINGS_YIELD} {
		_, err := s.tx.CreateTransfer(ctx(t), &txv1.CreateTransferRequest{IdempotencyKey: uuid.NewString(), Type: typ,
			FromAccountId: "00000000-0000-0000-0000-000000000008", ToAccountId: alice, Amount: brl(1_000_00)})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("%s from dev-client: %v", typ, err)
		}
	}
}
