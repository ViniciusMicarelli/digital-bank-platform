// Command server runs the ledger-service gRPC server.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	ledgerv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/ledger/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kafka"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/adapters/grpcapi"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/ledger-service/migrations"
)

func main() {
	grpcserver.HealthcheckIfAsked(":9001")
	log := config.Logger("ledger-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "ledger-service", log)()
	go telemetry.ServeMetrics(ctx, config.String("METRICS_ADDR", ":9101"), log)

	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL",
		"postgres://bank:bank@localhost:54329/ledger?sslmode=disable"))
	if err != nil {
		log.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool, migrations.FS, "."); err != nil {
		log.Error("migrate", "error", err)
		os.Exit(1)
	}

	app := application.New(pgstore.NewStore(pool), log)
	go kafka.RunOutboxRelay(ctx, pool, log, kafka.Brokers(config.String("KAFKA_BROKERS", "")))
	go app.RunReconciliation(ctx, config.Duration("RECONCILE_INTERVAL", 5*time.Minute))
	go postgres.RunRetention(ctx, pool, log, []postgres.RetentionRule{
		postgres.OutboxRetention(config.Duration("OUTBOX_RETENTION", 7*24*time.Hour)),
	}, time.Hour)

	srvOpts, err := grpcserver.OptionsFromEnv()
	if err != nil {
		log.Error("tls", "error", err)
		os.Exit(1)
	}
	srv := grpcserver.New(log, srvOpts)
	ledgerv1.RegisterLedgerServiceServer(srv, grpcapi.NewServer(app))

	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9001")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
