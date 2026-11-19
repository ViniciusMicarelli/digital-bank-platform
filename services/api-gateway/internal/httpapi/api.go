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
	backofficev1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/backoffice/v1"
	cardv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/card/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	creditv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/credit/v1"
	identityv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/identity/v1"
	notificationv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/notification/v1"
	paymentv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/payment/v1"
	pixv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/pix/v1"
	savingsv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/savings/v1"
	statementv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/statement/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	userv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/user/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/correlation"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/api-gateway/internal/auth"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/api-gateway/internal/ratelimit"
)

// Clients are the internal services behind the gateway.
type Clients struct {
	Identity      identityv1.IdentityServiceClient
	Accounts      accountv1.AccountServiceClient
	Txs           txv1.TransactionServiceClient
	Pix           pixv1.PixServiceClient
	Statement     statementv1.StatementServiceClient
	Cards         cardv1.CardServiceClient
	Credit        creditv1.CreditServiceClient
	Payments      paymentv1.PaymentServiceClient
	Notifications notificationv1.NotificationServiceClient
	Users         userv1.UserServiceClient
	BackOffice    backofficev1.BackOfficeServiceClient
	Savings       savingsv1.SavingsServiceClient
}

type Options struct {
	// Sandbox enables POST /v1/sandbox/pix-in to fund accounts in demos.
	Sandbox bool
	// Limiter limits authenticated requests per customer; AuthLimiter limits
	// the public /v1/auth routes per client IP (credential stuffing). Both
	// default to in-memory limiters; use ratelimit.Redis to share them
	// across replicas.
	Limiter, AuthLimiter ratelimit.Limiter
	// AllowedOrigins may call the API from a browser on another origin
	// (CORS), e.g. the mobile app running on the web. Native apps and the
	// web app (same origin) do not need it. Empty: none.
	AllowedOrigins []string
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
	// Public by design: other banks fetch dynamic QR payloads and our keys.
	mux.HandleFunc("GET /pix/qr/v2/{txid}", a.qrPayload)
	mux.HandleFunc("GET /pix/qr/jwks.json", a.qrJWKS)
	mux.HandleFunc("POST /v1/auth/register", a.perIP(a.register))
	mux.HandleFunc("POST /v1/auth/login", a.perIP(a.login))
	mux.HandleFunc("POST /v1/auth/refresh", a.perIP(a.refresh))
	mux.HandleFunc("POST /v1/auth/logout", a.perIP(a.logout))
	mux.HandleFunc("POST /v1/auth/step-up", a.authenticated(a.stepUp))
	mux.HandleFunc("POST /v1/auth/mfa/totp", a.authenticated(a.enrollTOTP))
	mux.HandleFunc("POST /v1/auth/mfa/totp/confirm", a.authenticated(a.confirmTOTP))
	if a.opts.Sandbox {
		mux.HandleFunc("POST /v1/sandbox/pix-in", a.authenticated(a.sandboxPixIn))
		mux.HandleFunc("POST /v1/sandbox/pix/run-schedules", a.authenticated(a.sandboxRunSchedules))
		mux.HandleFunc("POST /v1/sandbox/card-network/authorizations", a.authenticated(a.networkAuthorize))
		mux.HandleFunc("POST /v1/sandbox/card-network/authorizations/{authID}/clear", a.authenticated(a.networkClear))
		mux.HandleFunc("POST /v1/sandbox/card-network/authorizations/{authID}/reverse", a.authenticated(a.networkReverse))
		mux.HandleFunc("POST /v1/sandbox/card-network/authorizations/{authID}/refund", a.authenticated(a.networkRefund))
		mux.HandleFunc("POST /v1/sandbox/accounts/{id}/credit/cycle", a.authenticated(a.owned(a.advanceCreditCycle)))
		mux.HandleFunc("POST /v1/sandbox/boletos", a.authenticated(a.issueSandboxBoleto))
		mux.HandleFunc("POST /v1/sandbox/accounts/{id}/savings/{boxID}/yield", a.authenticated(a.owned(a.ownedBox(a.simulateYield))))
		mux.HandleFunc("POST /v1/sandbox/kyc/review", a.authenticated(a.reviewKYC))
	}
	mux.HandleFunc("GET /v1/me", a.authenticated(a.me))
	mux.HandleFunc("GET /v1/backoffice/kyc/reviews", a.staff(a.kycQueue))
	mux.HandleFunc("POST /v1/backoffice/kyc/reviews/{customerID}", a.staff(a.decideKYC))
	mux.HandleFunc("GET /v1/backoffice/customers/{customerID}", a.staff(a.customerFile))
	mux.HandleFunc("POST /v1/backoffice/accounts/{id}/block", a.staff(a.blockAccount))
	mux.HandleFunc("POST /v1/backoffice/accounts/{id}/unblock", a.staff(a.unblockAccount))
	mux.HandleFunc("GET /v1/backoffice/approvals", a.staff(a.approvals))
	mux.HandleFunc("POST /v1/backoffice/approvals/{id}/approve", a.staff(a.decideApproval(true)))
	mux.HandleFunc("POST /v1/backoffice/approvals/{id}/reject", a.staff(a.decideApproval(false)))
	mux.HandleFunc("GET /v1/backoffice/disputes", a.staff(a.backofficeDisputes))
	mux.HandleFunc("POST /v1/backoffice/disputes/{id}/decide", a.staff(a.decideDispute))
	mux.HandleFunc("GET /v1/backoffice/audit", a.staff(a.auditTrail))
	mux.HandleFunc("GET /v1/backoffice/audit/verify", a.staff(a.verifyAudit))
	mux.HandleFunc("GET /v1/me/profile", a.authenticated(a.getProfile))
	mux.HandleFunc("PUT /v1/me/profile", a.authenticated(a.putProfile))
	mux.HandleFunc("POST /v1/me/kyc", a.authenticated(a.submitKYC))
	mux.HandleFunc("GET /v1/notifications", a.authenticated(a.listNotifications))
	mux.HandleFunc("POST /v1/notifications/read", a.authenticated(a.markNotificationsRead))
	mux.HandleFunc("GET /v1/notifications/preferences", a.authenticated(a.notificationPreferences))
	mux.HandleFunc("PUT /v1/notifications/preferences", a.authenticated(a.updateNotificationPreferences))
	mux.HandleFunc("GET /v1/notifications/stream", a.authenticated(a.notificationStream))
	mux.HandleFunc("POST /v1/boletos/lookup", a.authenticated(a.lookupBoleto))
	mux.HandleFunc("POST /v1/accounts/{id}/boleto-payments", a.authenticated(a.owned(a.payBoleto)))
	mux.HandleFunc("GET /v1/accounts/{id}/boleto-payments", a.authenticated(a.owned(a.listBoletoPayments)))
	mux.HandleFunc("GET /v1/accounts/{id}/boleto-payments/{paymentID}", a.authenticated(a.owned(a.ownedBoletoPayment(a.getBoletoPayment))))
	mux.HandleFunc("DELETE /v1/accounts/{id}/boleto-payments/{paymentID}", a.authenticated(a.owned(a.ownedBoletoPayment(a.cancelBoletoPayment))))
	mux.HandleFunc("POST /v1/accounts/{id}/credit", a.authenticated(a.owned(a.openCreditLine)))
	mux.HandleFunc("GET /v1/accounts/{id}/credit", a.authenticated(a.owned(a.getCreditLine)))
	mux.HandleFunc("GET /v1/accounts/{id}/credit/invoices", a.authenticated(a.owned(a.listInvoices)))
	mux.HandleFunc("GET /v1/accounts/{id}/credit/installments", a.authenticated(a.owned(a.installmentPlans)))
	mux.HandleFunc("GET /v1/accounts/{id}/credit/invoices/{invoiceID}", a.authenticated(a.owned(a.ownedInvoice(a.getInvoice))))
	mux.HandleFunc("POST /v1/accounts/{id}/credit/invoices/{invoiceID}/payments", a.authenticated(a.owned(a.ownedInvoice(a.payInvoice))))
	mux.HandleFunc("POST /v1/accounts/{id}/cards", a.authenticated(a.owned(a.issueCard)))
	mux.HandleFunc("GET /v1/accounts/{id}/cards", a.authenticated(a.owned(a.listCards)))
	mux.HandleFunc("POST /v1/accounts/{id}/cards/{cardID}/block", a.authenticated(a.owned(a.ownedCard(a.blockCard))))
	mux.HandleFunc("POST /v1/accounts/{id}/cards/{cardID}/unblock", a.authenticated(a.owned(a.ownedCard(a.unblockCard))))
	mux.HandleFunc("PUT /v1/accounts/{id}/cards/{cardID}/controls", a.authenticated(a.owned(a.ownedCard(a.cardControls))))
	mux.HandleFunc("POST /v1/accounts/{id}/cards/{cardID}/reveal", a.authenticated(a.steppedUp(a.owned(a.ownedCard(a.revealCard)))))
	mux.HandleFunc("POST /v1/accounts/{id}/cards/{cardID}/tokens", a.authenticated(a.steppedUp(a.owned(a.ownedCard(a.tokenizeCard)))))
	mux.HandleFunc("GET /v1/accounts/{id}/cards/{cardID}/tokens", a.authenticated(a.owned(a.ownedCard(a.listTokens))))
	mux.HandleFunc("GET /v1/accounts/{id}/cards/{cardID}/purchases", a.authenticated(a.owned(a.ownedCard(a.cardPurchases))))
	mux.HandleFunc("POST /v1/accounts/{id}/cards/{cardID}/purchases/{authID}/disputes", a.authenticated(a.owned(a.ownedCard(a.openDispute))))
	mux.HandleFunc("GET /v1/accounts/{id}/disputes", a.authenticated(a.owned(a.accountDisputes)))
	mux.HandleFunc("GET /v1/accounts/{id}/savings", a.authenticated(a.owned(a.listBoxes)))
	mux.HandleFunc("POST /v1/accounts/{id}/savings", a.authenticated(a.owned(a.createBox)))
	mux.HandleFunc("GET /v1/accounts/{id}/savings/{boxID}", a.authenticated(a.owned(a.ownedBox(a.getBox))))
	mux.HandleFunc("PUT /v1/accounts/{id}/savings/{boxID}", a.authenticated(a.owned(a.ownedBox(a.updateBox))))
	mux.HandleFunc("DELETE /v1/accounts/{id}/savings/{boxID}", a.authenticated(a.owned(a.ownedBox(a.closeBox))))
	mux.HandleFunc("POST /v1/accounts/{id}/savings/{boxID}/deposits", a.authenticated(a.owned(a.ownedBox(a.boxMove(true)))))
	mux.HandleFunc("POST /v1/accounts/{id}/savings/{boxID}/withdrawals", a.authenticated(a.owned(a.ownedBox(a.boxMove(false)))))
	mux.HandleFunc("POST /v1/accounts/{id}/cards/{cardID}/tokens/{tokenID}/suspend", a.authenticated(a.owned(a.ownedCard(a.updateToken("SUSPEND")))))
	mux.HandleFunc("POST /v1/accounts/{id}/cards/{cardID}/tokens/{tokenID}/resume", a.authenticated(a.steppedUp(a.owned(a.ownedCard(a.updateToken("RESUME"))))))
	mux.HandleFunc("DELETE /v1/accounts/{id}/cards/{cardID}/tokens/{tokenID}", a.authenticated(a.owned(a.ownedCard(a.updateToken("DELETE")))))
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
	mux.HandleFunc("POST /v1/accounts/{id}/pix/schedules", a.authenticated(a.owned(a.createSchedule)))
	mux.HandleFunc("GET /v1/accounts/{id}/pix/schedules", a.authenticated(a.owned(a.listSchedules)))
	mux.HandleFunc("DELETE /v1/accounts/{id}/pix/schedules/{scheduleID}", a.authenticated(a.owned(a.cancelSchedule)))
	mux.HandleFunc("POST /v1/accounts/{id}/pix/mandates", a.authenticated(a.owned(a.requestMandate)))
	mux.HandleFunc("GET /v1/accounts/{id}/pix/mandates", a.authenticated(a.owned(a.listMandates)))
	mux.HandleFunc("POST /v1/accounts/{id}/pix/mandates/{mandateID}/approve", a.authenticated(a.owned(a.decideMandate(true))))
	mux.HandleFunc("POST /v1/accounts/{id}/pix/mandates/{mandateID}/reject", a.authenticated(a.owned(a.decideMandate(false))))
	mux.HandleFunc("POST /v1/accounts/{id}/pix/mandates/{mandateID}/revoke", a.authenticated(a.owned(a.revokeMandate)))
	mux.HandleFunc("POST /v1/accounts/{id}/pix/mandates/{mandateID}/charges", a.authenticated(a.owned(a.chargeMandate)))
	mux.HandleFunc("POST /v1/pix/qr-codes/decode", a.authenticated(a.decodeQRCode))
	mux.HandleFunc("POST /v1/accounts/{id}/pix/qr-payments", a.authenticated(a.owned(a.payQRCode)))
	mux.HandleFunc("POST /v1/accounts/{id}/pix/transfers/{transferID}/refunds", a.authenticated(a.owned(a.refundPix)))
	// MaxBytesHandler copies the request, so it wraps observe (which must see
	// the request the mux routes, to read its Pattern).
	return cors(a.opts.AllowedOrigins, http.MaxBytesHandler(observe(a.log, mux), 64<<10))
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
