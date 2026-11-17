// Package grpcserver runs a gRPC server with the defaults every service needs:
// panic recovery, request logging, health checks, reflection and graceful
// shutdown. It also builds clients for service-to-service calls.
package grpcserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/correlation"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/telemetry"
)

// Options customizes the server. Interceptors run after recovery and logging.
type Options struct {
	UnaryInterceptors []grpc.UnaryServerInterceptor
	ServerOptions     []grpc.ServerOption
}

// New builds a server with tracing (OpenTelemetry), correlation IDs, panic
// recovery, Prometheus metrics and request logging, plus the standard health
// service and reflection (for grpcurl).
func New(log *slog.Logger, opts Options) *grpc.Server {
	interceptors := append([]grpc.UnaryServerInterceptor{
		correlation.ServerInterceptor(),
		recoverUnary(log),
		metricsUnary(),
		logUnary(log),
	}, opts.UnaryInterceptors...)
	srv := grpc.NewServer(append(opts.ServerOptions,
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(interceptors...))...)

	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(srv, hs)
	reflection.Register(srv)
	return srv
}

// Serve listens on addr and serves until ctx is cancelled, then drains
// in-flight requests (up to 10s) before returning.
func Serve(ctx context.Context, log *slog.Logger, srv *grpc.Server, addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("grpcserver: listen %s: %w", addr, err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(lis) }()
	log.Info("grpc server listening", "addr", lis.Addr().String())

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	log.Info("grpc server shutting down")
	done := make(chan struct{})
	go func() { srv.GracefulStop(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		srv.Stop()
	}
	return nil
}

// Dial creates a client connection to another service.
// Transport security (mTLS) is added at the mesh layer, see docs/architecture/security.md.
func Dial(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts = append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// No service config via DNS TXT records: the lookup times out on
		// Docker's resolver and delayed every first call by ~8 s.
		grpc.WithDisableServiceConfig(),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(correlation.ClientInterceptor()),
	}, opts...)
	return grpc.NewClient(target, opts...)
}

func recoverUnary(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				log.ErrorContext(ctx, "panic in handler", "method", info.FullMethod, "panic", r, "stack", string(debug.Stack()))
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

func metricsUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		telemetry.RPCHandled.WithLabelValues(info.FullMethod, status.Code(err).String()).Inc()
		telemetry.RPCDuration.WithLabelValues(info.FullMethod).Observe(time.Since(start).Seconds())
		return resp, err
	}
}

func logUnary(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		code := status.Code(err)
		level := slog.LevelInfo
		switch code {
		case codes.OK, codes.NotFound, codes.InvalidArgument, codes.FailedPrecondition, codes.AlreadyExists:
		default:
			level = slog.LevelError
		}
		attrs := []any{"method", info.FullMethod, "code", code.String(), "duration_ms", time.Since(start).Milliseconds(),
			"correlation_id", correlation.ID(ctx)}
		if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
			attrs = append(attrs, "trace_id", sc.TraceID().String())
		}
		if err != nil {
			attrs = append(attrs, "error", status.Convert(err).Message())
		}
		log.Log(ctx, level, "grpc request", attrs...)
		return resp, err
	}
}
