// Package telemetry sets up tracing (OpenTelemetry, OTLP export) and metrics
// (Prometheus) for a service.
package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// SetupTracing exports traces over OTLP/gRPC when OTEL_EXPORTER_OTLP_ENDPOINT
// is set (e.g. "jaeger:4317"); otherwise tracing stays a no-op. W3C trace
// context is propagated in both cases. Call the returned func on shutdown.
func SetupTracing(ctx context.Context, service string, log *slog.Logger) func() {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return func() {}
	}
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		log.Warn("tracing disabled", "error", err)
		return func() {}
	}
	res, _ := resource.Merge(resource.Default(), resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(service)))
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp), sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)
	log.Info("tracing enabled", "endpoint", endpoint)
	return func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(c)
	}
}

// ServeMetrics exposes /metrics on addr until ctx ends. An empty addr disables it.
func ServeMetrics(ctx context.Context, addr string, log *slog.Logger) {
	if addr == "" {
		return
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	log.Info("metrics listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("metrics server", "error", err)
	}
}

// Shared metrics. Labels stay low-cardinality (no ids).
var (
	RPCHandled = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_server_handled_total", Help: "gRPC requests by method and status code.",
	}, []string{"method", "code"})
	RPCDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "grpc_server_handling_seconds", Help: "gRPC request latency.",
		Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5},
	}, []string{"method"})
	OutboxPublished = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_published_total", Help: "Outbox events published to Kafka.",
	})
	OutboxErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_publish_errors_total", Help: "Failed outbox publish batches.",
	})
	EventsConsumed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "events_consumed_total", Help: "Kafka events handled, by type and outcome (ok, retry, dead_letter).",
	}, []string{"type", "outcome"})
	BusinessEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bank_business_events_total", Help: "Domain outcomes, e.g. transaction.completed by type.",
	}, []string{"event", "kind"})
)
