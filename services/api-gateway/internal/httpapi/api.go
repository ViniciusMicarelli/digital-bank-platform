// Package httpapi is the public REST/JSON API. It authenticates customers,
// checks account ownership and translates to the internal gRPC services.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	identityv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/identity/v1"
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
	statementv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/statement/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/correlation"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/api-gateway/internal/auth"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/api-gateway/internal/ratelimit"
)

// Clients are the internal services behind the gateway.
type Clients struct {
	Identity  identityv1.IdentityServiceClient
	Accounts  accountv1.AccountServiceClient
	Txs       txv1.TransactionServiceClient
	Pix       pixv1.PixServiceClient
	Statement statementv1.StatementServiceClient
}

type Options struct {
	// Sandbox enables POST /v1/sandbox/pix-in to fund accounts in demos.
	Sandbox bool
	// Limiter limits authenticated requests per customer; AuthLimiter limits
	// the public /v1/auth routes per client IP (credential stuffing). Both
	// default to in-memory limiters; use ratelimit.Redis to share them
	// across replicas.
	Limiter, AuthLimiter ratelimit.Limiter
}

type API struct {
	c           Clients
	auth        *auth.Verifier
	log         *slog.Logger
	opts        Options
	limiter     ratelimit.Limiter
	authLimiter ratelimit.Limiter
}

func New(c Clients, v *auth.Verifier, log *slog.Logger, opts Options) *API {
	if opts.Limiter == nil {
		opts.Limiter = ratelimit.NewMemory(20, 40)
	}
	if opts.AuthLimiter == nil {
		opts.AuthLimiter = ratelimit.NewMemory(2, 10)
	}
	return &API{c: c, auth: v, log: log, opts: opts, limiter: opts.Limiter, authLimiter: opts.AuthLimiter}
}

// Handler returns the routed, observed HTTP handler.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("POST /v1/auth/register", a.perIP(a.register))
	mux.HandleFunc("POST /v1/auth/login", a.perIP(a.login))
	mux.HandleFunc("POST /v1/auth/refresh", a.perIP(a.refresh))
	mux.HandleFunc("POST /v1/auth/logout", a.perIP(a.logout))
	mux.HandleFunc("POST /v1/auth/mfa/totp", a.authenticated(a.enrollTOTP))
	mux.HandleFunc("POST /v1/auth/mfa/totp/confirm", a.authenticated(a.confirmTOTP))
	if a.opts.Sandbox {
		mux.HandleFunc("POST /v1/sandbox/pix-in", a.authenticated(a.sandboxPixIn))
	}
	mux.HandleFunc("POST /v1/accounts", a.authenticated(a.openAccount))
	mux.HandleFunc("GET /v1/accounts", a.authenticated(a.listAccounts))
	mux.HandleFunc("GET /v1/accounts/{id}", a.authenticated(a.owned(a.getAccount)))
	mux.HandleFunc("GET /v1/accounts/{id}/balance", a.authenticated(a.owned(a.getBalance)))
	mux.HandleFunc("GET /v1/accounts/{id}/limits", a.authenticated(a.owned(a.getLimits)))
	mux.HandleFunc("PUT /v1/accounts/{id}/limits", a.authenticated(a.owned(a.updateLimits)))
	mux.HandleFunc("GET /v1/accounts/{id}/statement", a.authenticated(a.owned(a.statement)))
	mux.HandleFunc("POST /v1/accounts/{id}/transfers", a.authenticated(a.owned(a.transfer)))
	mux.HandleFunc("GET /v1/accounts/{id}/pix/keys", a.authenticated(a.owned(a.listKeys)))
	mux.HandleFunc("POST /v1/accounts/{id}/pix/keys", a.authenticated(a.owned(a.registerKey)))
	mux.HandleFunc("DELETE /v1/accounts/{id}/pix/keys/{keyID}", a.authenticated(a.owned(a.deleteKey)))
	mux.HandleFunc("POST /v1/accounts/{id}/pix/transfers", a.authenticated(a.owned(a.sendPix)))
	mux.HandleFunc("GET /v1/accounts/{id}/pix/transfers", a.authenticated(a.owned(a.listPix)))
	mux.HandleFunc("GET /v1/pix/keys/{key}", a.authenticated(a.lookupKey))
	mux.HandleFunc("POST /v1/accounts/{id}/pix/qr-codes", a.authenticated(a.owned(a.createQRCode)))
	mux.HandleFunc("POST /v1/pix/qr-codes/decode", a.authenticated(a.decodeQRCode))
	mux.HandleFunc("POST /v1/accounts/{id}/pix/qr-payments", a.authenticated(a.owned(a.payQRCode)))
	mux.HandleFunc("POST /v1/accounts/{id}/pix/transfers/{transferID}/refunds", a.authenticated(a.owned(a.refundPix)))
	// MaxBytesHandler copies the request, so it wraps observe (which must see
	// the request the mux routes, to read its Pattern).
	return http.MaxBytesHandler(observe(a.log, mux), 64<<10)
}

// ---- helpers ----

type errorBody struct {
	Error struct {
		Code          string `json:"code"`
		Message       string `json:"message"`
		CorrelationID string `json:"correlation_id,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, ctx context.Context, status int, code, msg string) {
	var b errorBody
	b.Error.Code, b.Error.Message, b.Error.CorrelationID = code, msg, correlation.ID(ctx)
	writeJSON(w, status, b)
}

// writeGRPCError maps an internal gRPC error to HTTP, keeping its reason as the code.
func writeGRPCError(w http.ResponseWriter, ctx context.Context, err error) {
	st := status.Convert(err)
	httpStatus := map[codes.Code]int{
		codes.InvalidArgument:    http.StatusBadRequest,
		codes.NotFound:           http.StatusNotFound,
		codes.AlreadyExists:      http.StatusConflict,
		codes.Aborted:            http.StatusConflict,
		codes.FailedPrecondition: http.StatusUnprocessableEntity,
		codes.Unauthenticated:    http.StatusUnauthorized,
		codes.PermissionDenied:   http.StatusForbidden,
		codes.Unavailable:        http.StatusServiceUnavailable,
		codes.DeadlineExceeded:   http.StatusGatewayTimeout,
	}[st.Code()]
	if httpStatus == 0 {
		httpStatus = http.StatusInternalServerError
	}
	code := grpcserver.Reason(err)
	if code == "" {
		code = st.Code().String()
	}
	msg := st.Message()
	if httpStatus >= 500 {
		msg = "the service is temporarily unavailable, retry with the same Idempotency-Key"
	}
	writeError(w, ctx, httpStatus, code, msg)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, r.Context(), http.StatusBadRequest, "INVALID_JSON", err.Error())
		return false
	}
	return true
}

var idemKeyRE = regexp.MustCompile(`^[A-Za-z0-9_\-:.]{8,100}$`)

// idempotencyKey reads the Idempotency-Key header and scopes it to the
// customer, so two customers can never collide on the same key.
func idempotencyKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	k := r.Header.Get("Idempotency-Key")
	if !idemKeyRE.MatchString(k) {
		writeError(w, r.Context(), http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED",
			"send an Idempotency-Key header (8-100 characters: letters, digits, - _ : .)")
		return "", false
	}
	return customerID(r.Context()) + ":" + k, true
}

// Money is an amount as exposed publicly: a decimal string, never a float.
type Money struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func moneyJSON(m *commonv1.Money) Money {
	v, err := money.New(m.GetMinorUnits(), money.Currency(m.GetCurrency()))
	if err != nil {
		return Money{Amount: "0", Currency: m.GetCurrency()}
	}
	return Money{Amount: v.Decimal(), Currency: string(v.Currency())}
}

// parseBRL parses a request amount like "10.50".
func parseBRL(w http.ResponseWriter, r *http.Request, s string) (*commonv1.Money, bool) {
	m, err := money.Parse(s, money.BRL)
	if err != nil || !m.IsPositive() {
		writeError(w, r.Context(), http.StatusBadRequest, "INVALID_AMOUNT", `amount must be a positive decimal string, e.g. "10.50"`)
		return nil, false
	}
	return &commonv1.Money{MinorUnits: m.Minor(), Currency: string(m.Currency())}, true
}

type accountKey struct{}

// owned loads the {id} account and checks it belongs to the caller. Other
// customers' accounts answer 404 so their existence is not revealed.
func (a *API) owned(next func(http.ResponseWriter, *http.Request, *accountv1.Account)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, err := uuid.Parse(id); err != nil {
			writeError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "account not found")
			return
		}
		resp, err := a.c.Accounts.GetAccount(r.Context(), &accountv1.GetAccountRequest{Id: id})
		if status.Code(err) == codes.NotFound || (err == nil && resp.GetAccount().GetCustomerId() != customerID(r.Context())) {
			writeError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "account not found")
			return
		}
		if err != nil {
			writeGRPCError(w, r.Context(), err)
			return
		}
		next(w, r, resp.GetAccount())
	}
}

func ts(t interface{ AsTime() time.Time }) string { return t.AsTime().UTC().Format(time.RFC3339Nano) }
