// Command server runs the public REST API gateway.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	cardv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/card/v1"
	creditv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/credit/v1"
	identityv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/identity/v1"
	paymentv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/payment/v1"
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
	statementv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/statement/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/api-gateway/internal/auth"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/api-gateway/internal/httpapi"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/api-gateway/internal/ratelimit"
)

// limiters shares rate limits through Redis when REDIS_ADDR is set, so every
// replica enforces the same budget; otherwise each replica counts alone.
func limiters(log *slog.Logger) httpapi.Options {
	rps := float64(config.Int("RATE_LIMIT_RPS", 20))
	authRPS := float64(config.Int("AUTH_RATE_LIMIT_RPS", 2))
	opts := httpapi.Options{Sandbox: config.String("SANDBOX", "false") == "true"}
	if addr := config.String("REDIS_ADDR", ""); addr != "" {
		client := redis.NewClient(&redis.Options{Addr: addr})
		opts.Limiter = ratelimit.NewRedis(client, "rl:", rps, int(rps*2), log)
		opts.AuthLimiter = ratelimit.NewRedis(client, "rl:", authRPS, int(authRPS*5), log)
		log.Info("rate limits shared through redis", "addr", addr)
		return opts
	}
	opts.Limiter = ratelimit.NewMemory(rps, int(rps*2))
	opts.AuthLimiter = ratelimit.NewMemory(authRPS, int(authRPS*5))
	return opts
}

func main() {
	grpcserver.HTTPHealthcheckIfAsked(":8080")
	log := config.Logger("api-gateway")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer telemetry.SetupTracing(ctx, "api-gateway", log)()

	dial := func(env, def string) *grpc.ClientConn {
		conn, err := grpcserver.Dial(config.String(env, def))
		if err != nil {
			log.Error("dial", "target", env, "error", err)
			os.Exit(1)
		}
		return conn
	}
	identity := dial("IDENTITY_ADDR", "127.0.0.1:9006")
	accounts := dial("ACCOUNT_ADDR", "127.0.0.1:9002")
	txs := dial("TRANSACTION_ADDR", "127.0.0.1:9003")
	pix := dial("PIX_ADDR", "127.0.0.1:9004")
	statement := dial("STATEMENT_ADDR", "127.0.0.1:9005")
	cards := dial("CARD_ADDR", "127.0.0.1:9007")
	credit := dial("CREDIT_ADDR", "127.0.0.1:9009")
	payments := dial("PAYMENT_ADDR", "127.0.0.1:9010")

	identityClient := identityv1.NewIdentityServiceClient(identity)
	verifier := auth.NewVerifier(func(ctx context.Context) ([]auth.JWK, error) {
		resp, err := identityClient.GetJwks(ctx, &identityv1.GetJwksRequest{})
		if err != nil {
			return nil, err
		}
		keys := make([]auth.JWK, len(resp.GetKeys()))
		for i, k := range resp.GetKeys() {
			keys[i] = auth.JWK{Kid: k.GetKid(), Kty: k.GetKty(), Crv: k.GetCrv(), X: k.GetX()}
		}
		return keys, nil
	})

	api := httpapi.New(httpapi.Clients{
		Identity:  identityClient,
		Accounts:  accountv1.NewAccountServiceClient(accounts),
		Txs:       txv1.NewTransactionServiceClient(txs),
		Pix:       pixv1.NewPixServiceClient(pix),
		Statement: statementv1.NewStatementServiceClient(statement),
		Cards:     cardv1.NewCardServiceClient(cards),
		Credit:    creditv1.NewCreditServiceClient(credit),
		Payments:  paymentv1.NewPaymentServiceClient(payments),
	}, verifier, log, limiters(log))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/", otelhttp.NewHandler(api.Handler(), "api-gateway"))
	srv := &http.Server{
		Addr:              config.String("HTTP_ADDR", ":8080"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("http server listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "error", err)
		os.Exit(1)
	}
}
