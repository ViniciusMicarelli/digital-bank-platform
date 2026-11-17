// Package correlation carries a request's correlation ID through contexts,
// gRPC metadata and events, so one customer action can be traced across services.
package correlation

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// MetadataKey is the gRPC metadata / HTTP header carrying the ID.
const MetadataKey = "x-correlation-id"

type ctxKey struct{}

// WithID returns a context carrying id.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// ID returns the correlation ID in ctx, or "" if there is none.
func ID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Ensure returns ctx with a correlation ID, generating one if missing.
func Ensure(ctx context.Context) context.Context {
	if ID(ctx) != "" {
		return ctx
	}
	return WithID(ctx, uuid.NewString())
}

// ServerInterceptor reads the ID from incoming metadata (or creates one).
func ServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if v := md.Get(MetadataKey); len(v) > 0 && v[0] != "" {
				return handler(WithID(ctx, v[0]), req)
			}
		}
		return handler(Ensure(ctx), req)
	}
}

// ClientInterceptor forwards the ID to downstream services.
func ClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if id := ID(ctx); id != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, MetadataKey, id)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
