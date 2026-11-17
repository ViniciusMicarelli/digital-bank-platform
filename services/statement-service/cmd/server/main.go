// Command server runs the statement-service: Kafka consumers that project the
// feed, and the gRPC API that serves it.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	statementv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/statement/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kafka"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/statement-service/internal/grpcapi"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/statement-service/internal/projection"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/statement-service/migrations"
)

func main() {
	log := config.Logger("statement-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL",
		"postgres://bank:bank@localhost:54329/statement?sslmode=disable"))
	if err != nil {
		log.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool, migrations.FS, "."); err != nil {
		log.Error("migrate", "error", err)
		os.Exit(1)
	}

	p := projection.New(pool)
	if brokers := kafka.Brokers(config.String("KAFKA_BROKERS", "")); len(brokers) > 0 {
		go func() {
			err := kafka.Consume(ctx, kafka.ConsumerConfig{
				Brokers: brokers,
				Group:   config.String("CONSUMER_GROUP", "statement-service"),
				Topics:  []string{"bank.account.events", "bank.transaction.events"},
			}, log, p.Handle)
			if err != nil {
				log.Error("consumer", "error", err)
			}
		}()
	} else {
		log.Warn("KAFKA_BROKERS not set: the feed will not be updated")
	}

	srv := grpcserver.New(log, grpcserver.Options{})
	statementv1.RegisterStatementServiceServer(srv, grpcapi.NewServer(p))
	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9005")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
