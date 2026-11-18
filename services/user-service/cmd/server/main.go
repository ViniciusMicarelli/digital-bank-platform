// Command server runs the user-service: gRPC API (profile, KYC), the
// bureau retry worker and the outbox relay.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	userv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/user/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kafka"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kms"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/adapters/bureau"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/adapters/grpcapi"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/migrations"
)

func main() {
	grpcserver.HealthcheckIfAsked(":9012")
	log := config.Logger("user-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "user-service", log)()
	go telemetry.ServeMetrics(ctx, config.String("METRICS_ADDR", ":9112"), log)

	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL",
		"postgres://bank:bank@localhost:54329/users?sslmode=disable"))
	if err != nil {
		log.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool, migrations.FS, "."); err != nil {
		log.Error("migrate", "error", err)
		os.Exit(1)
	}
	// Data key for CPFs at rest, unwrapped by kms-service.
	key, err := kms.MasterKey(ctx, log, pool, "user-service/documents", "USER_KEK")
	if err != nil {
		log.Error("data key", "error", err)
		os.Exit(1)
	}
	store, err := pgstore.NewStore(pool, key)
	if err != nil {
		log.Error("store", "error", err)
		os.Exit(1)
	}
	app := application.New(store, bureau.Simulator{}, log)

	go kafka.RunOutboxRelay(ctx, pool, log, kafka.Brokers(config.String("KAFKA_BROKERS", "")))
	go app.RunRetries(ctx, config.Duration("RETRY_INTERVAL", 10*time.Second), 10*time.Second)
	go postgres.RunRetention(ctx, pool, log, []postgres.RetentionRule{postgres.OutboxRetention(7 * 24 * time.Hour)}, time.Hour)

	srvOpts, err := grpcserver.OptionsFromEnv()
	if err != nil {
		log.Error("tls", "error", err)
		os.Exit(1)
	}
	srv := grpcserver.New(log, srvOpts)
	userv1.RegisterUserServiceServer(srv, grpcapi.NewServer(app))
	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9012")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
