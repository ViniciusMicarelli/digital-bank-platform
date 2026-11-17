// Command server runs the identity-service.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	identityv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/identity/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kafka"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/secretbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/internal/adapters/grpcapi"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/identity-service/migrations"
)

func main() {
	log := config.Logger("identity-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "identity-service", log)()
	go telemetry.ServeMetrics(ctx, config.String("METRICS_ADDR", ":9106"), log)

	// Key-encryption key for signing keys and TOTP seeds: 32 random bytes,
	// base64. In production it comes from a KMS / secret manager.
	box, err := secretbox.FromBase64(config.String("IDENTITY_KEK", ""))
	if err != nil {
		log.Error("IDENTITY_KEK", "error", err)
		os.Exit(1)
	}

	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL",
		"postgres://bank:bank@localhost:54329/identity?sslmode=disable"))
	if err != nil {
		log.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool, migrations.FS, "."); err != nil {
		log.Error("migrate", "error", err)
		os.Exit(1)
	}

	app := application.New(pgstore.NewStore(pool), box, log)
	if err := app.EnsureSigningKey(ctx); err != nil {
		log.Error("signing key", "error", err)
		os.Exit(1)
	}
	go app.RunKeyRotation(ctx, config.Duration("SIGNING_KEY_MAX_AGE", 30*24*time.Hour))
	go kafka.RunOutboxRelay(ctx, pool, log, kafka.Brokers(config.String("KAFKA_BROKERS", "")))

	srv := grpcserver.New(log, grpcserver.Options{})
	identityv1.RegisterIdentityServiceServer(srv, grpcapi.NewServer(app))
	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9006")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
