// Command server runs the account-service gRPC server.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // limits use America/Sao_Paulo; the scratch image has no zoneinfo

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	ledgerv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/ledger/v1"
	userv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/user/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kafka"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kms"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/adapters/grpcapi"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/adapters/ledger"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/adapters/users"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/account-service/migrations"
)

func main() {
	grpcserver.HealthcheckIfAsked(":9002")
	log := config.Logger("account-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "account-service", log)()
	go telemetry.ServeMetrics(ctx, config.String("METRICS_ADDR", ":9102"), log)

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

	ledgerConn, err := grpcserver.Dial(config.String("LEDGER_ADDR", "127.0.0.1:9001"))
	if err != nil {
		log.Error("dial ledger", "error", err)
		os.Exit(1)
	}
	defer ledgerConn.Close()

	// Data key for CPFs at rest (ADR-0015), unwrapped by kms-service.
	kek, err := kms.MasterKey(ctx, log, pool, "account-service/documents", "ACCOUNT_KEK")
	if err != nil {
		log.Error("data key", "error", err)
		os.Exit(1)
	}
	store, err := pgstore.NewStore(pool, kek)
	if err != nil {
		log.Error("store", "error", err)
		os.Exit(1)
	}
	app := application.New(store, ledger.New(ledgerv1.NewLedgerServiceClient(ledgerConn)), nil)
	// Accounts only for customers whose identity user-service verified (KYC).
	if addr := config.String("USER_ADDR", ""); addr != "" {
		userConn, err := grpcserver.Dial(addr)
		if err != nil {
			log.Error("dial users", "error", err)
			os.Exit(1)
		}
		defer userConn.Close()
		app.RequireKYC(users.New(userv1.NewUserServiceClient(userConn)))
	}
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
	accountv1.RegisterAccountServiceServer(srv, grpcapi.NewServer(app))

	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9002")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
