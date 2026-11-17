package httpapi

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/trace"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/correlation"
)

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total", Help: "HTTP requests by route and status.",
	}, []string{"route", "status"})
	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_request_duration_seconds", Help: "HTTP request latency by route.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5},
	}, []string{"route"})
)

var correlationRE = regexp.MustCompile(`^[A-Za-z0-9_\-.]{1,64}$`)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

// Unwrap lets http.ResponseController reach the connection (flushing and
// deadlines for streams).
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// observe adds a correlation id, recovers panics, logs and records metrics.
func observe(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// Accept a caller's id only if it is short and safe to log.
		id := r.Header.Get("X-Correlation-ID")
		if !correlationRE.MatchString(id) {
			id = uuid.NewString()
		}
		w.Header().Set("X-Correlation-ID", id)
		ctx := correlation.WithID(r.Context(), id)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		// The mux sets Pattern on the request it routes, so keep that copy.
		req := r.WithContext(ctx)

		defer func() {
			if p := recover(); p != nil {
				log.ErrorContext(ctx, "panic", "panic", p, "stack", string(debug.Stack()))
				writeError(rec, ctx, http.StatusInternalServerError, "INTERNAL", "internal error")
			}
			route := req.Pattern
			if route == "" {
				route = "unmatched"
			}
			trace.SpanFromContext(ctx).SetName(route)
			httpRequests.WithLabelValues(route, strconv.Itoa(rec.status)).Inc()
			httpDuration.WithLabelValues(route).Observe(time.Since(start).Seconds())
			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
			}
			log.Log(ctx, level, "http request", "method", r.Method, "route", route, "status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(), "correlation_id", id)
		}()
		next.ServeHTTP(rec, req)
	})
}

type customerKey struct{}

func customerID(ctx context.Context) string {
	id, _ := ctx.Value(customerKey{}).(string)
	return id
}

// authenticated requires a valid bearer token and applies the per-customer rate limit.
func (a *API) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			writeError(w, r.Context(), http.StatusUnauthorized, "UNAUTHENTICATED", "missing bearer token")
			return
		}
		customer, err := a.auth.Verify(r.Context(), token)
		if err != nil {
			writeError(w, r.Context(), http.StatusUnauthorized, "UNAUTHENTICATED", err.Error())
			return
		}
		if !a.limiter.Allow(r.Context(), "cust:"+customer) {
			w.Header().Set("Retry-After", "1")
			writeError(w, r.Context(), http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), customerKey{}, customer)))
	}
}

// perIP rate-limits unauthenticated routes by client address.
func (a *API) perIP(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !a.authLimiter.Allow(r.Context(), "ip:"+ip) {
			w.Header().Set("Retry-After", "1")
			writeError(w, r.Context(), http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
			return
		}
		next(w, r)
	}
}

// steppedUp requires a step-up token (X-Step-Up-Token) for the same
// customer: a fresh password (+ TOTP) check, valid for 5 minutes.
func (a *API) steppedUp(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sub, err := a.auth.VerifyStepUp(r.Context(), r.Header.Get("X-Step-Up-Token"))
		if err != nil || sub != customerID(r.Context()) {
			writeError(w, r.Context(), http.StatusForbidden, "STEP_UP_REQUIRED",
				"confirm your password at POST /v1/auth/step-up and send the token in X-Step-Up-Token")
			return
		}
		next(w, r)
	}
}
