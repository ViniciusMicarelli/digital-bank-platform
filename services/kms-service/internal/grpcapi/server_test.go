package grpcapi_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	kmsv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/kms/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kms"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/kms-service/internal/grpcapi"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/kms-service/internal/keyring"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/kms-service/migrations"
)

func client(t *testing.T) (kmsv1.KmsServiceClient, *keyring.Keyring) {
	t.Helper()
	k, err := keyring.New(pgtest.NewDB(t, migrations.FS, "."), bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	srv := grpcserver.New(slog.New(slog.NewTextHandler(io.Discard, nil)), grpcserver.Options{})
	kmsv1.RegisterKmsServiceServer(srv, grpcapi.NewServer(k))
	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///kms", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return kmsv1.NewKmsServiceClient(conn), k
}

// No TLS in this test: the caller identity comes from KMS_INSECURE_CALLER.
func TestDataKeyLifecycle(t *testing.T) {
	t.Setenv("KMS_INSECURE_CALLER", "account-service")
	c, k := client(t)
	ctx := context.Background()
	svcDB := pgtest.NewDB(t, migrations.FS, ".") // stands in for the service's own database

	first, err := kms.LoadDataKey(ctx, svcDB, c, "account-service/documents", nil)
	if err != nil || len(first) != 32 {
		t.Fatalf("first load = %d bytes, %v", len(first), err)
	}
	again, _ := kms.LoadDataKey(ctx, svcDB, c, "account-service/documents", nil)
	if !bytes.Equal(first, again) {
		t.Fatal("a restart must get the same data key")
	}
	var stored []byte
	_ = svcDB.QueryRow(ctx, `SELECT wrapped FROM data_keys WHERE name = 'account-service/documents'`).Scan(&stored)
	if bytes.Contains(stored, first) {
		t.Fatal("the service database must hold only the wrapped key")
	}

	legacy := bytes.Repeat([]byte{4}, 32)
	imported, err := kms.LoadDataKey(ctx, svcDB, c, "account-service/legacy", legacy)
	if err != nil || !bytes.Equal(imported, legacy) {
		t.Fatalf("importing an existing key must keep it: %v", err)
	}

	if _, err := k.Rotate(ctx, "account-service/documents"); err != nil {
		t.Fatal(err)
	}
	after, err := kms.LoadDataKey(ctx, svcDB, c, "account-service/documents", nil)
	if err != nil || !bytes.Equal(after, first) {
		t.Fatalf("rotation changes the master key, not the data key: %v", err)
	}
	var rewrapped []byte
	_ = svcDB.QueryRow(ctx, `SELECT wrapped FROM data_keys WHERE name = 'account-service/documents'`).Scan(&rewrapped)
	if bytes.Equal(rewrapped, stored) {
		t.Fatal("the wrapped key must move to the new master key version")
	}
}

func TestCallerCannotUseOtherServicesKeys(t *testing.T) {
	t.Setenv("KMS_INSECURE_CALLER", "api-gateway")
	c, _ := client(t)
	_, err := c.GenerateDataKey(context.Background(), &kmsv1.GenerateDataKeyRequest{KeyName: "card-service/vault"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("err = %v", err)
	}
}
