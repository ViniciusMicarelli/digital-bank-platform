// Command server runs the notification-service: the Kafka consumer that
// turns events into notifications, the push / e-mail dispatcher, the
// LISTEN loop that wakes live streams, and the gRPC API.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	notificationv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/notification/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/kafka"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/adapters/clients"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/adapters/grpcapi"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/notification-service/migrations"
)

func main() {
	grpcserver.HealthcheckIfAsked(":9011")
	log := config.Logger("notification-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "notification-service", log)()
	go telemetry.ServeMetrics(ctx, config.String("METRICS_ADDR", ":9111"), log)

	pool, err := postgres.Connect(ctx, config.String("DATABASE_URL",
		"postgres://bank:bank@localhost:54329/notification?sslmode=disable"))
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

	store := pgstore.NewStore(pool)
	app := application.New(store, clients.NewAccounts(accountv1.NewAccountServiceClient(accountConn)), application.LogSender{Log: log}, log)

	if brokers := kafka.Brokers(config.String("KAFKA_BROKERS", "")); len(brokers) > 0 {
		defer kafka.Start(ctx, kafka.ConsumerConfig{
			// A new deployment notifies what happens from (just before) now on.
			Brokers: brokers, Group: "notification-service", NewGroupLookback: 10 * time.Minute,
			Topics: []string{"bank.account.events", "bank.transaction.events", "bank.pix.events", "bank.card.events",
				"bank.credit.events", "bank.payment.events"},
		}, log, app.HandleEvent)()
	}
	go store.Listen(ctx, log, app.Wake)
	go app.RunDeliveries(ctx, config.Duration("DELIVERY_INTERVAL", time.Second))
	go postgres.RunRetention(ctx, pool, log, []postgres.RetentionRule{
		postgres.InboxRetention(30 * 24 * time.Hour),
	}, time.Hour)

	srvOpts, err := grpcserver.OptionsFromEnv()
	if err != nil {
		log.Error("tls", "error", err)
		os.Exit(1)
	}
	srv := grpcserver.New(log, srvOpts)
	notificationv1.RegisterNotificationServiceServer(srv, grpcapi.NewServer(app, config.Duration("STREAM_POLL_INTERVAL", 15*time.Second)))
	if err := grpcserver.Serve(ctx, log, srv, config.String("GRPC_ADDR", ":9011")); err != nil {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
