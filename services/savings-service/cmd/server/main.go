// Command server runs the savings-service: savings boxes over gRPC, and the
// loop that pays the daily yield and settles pending movements.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	ledgerv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/ledger/v1"
	savingsv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/savings/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/adapters/clients"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/adapters/grpcapi"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/savings-service/migrations"
)

func main() {
	grpcserver.HealthcheckIfAsked(":9014")
	log := config.Logger("savings-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "savings-service", log)()
	go telemetry.ServeMetrics(ctx, config.String("METRICS_ADDR", ":9114"), log)

	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL", "postgres://bank:bank@localhost:54329/savings?sslmode=disable"))
	if err != nil {
		log.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool, migrations.FS, "."); err != nil {
		log.Error("migrate", "error", err)
		os.Exit(1)
	}
	dial := func(env, def string) *grpc.ClientConn {
		conn, err := grpcserver.Dial(config.String(env, def))
		if err != nil {
			log.Error("dial", "target", env, "error", err)
			os.Exit(1)
		}
		return conn
	}
	ledgerConn := dial("LEDGER_ADDR", "127.0.0.1:9001")
	txConn := dial("TRANSACTION_ADDR", "127.0.0.1:9003")
	defer func() { _ = ledgerConn.Close(); _ = txConn.Close() }()

	// The yearly rate paid on boxes (default 10.65%, a CDI-like rate).
	rate := config.Int("SAVINGS_ANNUAL_RATE_BP", 1065)
	app := application.New(pgstore.NewStore(pool), clients.NewLedger(ledgerv1.NewLedgerServiceClient(ledgerConn)),
		clients.NewTransactions(txv1.NewTransactionServiceClient(txConn)), log, rate)
	go app.Run(ctx, config.Duration("WORKER_INTERVAL", 30*time.Second))

	srvOpts, err := grpcserver.OptionsFromEnv()
	if err != nil {
		log.Error("tls", "error", err)
		os.Exit(1)
	}
	srv := grpcserver.New(log, srvOpts)
	savingsv1.RegisterSavingsServiceServer(srv, grpcapi.NewServer(app))
	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9014")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
