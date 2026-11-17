// Package mtls secures service-to-service gRPC with mutual TLS (ADR-0014).
//
// Every workload holds a certificate whose URI SAN is its identity,
// "spiffe://digital-bank/<name>". Servers require and verify client
// certificates (TLS 1.3 only) and authorize callers by identity, so a
// compromised gateway cannot, for example, post ledger entries directly.
//
// Configuration comes from the environment: TLS_CA, TLS_CERT, TLS_KEY (PEM
// file paths) and ALLOWED_CALLERS (comma-separated identities). Without
// TLS_CERT, services fall back to plaintext (unit tests, `go run`).
package mtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// TrustDomain is the SPIFFE trust domain of every workload identity.
const TrustDomain = "digital-bank"

// SPIFFEID returns the identity URI of a workload.
func SPIFFEID(name string) *url.URL {
	return &url.URL{Scheme: "spiffe", Host: TrustDomain, Path: "/" + name}
}

// Config locates the PEM files of a workload.
type Config struct {
	CAFile, CertFile, KeyFile string
}

// FromEnv reads TLS_CA, TLS_CERT and TLS_KEY. Enabled reports whether they are set.
func FromEnv() Config {
	return Config{CAFile: os.Getenv("TLS_CA"), CertFile: os.Getenv("TLS_CERT"), KeyFile: os.Getenv("TLS_KEY")}
}

func (c Config) Enabled() bool { return c.CertFile != "" }

func (c Config) load() (tls.Certificate, *x509.CertPool, error) {
	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("mtls: load key pair: %w", err)
	}
	caPEM, err := os.ReadFile(c.CAFile)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("mtls: read CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return tls.Certificate{}, nil, errors.New("mtls: no CA certificate found")
	}
	return cert, pool, nil
}

// ServerCredentials require a client certificate signed by the CA.
func (c Config) ServerCredentials() (credentials.TransportCredentials, error) {
	cert, pool, err := c.load()
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}), nil
}

// ClientCredentials present our certificate and verify the server against
// the CA and the target host name.
func (c Config) ClientCredentials(target string) (credentials.TransportCredentials, error) {
	cert, pool, err := c.load()
	if err != nil {
		return nil, err
	}
	host := target
	if h, _, err := net.SplitHostPort(target); err == nil {
		host = h
	}
	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ServerName:   host,
		MinVersion:   tls.VersionTLS13,
	}), nil
}

// PeerIdentity returns the workload name of the verified client certificate.
func PeerIdentity(ctx context.Context) (string, bool) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", false
	}
	info, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(info.State.VerifiedChains) == 0 || len(info.State.VerifiedChains[0]) == 0 {
		return "", false
	}
	for _, u := range info.State.VerifiedChains[0][0].URIs {
		if u.Scheme == "spiffe" && u.Host == TrustDomain {
			return strings.TrimPrefix(u.Path, "/"), true
		}
	}
	return "", false
}

// AllowedCallersFromEnv parses ALLOWED_CALLERS ("api-gateway,pix-service").
func AllowedCallersFromEnv() []string {
	var out []string
	for _, s := range strings.Split(os.Getenv("ALLOWED_CALLERS"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Authorize rejects callers whose identity is not in allowed, except for
// health checks (probes). An empty list allows any authenticated workload.
func Authorize(allowed []string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod == "/grpc.health.v1.Health/Check" {
			return handler(ctx, req)
		}
		id, ok := PeerIdentity(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "client certificate required")
		}
		if len(allowed) > 0 && !slices.Contains(allowed, id) {
			return nil, status.Errorf(codes.PermissionDenied, "%s may not call %s", id, info.FullMethod)
		}
		return handler(ctx, req)
	}
}
