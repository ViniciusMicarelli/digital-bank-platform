// Command server runs the card-service: gRPC API (issuing, controls and the
// network-facing authorization), orphan-reservation worker, outbox relay and
// the account events consumer.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	cardv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/card/v1"
	creditv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/credit/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kafka"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kms"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/adapters/clients"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/adapters/grpcapi"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/migrations"
)

func main() {
	grpcserver.HealthcheckIfAsked(":9007")
	log := config.Logger("card-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "card-service", log)()
	go telemetry.ServeMetrics(ctx, config.String("METRICS_ADDR", ":9107"), log)

	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL",
		"postgres://bank:bank@localhost:54329/card?sslmode=disable"))
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
	creditConn, err := grpcserver.Dial(config.String("CREDIT_ADDR", "127.0.0.1:9009"))
	if err != nil {
		log.Error("dial credit", "error", err)
		os.Exit(1)
	}
	defer creditConn.Close()

	// Vault master key (PAN encryption, PAN index and CVV keys derive from
	// it), unwrapped by kms-service.
	master, err := kms.MasterKey(ctx, log, pool, "card-service/vault", "CARD_VAULT_KEK")
	if err != nil {
		log.Error("data key", "error", err)
		os.Exit(1)
	}
	keys, err := application.NewKeys(master)
	if err != nil {
		log.Error("vault keys", "error", err)
		os.Exit(1)
	}
	store := pgstore.NewStore(pool)
	app := application.New(store,
		clients.NewAccounts(accountv1.NewAccountServiceClient(accountConn)),
		clients.NewTransactions(txv1.NewTransactionServiceClient(txConn)),
		clients.NewCredit(creditv1.NewCreditServiceClient(creditConn)),
		keys, log)

	brokers := kafka.Brokers(config.String("KAFKA_BROKERS", ""))
	go kafka.RunOutboxRelay(ctx, pool, log, brokers)
	if len(brokers) > 0 {
		defer kafka.Start(ctx, kafka.ConsumerConfig{
			Brokers: brokers, Group: "card-service", Topics: []string{"bank.account.events"},
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
	cardv1.RegisterCardServiceServer(srv, grpcapi.NewServer(app))
	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9007")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
