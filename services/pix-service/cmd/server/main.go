// Command server runs the pix-service: gRPC API, transfer recovery worker,
// outbox relay and the account events consumer.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kafka"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/adapters/clients"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/adapters/grpcapi"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/adapters/simulator"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/migrations"
)

func main() {
	log := config.Logger("pix-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	brokers := kafka.Brokers(config.String("KAFKA_BROKERS", ""))
	go kafka.RunOutboxRelay(ctx, pool, log, brokers)
	if len(brokers) > 0 {
		go func() {
			err := kafka.Consume(ctx, kafka.ConsumerConfig{
				Brokers: brokers, Group: "pix-service", Topics: []string{"bank.account.events"},
			}, log, store.HandleAccountEvent)
			if err != nil {
				log.Error("account events consumer", "error", err)
			}
		}()
	}
	go app.RunRecovery(ctx, config.Duration("RECOVERY_INTERVAL", 5*time.Second), config.Duration("RECOVERY_STALE_AFTER", 10*time.Second))

	srv := grpcserver.New(log, grpcserver.Options{})
	pixv1.RegisterPixServiceServer(srv, grpcapi.NewServer(app))
	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9004")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
