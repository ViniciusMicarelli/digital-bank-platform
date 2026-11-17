// Command server runs the public REST API gateway.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	identityv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/identity/v1"
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
	statementv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/statement/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/config"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/api-gateway/internal/auth"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/api-gateway/internal/httpapi"
)

func main() {
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
	}, verifier, log, httpapi.Options{
		Sandbox:       config.String("SANDBOX", "false") == "true",
		RateLimit:     float64(config.Int("RATE_LIMIT_RPS", 20)),
		AuthRateLimit: float64(config.Int("AUTH_RATE_LIMIT_RPS", 2)),
	})

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
