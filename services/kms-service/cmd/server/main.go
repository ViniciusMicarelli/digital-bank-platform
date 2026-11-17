// Command server runs the kms-service.
package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	kmsv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/kms/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/secretbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/kms-service/internal/grpcapi"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/kms-service/internal/keyring"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/kms-service/migrations"
)

func main() {
	grpcserver.HealthcheckIfAsked(":9008")
	log := config.Logger("kms-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "kms-service", log)()
	go telemetry.ServeMetrics(ctx, config.String("METRICS_ADDR", ":9108"), log)

	// The root key protects the master keys. In production it never leaves
	// an HSM; here it comes from the environment.
	root, err := secretbox.DecodeKey(config.String("KMS_ROOT_KEY", ""))
	if err != nil {
		log.Error("KMS_ROOT_KEY", "error", err)
		os.Exit(1)
	}
	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL", "postgres://bank:bank@localhost:54329/kms?sslmode=disable"))
	if err != nil {
		log.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool, migrations.FS, "."); err != nil {
		log.Error("migrate", "error", err)
		os.Exit(1)
	}
	k, err := keyring.New(pool, root)
	if err != nil {
		log.Error("keyring", "error", err)
		os.Exit(1)
	}
	srvOpts, err := grpcserver.OptionsFromEnv()
	if err != nil {
		log.Error("tls", "error", err)
		os.Exit(1)
	}
	srv := grpcserver.New(log, srvOpts)
	kmsv1.RegisterKmsServiceServer(srv, grpcapi.NewServer(k, strings.Split(config.String("KMS_OPERATORS", "dev-client"), ",")...))
	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9008")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
