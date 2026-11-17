package ratelimit

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func redisClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6380" // docker compose
	}
	c := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		if os.Getenv("PGTEST_REQUIRED") == "1" {
			t.Fatalf("redis unavailable: %v", err)
		}
		t.Skipf("redis unavailable (run `make up`): %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestRedisLimiterIsSharedAcrossReplicas(t *testing.T) {
	c := redisClient(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// Two gateway replicas, same limit: 1 request/s, burst 5.
	a := NewRedis(c, "test:", 1, 5, log)
	b := NewRedis(c, "test:", 1, 5, log)
	key := uuid.NewString()
	ctx := context.Background()

	allowed := 0
	for i := range 20 {
		lim := a
		if i%2 == 1 {
			lim = b
		}
		if lim.Allow(ctx, key) {
			allowed++
		}
	}
	if allowed != 5 {
		t.Fatalf("allowed %d of 20 across replicas, want the burst (5)", allowed)
	}
	if !a.Allow(ctx, uuid.NewString()) {
		t.Fatal("other keys are independent")
	}
	time.Sleep(1100 * time.Millisecond)
	if !b.Allow(ctx, key) {
		t.Fatal("the bucket must refill at the steady rate")
	}
}

func TestRedisLimiterFailsOpen(t *testing.T) {
	c := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 20 * time.Millisecond})
	defer c.Close()
	l := NewRedis(c, "test:", 1, 1, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for range 3 {
		if !l.Allow(context.Background(), "k") {
			t.Fatal("must allow requests while Redis is down")
		}
	}
}

func TestMemoryLimiter(t *testing.T) {
	m := NewMemory(1, 3)
	n := 0
	for range 10 {
		if m.Allow(context.Background(), "k") {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("allowed %d, want 3", n)
	}
}
