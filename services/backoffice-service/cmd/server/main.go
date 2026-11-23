// Command server runs the backoffice-service: the staff audit trail and
// four-eyes approvals (gRPC), and the loop that settles approved actions
// (account-service, credit-service, transaction-service).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	backofficev1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/backoffice/v1"
	creditv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/credit/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/adapters/actions"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/adapters/grpcapi"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/migrations"
)

func main() {
	grpcserver.HealthcheckIfAsked(":9013")
	log := config.Logger("backoffice-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "backoffice-service", log)()
	go telemetry.ServeMetrics(ctx, config.String("METRICS_ADDR", ":9113"), log)

	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL",
		"postgres://bank:bank@localhost:54329/backoffice?sslmode=disable"))
	if err != nil {
		log.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool, migrations.FS, "."); err != nil {
		log.Error("migrate", "error", err)
		os.Exit(1)
	}
	accountConn, err := grpcserver.Dial(config.String("ACCOUNT_ADDR", "127.0.0.1:9002"))
	if err != nil {
		log.Error("dial account-service", "error", err)
		os.Exit(1)
	}
	defer func() { _ = accountConn.Close() }()
	creditConn, err := grpcserver.Dial(config.String("CREDIT_ADDR", "127.0.0.1:9009"))
	if err != nil {
		log.Error("dial credit-service", "error", err)
		os.Exit(1)
	}
	defer func() { _ = creditConn.Close() }()
	txConn, err := grpcserver.Dial(config.String("TRANSACTION_ADDR", "127.0.0.1:9003"))
	if err != nil {
		log.Error("dial transaction-service", "error", err)
		os.Exit(1)
	}
	defer func() { _ = txConn.Close() }()

	app := application.New(pgstore.NewStore(pool),
		actions.New(accountv1.NewAccountServiceClient(accountConn), creditv1.NewCreditServiceClient(creditConn),
			txv1.NewTransactionServiceClient(txConn)), log)
	go app.RunRecovery(ctx, config.Duration("RECOVERY_INTERVAL", 30*time.Second))

	srvOpts, err := grpcserver.OptionsFromEnv()
	if err != nil {
		log.Error("tls", "error", err)
		os.Exit(1)
	}
	srv := grpcserver.New(log, srvOpts)
	backofficev1.RegisterBackOfficeServiceServer(srv, grpcapi.NewServer(app))
	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9013")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
