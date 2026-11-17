// Command server runs the transaction-service: the saga orchestrator's gRPC
// API plus the recovery worker that resumes interrupted sagas.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	ledgerv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/ledger/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kafka"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/adapters/clients"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/adapters/grpcapi"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/transaction-service/migrations"
)

func main() {
	grpcserver.HealthcheckIfAsked(":9003")
	log := config.Logger("transaction-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "transaction-service", log)()
	go telemetry.ServeMetrics(ctx, config.String("METRICS_ADDR", ":9103"), log)

	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL",
		"postgres://bank:bank@localhost:54329/transaction?sslmode=disable"))
	if err != nil {
		log.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool, migrations.FS, "."); err != nil {
		log.Error("migrate", "error", err)
		os.Exit(1)
	}

	ledgerConn, err := grpcserver.Dial(config.String("LEDGER_ADDR", "127.0.0.1:9001"))
	if err != nil {
		log.Error("dial ledger", "error", err)
		os.Exit(1)
	}
	defer ledgerConn.Close()
	accountConn, err := grpcserver.Dial(config.String("ACCOUNT_ADDR", "127.0.0.1:9002"))
	if err != nil {
		log.Error("dial accounts", "error", err)
		os.Exit(1)
	}
	defer accountConn.Close()

	app := application.New(pgstore.NewStore(pool),
		clients.NewLedger(ledgerv1.NewLedgerServiceClient(ledgerConn)),
		clients.NewAccounts(accountv1.NewAccountServiceClient(accountConn)),
		log)

	go app.RunRecovery(ctx,
		config.Duration("RECOVERY_INTERVAL", 5*time.Second),
		config.Duration("RECOVERY_STALE_AFTER", 10*time.Second),
		application.ReservationTimeouts{
			domain.TypePixOut:    config.Duration("PIX_RESERVATION_TIMEOUT", 15*time.Minute),
			domain.TypeDebitCard: config.Duration("CARD_AUTHORIZATION_TIMEOUT", 7*24*time.Hour),
		})

	go kafka.RunOutboxRelay(ctx, pool, log, kafka.Brokers(config.String("KAFKA_BROKERS", "")))

	// Delete published outbox rows, old inbox rows, expired data.
	go postgres.RunRetention(ctx, pool, log, []postgres.RetentionRule{
		postgres.OutboxRetention(7 * 24 * time.Hour),
	}, time.Hour)

	srvOpts, err := grpcserver.OptionsFromEnv()
	if err != nil {
		log.Error("tls", "error", err)
		os.Exit(1)
	}
	srv := grpcserver.New(log, srvOpts)
	txv1.RegisterTransactionServiceServer(srv, grpcapi.NewServer(app))
	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9003")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
