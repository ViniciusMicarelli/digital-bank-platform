//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	ledgerv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/ledger/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/mtls"
)

// The ledger moves money: only account-service and transaction-service may
// call it. Neither plaintext clients nor the gateway's identity get in.
func TestServiceToServiceAuthorization(t *testing.T) {
	ca := os.Getenv("TLS_CA")
	if ca == "" {
		t.Skip("stack runs without mTLS")
	}
	target := addr("LEDGER_ADDR", "127.0.0.1:9001")
	getAccount := func(conn *grpc.ClientConn) error {
		_, err := ledgerv1.NewLedgerServiceClient(conn).GetLedgerAccount(ctx(t), &ledgerv1.GetLedgerAccountRequest{Id: bankFunding})
		return err
	}

	plain, _ := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer plain.Close()
	if err := getAccount(plain); err == nil {
		t.Fatal("plaintext client reached the ledger")
	}

	dir := filepath.Dir(ca)
	gateway := mtls.Config{CAFile: ca, CertFile: filepath.Join(dir, "api-gateway.pem"), KeyFile: filepath.Join(dir, "api-gateway-key.pem")}
	creds, err := gateway.ClientCredentials(target)
	if err != nil {
		t.Fatal(err)
	}
	asGateway, _ := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
	defer asGateway.Close()
	if err := getAccount(asGateway); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("api-gateway calling the ledger: %v", err)
	}

	if _, err := connect(t).ledger.GetLedgerAccount(ctx(t), &ledgerv1.GetLedgerAccountRequest{Id: bankFunding}); err != nil {
		t.Fatalf("dev-client (allowed): %v", err)
	}
}
