package grpcapi_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	ledgerv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/ledger/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/adapters/grpcapi"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/migrations"
)

func client(t *testing.T) ledgerv1.LedgerServiceClient {
	t.Helper()
	pool := pgtest.NewDB(t, migrations.FS, ".")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := grpcserver.New(log, grpcserver.Options{})
	ledgerv1.RegisterLedgerServiceServer(srv, grpcapi.NewServer(application.New(pgstore.NewStore(pool), log)))

	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return ledgerv1.NewLedgerServiceClient(conn)
}

func brl(minor int64) *commonv1.Money { return &commonv1.Money{MinorUnits: minor, Currency: "BRL"} }

func TestGRPCFlow(t *testing.T) {
	c := client(t)
	ctx := context.Background()

	acc, err := c.CreateLedgerAccount(ctx, &ledgerv1.CreateLedgerAccountRequest{
		Id: uuid.NewString(), Kind: ledgerv1.AccountKind_ACCOUNT_KIND_CUSTOMER, Currency: "BRL",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := acc.GetAccount().GetId()
	const funding = "00000000-0000-0000-0000-000000000001"

	_, err = c.PostEntry(ctx, &ledgerv1.PostEntryRequest{
		IdempotencyKey: "fund-1",
		Postings: []*ledgerv1.Posting{
			{AccountId: funding, Amount: brl(-2500)},
			{AccountId: id, Amount: brl(2500)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	bal, err := c.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: id})
	if err != nil || bal.GetBalance().GetAvailable().GetMinorUnits() != 2500 {
		t.Fatalf("balance = %v, %v", bal, err)
	}

	_, err = c.PostEntry(ctx, &ledgerv1.PostEntryRequest{
		IdempotencyKey: "too-much",
		Postings: []*ledgerv1.Posting{
			{AccountId: id, Amount: brl(-2501)},
			{AccountId: funding, Amount: brl(2501)},
		},
	})
	if status.Code(err) != codes.FailedPrecondition || grpcserver.Reason(err) != grpcapi.ReasonInsufficientFunds {
		t.Fatalf("overdraw err = %v (reason %q)", err, grpcserver.Reason(err))
	}

	_, err = c.PostEntry(ctx, &ledgerv1.PostEntryRequest{
		IdempotencyKey: "unbalanced",
		Postings:       []*ledgerv1.Posting{{AccountId: id, Amount: brl(-1)}, {AccountId: funding, Amount: brl(2)}},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unbalanced err = %v", err)
	}

	_, err = c.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: uuid.NewString()})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("unknown account err = %v", err)
	}

	page, err := c.ListPostings(ctx, &ledgerv1.ListPostingsRequest{AccountId: id, PageSize: 10})
	if err != nil || len(page.GetPostings()) != 1 || page.GetNextPageToken() != "" {
		t.Fatalf("postings = %v, %v", page, err)
	}
}
