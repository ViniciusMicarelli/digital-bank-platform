// Command server runs the pix-service: gRPC API, transfer recovery worker,
// outbox relay and the account events consumer.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // schedules use America/Sao_Paulo; the scratch image has no zoneinfo

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kafka"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kms"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/adapters/clients"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/adapters/grpcapi"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/adapters/qrfetch"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/adapters/simulator"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/migrations"
)

func main() {
	grpcserver.HealthcheckIfAsked(":9004")
	log := config.Logger("pix-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "pix-service", log)()
	go telemetry.ServeMetrics(ctx, config.String("METRICS_ADDR", ":9104"), log)

	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL",
		"postgres://bank:bank@localhost:54329/pix?sslmode=disable"))
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

	store := pgstore.NewStore(pool)
	app := application.New(store,
		clients.NewAccounts(accountv1.NewAccountServiceClient(accountConn)),
		clients.NewTransactions(txv1.NewTransactionServiceClient(txConn)),
		simulator.NewDirectory(), simulator.NewSPI(), log)

	// Dynamic QR codes: the signing key derives from a KMS data key.
	if os.Getenv("KMS_ADDR") != "" || os.Getenv("PIX_QR_SEED") != "" {
		seed, err := kms.MasterKey(ctx, log, pool, "pix-service/qr-signing", "PIX_QR_SEED")
		if err != nil {
			log.Error("QR signing key", "error", err)
			os.Exit(1)
		}
		app.SetQRSigner(application.NewQRSigner(seed, config.String("PIX_QR_LOCATION", "pix.digitalbank.example/pix/qr/v2")))
	}
	app.SetQRFetcher(qrfetch.New(config.String("PIX_QR_SCHEME", "https")))
	go app.RunSchedules(ctx, config.Duration("SCHEDULE_INTERVAL", 30*time.Second))

	brokers := kafka.Brokers(config.String("KAFKA_BROKERS", ""))
	go kafka.RunOutboxRelay(ctx, pool, log, brokers)
	if len(brokers) > 0 {
		defer kafka.Start(ctx, kafka.ConsumerConfig{
			Brokers: brokers, Group: "pix-service", Topics: []string{"bank.account.events"},
		}, log, store.HandleAccountEvent)()
	}
	go app.RunRecovery(ctx, config.Duration("RECOVERY_INTERVAL", 5*time.Second), config.Duration("RECOVERY_STALE_AFTER", 10*time.Second))

	// Delete published outbox rows, old inbox rows, expired data.
	go postgres.RunRetention(ctx, pool, log, []postgres.RetentionRule{
		postgres.OutboxRetention(7 * 24 * time.Hour),
		postgres.InboxRetention(30 * 24 * time.Hour),
	}, time.Hour)

	srvOpts, err := grpcserver.OptionsFromEnv()
	if err != nil {
		log.Error("tls", "error", err)
		os.Exit(1)
	}
	srv := grpcserver.New(log, srvOpts)
	pixv1.RegisterPixServiceServer(srv, grpcapi.NewServer(app))
	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9004")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
