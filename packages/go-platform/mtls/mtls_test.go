package mtls_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/mtls"
)

// workload writes a CA-signed certificate for name and returns its Config.
func workload(t *testing.T, ca *mtls.Authority, dir, name string) mtls.Config {
	t.Helper()
	cert, key, err := ca.Issue(name, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	c := mtls.Config{CAFile: filepath.Join(dir, "ca.pem"), CertFile: filepath.Join(dir, name+".pem"), KeyFile: filepath.Join(dir, name+"-key.pem")}
	_ = os.WriteFile(c.CAFile, ca.PEM, 0o600)
	_ = os.WriteFile(c.CertFile, cert, 0o600)
	_ = os.WriteFile(c.KeyFile, key, 0o600)
	return c
}

func call(t *testing.T, addr string, creds credentials.TransportCredentials, method string) error {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if method == "health" {
		_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
		return err
	}
	// List is a protected method (only Check is open to every workload).
	_, err = healthpb.NewHealthClient(conn).List(ctx, &healthpb.HealthListRequest{})
	return err
}

func TestMutualTLSAndAuthorization(t *testing.T) {
	dir := t.TempDir()
	ca, _ := mtls.NewAuthority(time.Hour)
	ledger := workload(t, ca, dir, "ledger-service")
	account := workload(t, ca, dir, "account-service")
	gateway := workload(t, ca, dir, "api-gateway")

	serverCreds, err := ledger.ServerCredentials()
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer(grpc.Creds(serverCreds), grpc.ChainUnaryInterceptor(mtls.Authorize([]string{"account-service"})))
	healthpb.RegisterHealthServer(srv, health.NewServer())
	lis, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	addr := lis.Addr().String()

	allowed, _ := account.ClientCredentials(addr)
	if err := call(t, addr, allowed, "list"); err != nil {
		t.Fatalf("allowed caller: %v", err)
	}

	denied, _ := gateway.ClientCredentials(addr)
	if err := call(t, addr, denied, "list"); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("api-gateway calling the ledger: %v", err)
	}
	// Health checks are open to any authenticated workload.
	if err := call(t, addr, denied, "health"); err != nil {
		t.Fatalf("health: %v", err)
	}

	if err := call(t, addr, insecure.NewCredentials(), "list"); err == nil {
		t.Fatal("plaintext client accepted")
	}

	otherCA, _ := mtls.NewAuthority(time.Hour)
	rogue := workload(t, otherCA, t.TempDir(), "account-service")
	rogueCreds, _ := rogue.ClientCredentials(addr)
	if err := call(t, addr, rogueCreds, "list"); err == nil {
		t.Fatal("certificate from another CA accepted")
	}
}
