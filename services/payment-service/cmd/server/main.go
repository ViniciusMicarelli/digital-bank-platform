// Command server runs the payment-service: gRPC API (boleto lookup,
// payments, scheduling), the recovery / scheduled-payments worker and the
// outbox relay.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	paymentv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/payment/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kafka"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/adapters/clients"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/adapters/grpcapi"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/adapters/simulator"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/payment-service/migrations"
)

func main() {
	grpcserver.HealthcheckIfAsked(":9010")
	log := config.Logger("payment-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "payment-service", log)()
	go telemetry.ServeMetrics(ctx, config.String("METRICS_ADDR", ":9110"), log)

	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL",
		"postgres://bank:bank@localhost:54329/payment?sslmode=disable"))
	if err != nil {
		log.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool, migrations.FS, "."); err != nil {
		log.Error("migrate", "error", err)
		os.Exit(1)
	}
	txConn, err := grpcserver.Dial(config.String("TRANSACTION_ADDR", "127.0.0.1:9003"))
	if err != nil {
		log.Error("dial transactions", "error", err)
		os.Exit(1)
	}
	defer txConn.Close()

	store := pgstore.NewStore(pool)
	app := application.New(store, simulator.NewRegistry(store), simulator.NewClearing(),
		clients.NewTransactions(txv1.NewTransactionServiceClient(txConn)), log)

	go kafka.RunOutboxRelay(ctx, pool, log, kafka.Brokers(config.String("KAFKA_BROKERS", "")))
	go app.RunRecovery(ctx, config.Duration("RECOVERY_INTERVAL", 5*time.Second), config.Duration("RECOVERY_STALE_AFTER", 10*time.Second))
	go postgres.RunRetention(ctx, pool, log, []postgres.RetentionRule{
		postgres.OutboxRetention(7 * 24 * time.Hour),
	}, time.Hour)

	srvOpts, err := grpcserver.OptionsFromEnv()
	if err != nil {
		log.Error("tls", "error", err)
		os.Exit(1)
	}
	srv := grpcserver.New(log, srvOpts)
	paymentv1.RegisterPaymentServiceServer(srv, grpcapi.NewServer(app, store))
	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9010")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
