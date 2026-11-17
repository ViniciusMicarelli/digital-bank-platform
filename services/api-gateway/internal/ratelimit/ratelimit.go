// Package ratelimit limits requests per key (customer, client IP) with the
// generic cell rate algorithm (GCRA): a steady rate plus a burst.
//
// The Redis implementation is shared by every gateway replica. If Redis is
// unreachable it fails open (requests pass, the error is counted): losing
// rate limiting for a moment is better than losing the API.
package ratelimit

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// Limiter decides whether one more request for key is allowed now.
type Limiter interface {
	Allow(ctx context.Context, key string) bool
}

// Memory is a per-process token bucket per key.
type Memory struct {
	mu      sync.Mutex
	buckets map[string]*rate.Limiter
	rps     rate.Limit
	burst   int
}

func NewMemory(rps float64, burst int) *Memory {
	return &Memory{buckets: map[string]*rate.Limiter{}, rps: rate.Limit(rps), burst: burst}
}

func (m *Memory) Allow(_ context.Context, key string) bool {
	m.mu.Lock()
	b, ok := m.buckets[key]
	if !ok {
		b = rate.NewLimiter(m.rps, m.burst)
		m.buckets[key] = b
	}
	m.mu.Unlock()
	return b.Allow()
}

var redisErrors = promauto.NewCounter(prometheus.CounterOpts{
	Name: "ratelimit_redis_errors_total", Help: "Rate limit checks that failed open because Redis was unavailable.",
})

// gcra stores the "theoretical arrival time" (TAT) per key, in microseconds,
// using Redis' own clock so replicas with skewed clocks agree.
// A request is allowed when the new TAT is within burst*interval of now.
var gcra = redis.NewScript(`
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000000 + tonumber(t[2])
local interval = tonumber(ARGV[1])
local tolerance = tonumber(ARGV[2])
local tat = tonumber(redis.call('GET', KEYS[1]) or now)
if tat < now then tat = now end
local new_tat = tat + interval
if new_tat - now > tolerance then
  return 0
end
redis.call('SET', KEYS[1], new_tat, 'PX', math.ceil((new_tat - now) / 1000) + 1)
return 1
`)

// Redis is a GCRA limiter shared through Redis.
type Redis struct {
	client    *redis.Client
	prefix    string
	interval  int64 // microseconds between requests at the steady rate
	tolerance int64 // burst * interval
	log       *slog.Logger
}

// NewRedis allows rps requests per second per key, with bursts of up to burst.
func NewRedis(client *redis.Client, prefix string, rps float64, burst int, log *slog.Logger) *Redis {
	interval := int64(float64(time.Second/time.Microsecond) / rps)
	return &Redis{client: client, prefix: prefix, interval: interval, tolerance: interval * int64(burst), log: log}
}

func (r *Redis) Allow(ctx context.Context, key string) bool {
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	ok, err := gcra.Run(ctx, r.client, []string{r.prefix + key}, r.interval, r.tolerance).Int()
	if err != nil {
		redisErrors.Inc()
		r.log.WarnContext(ctx, "rate limiter unavailable, allowing request", "error", err)
		return true
	}
	return ok == 1
}
