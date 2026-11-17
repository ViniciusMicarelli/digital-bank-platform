package grpcserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// HealthcheckIfAsked implements `server healthcheck` for container
// healthchecks (the images have no shell or curl). It checks the gRPC health
// service on the port of GRPC_ADDR over the service's own credentials (mTLS
// when configured) and exits 0 when SERVING, 1 otherwise. It returns when the
// process was not started as a healthcheck.
func HealthcheckIfAsked(defaultAddr string) {
	if len(os.Args) < 2 || os.Args[1] != "healthcheck" {
		return
	}
	addr := os.Getenv("GRPC_ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		fail(err)
	}
	conn, err := Dial(net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		fail(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil || resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		fail(fmt.Errorf("not serving: %v", err))
	}
	os.Exit(0)
}

// HTTPHealthcheckIfAsked is the HTTP variant: GET http://127.0.0.1:<port>/healthz.
func HTTPHealthcheckIfAsked(defaultAddr string) {
	if len(os.Args) < 2 || os.Args[1] != "healthcheck" {
		return
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		fail(err)
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz")
	if err != nil {
		fail(err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		fail(fmt.Errorf("status %d", resp.StatusCode))
	}
	os.Exit(0)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "healthcheck:", err)
	os.Exit(1)
}
