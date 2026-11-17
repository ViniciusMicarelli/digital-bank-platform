// Command server runs the account-service gRPC server.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // limits use America/Sao_Paulo; the scratch image has no zoneinfo

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	ledgerv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/ledger/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/adapters/grpcapi"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/adapters/ledger"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/migrations"
)

func main() {
	log := config.Logger("account-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL",
		"postgres://bank:bank@localhost:54329/account?sslmode=disable"))
	if err != nil {
		log.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool, migrations.FS, "."); err != nil {
		log.Error("migrate", "error", err)
		os.Exit(1)
	}

	ledgerConn, err := grpcserver.Dial(config.String("LEDGER_ADDR", "localhost:9001"))
	if err != nil {
		log.Error("dial ledger", "error", err)
		os.Exit(1)
	}
	defer ledgerConn.Close()

	app := application.New(pgstore.NewStore(pool), ledger.New(ledgerv1.NewLedgerServiceClient(ledgerConn)), nil)
	srv := grpcserver.New(log, grpcserver.Options{})
	accountv1.RegisterAccountServiceServer(srv, grpcapi.NewServer(app))

	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9002")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
