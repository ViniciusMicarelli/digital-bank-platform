// Command server runs the credit-service: gRPC API (credit lines, invoices,
// payments and the card-facing holds), the invoice cycle worker and the
// outbox relay.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	creditv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/credit/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kafka"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/adapters/clients"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/adapters/grpcapi"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/credit-service/migrations"
)

func main() {
	grpcserver.HealthcheckIfAsked(":9009")
	log := config.Logger("credit-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "credit-service", log)()
	go telemetry.ServeMetrics(ctx, config.String("METRICS_ADDR", ":9109"), log)

	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL",
		"postgres://bank:bank@localhost:54329/credit?sslmode=disable"))
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
		log.Error("dial accounts", "error", err)
		os.Exit(1)
	}
	defer accountConn.Close()
	txConn, err := grpcserver.Dial(config.String("TRANSACTION_ADDR", "127.0.0.1:9003"))
	if err != nil {
		log.Error("dial transactions", "error", err)
		os.Exit(1)
	}
	defer txConn.Close()

	app := application.New(pgstore.NewStore(pool),
		clients.NewAccounts(accountv1.NewAccountServiceClient(accountConn)),
		clients.NewTransactions(txv1.NewTransactionServiceClient(txConn)),
		log, int64(config.Int("CREDIT_DEFAULT_LIMIT_CENTS", 1_000_00)))

	go kafka.RunOutboxRelay(ctx, pool, log, kafka.Brokers(config.String("KAFKA_BROKERS", "")))
	go app.Run(ctx, config.Duration("CYCLE_INTERVAL", 30*time.Second))
	go postgres.RunRetention(ctx, pool, log, []postgres.RetentionRule{
		postgres.OutboxRetention(7 * 24 * time.Hour),
	}, time.Hour)

	srvOpts, err := grpcserver.OptionsFromEnv()
	if err != nil {
		log.Error("tls", "error", err)
		os.Exit(1)
	}
	srv := grpcserver.New(log, srvOpts)
	creditv1.RegisterCreditServiceServer(srv, grpcapi.NewServer(app))
	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9009")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
