package httpapi

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	cardv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/card/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	identityv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/identity/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/api-gateway/internal/auth"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/api-gateway/internal/ratelimit"
)

// fakeAccounts implements only what the tests call; other methods panic.
type fakeAccounts struct {
	accountv1.AccountServiceClient
	owners map[string]string // account id -> customer id
}

func (f *fakeAccounts) GetAccount(_ context.Context, in *accountv1.GetAccountRequest, _ ...grpc.CallOption) (*accountv1.GetAccountResponse, error) {
	owner, ok := f.owners[in.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "account")
	}
	return &accountv1.GetAccountResponse{Account: &accountv1.Account{Id: in.GetId(), CustomerId: owner, HolderName: "Ana"}}, nil
}

func (f *fakeAccounts) GetBalance(context.Context, *accountv1.GetBalanceRequest, ...grpc.CallOption) (*accountv1.GetBalanceResponse, error) {
	m := &commonv1.Money{MinorUnits: 1050, Currency: "BRL"}
	return &accountv1.GetBalanceResponse{Available: m, Posted: m, Pending: &commonv1.Money{Currency: "BRL"}}, nil
}

type fakeTxs struct {
	txv1.TransactionServiceClient
	lastKey string
	err     error
	failed  string
}

func (f *fakeTxs) CreateTransfer(_ context.Context, in *txv1.CreateTransferRequest, _ ...grpc.CallOption) (*txv1.CreateTransferResponse, error) {
	f.lastKey = in.GetIdempotencyKey()
	if f.err != nil {
		return nil, f.err
	}
	st := txv1.TransactionStatus_TRANSACTION_STATUS_COMPLETED
	if f.failed != "" {
		st = txv1.TransactionStatus_TRANSACTION_STATUS_FAILED
	}
	return &txv1.CreateTransferResponse{Transaction: &txv1.Transaction{
		Id: uuid.NewString(), Status: st, FailureReason: f.failed, Amount: in.GetAmount(),
	}}, nil
}

// fakeIdentity answers Login like identity-service would.
type fakeIdentity struct {
	identityv1.IdentityServiceClient
	sign func(sub string) string
}

func (f *fakeIdentity) Login(_ context.Context, in *identityv1.LoginRequest, _ ...grpc.CallOption) (*identityv1.LoginResponse, error) {
	if in.GetPassword() != "correct horse battery" {
		return nil, grpcserver.Error(codes.Unauthenticated, "INVALID_CREDENTIALS", "invalid email or password")
	}
	return &identityv1.LoginResponse{CustomerId: "ana", Tokens: &identityv1.TokenPair{
		AccessToken: f.sign("ana"), AccessExpiresAt: timestamppb.Now(), RefreshToken: "r", RefreshExpiresAt: timestamppb.Now(),
	}}, nil
}

type harness struct {
	srv     *httptest.Server
	sign    func(sub string) string
	stepUp  func(sub string) string
	txs     *fakeTxs
	account string
}

func newHarness(t *testing.T, opts Options) harness {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	jwk := auth.JWK{Kid: "test", Kty: "OKP", Crv: "Ed25519", X: base64.RawURLEncoding.EncodeToString(pub)}
	verifier := auth.NewVerifier(func(context.Context) ([]auth.JWK, error) { return []auth.JWK{jwk}, nil })
	signFor := func(aud string) func(string) string {
		return func(sub string) string {
			tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.RegisteredClaims{
				Subject: sub, Issuer: auth.Issuer, Audience: jwt.ClaimStrings{aud},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
			})
			tok.Header["kid"] = "test"
			s, _ := tok.SignedString(priv)
			return s
		}
	}
	sign := signFor(auth.Audience)
	account := uuid.NewString()
	txs := &fakeTxs{}
	if opts.Limiter == nil {
		opts.Limiter = ratelimit.NewMemory(1000, 1000)
	}
	api := New(Clients{
		Identity: &fakeIdentity{sign: sign},
		Accounts: &fakeAccounts{owners: map[string]string{account: "ana"}}, Txs: txs,
		Cards: &fakeCards{account: account},
	}, verifier, slog.New(slog.NewTextHandler(io.Discard, nil)), opts)
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	return harness{srv: srv, sign: sign, stepUp: signFor(auth.StepUpAudience), txs: txs, account: account}
}

func (h harness) do(t *testing.T, method, path, customer, body string, headers map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, h.srv.URL+path, strings.NewReader(body))
	if customer != "" {
		req.Header.Set("Authorization", "Bearer "+h.sign(customer))
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func errCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func TestAuthAndOwnership(t *testing.T) {
	h := newHarness(t, Options{})
	path := "/v1/accounts/" + h.account + "/balance"

	if resp, body := h.do(t, "GET", path, "", "", nil); resp.StatusCode != 401 || errCode(body) != "UNAUTHENTICATED" {
		t.Fatalf("no token: %d %v", resp.StatusCode, body)
	}
	resp, body := h.do(t, "GET", path, "ana", "", map[string]string{"X-Correlation-ID": "trace-me-1"})
	if resp.StatusCode != 200 || body["available"].(map[string]any)["amount"] != "10.50" {
		t.Fatalf("owner: %d %v", resp.StatusCode, body)
	}
	if resp.Header.Get("X-Correlation-ID") != "trace-me-1" {
		t.Error("correlation id not echoed")
	}
	// Someone else's account looks exactly like a missing one.
	if resp, body := h.do(t, "GET", path, "bruno", "", nil); resp.StatusCode != 404 || errCode(body) != "NOT_FOUND" {
		t.Fatalf("other customer: %d %v", resp.StatusCode, body)
	}
	if resp, _ := h.do(t, "GET", "/v1/accounts/not-a-uuid/balance", "ana", "", nil); resp.StatusCode != 404 {
		t.Fatalf("bad id: %d", resp.StatusCode)
	}
	resp, _ = h.do(t, "GET", path, "ana", "", map[string]string{"X-Correlation-ID": "bad value <script>"})
	if resp.Header.Get("X-Correlation-ID") == "bad value <script>" || resp.Header.Get("X-Correlation-ID") == "" {
		t.Error("unsafe correlation ids must be replaced")
	}
}

func TestTransferValidationAndIdempotency(t *testing.T) {
	h := newHarness(t, Options{})
	path := "/v1/accounts/" + h.account + "/transfers"
	body := `{"to_account_id":"` + uuid.NewString() + `","amount":"12.34","description":"x"}`

	if resp, b := h.do(t, "POST", path, "ana", body, nil); resp.StatusCode != 400 || errCode(b) != "IDEMPOTENCY_KEY_REQUIRED" {
		t.Fatalf("missing key: %d %v", resp.StatusCode, b)
	}
	key := map[string]string{"Idempotency-Key": "transfer-0001"}
	for _, bad := range []string{`{"to_account_id":"x","amount":"1"}`, `{"to_account_id":"` + uuid.NewString() + `","amount":12.34}`,
		`{"to_account_id":"` + uuid.NewString() + `","amount":"-1"}`, `{"unknown":1}`} {
		if resp, _ := h.do(t, "POST", path, "ana", bad, key); resp.StatusCode != 400 {
			t.Errorf("%s: %d", bad, resp.StatusCode)
		}
	}
	resp, b := h.do(t, "POST", path, "ana", body, key)
	if resp.StatusCode != 201 || b["amount"].(map[string]any)["amount"] != "12.34" {
		t.Fatalf("transfer: %d %v", resp.StatusCode, b)
	}
	if h.txs.lastKey != "ana:transfer-0001" {
		t.Fatalf("idempotency key must be scoped to the customer: %q", h.txs.lastKey)
	}

	h.txs.failed = "INSUFFICIENT_FUNDS"
	if resp, b := h.do(t, "POST", path, "ana", body, key); resp.StatusCode != 422 || errCode(b) != "INSUFFICIENT_FUNDS" {
		t.Fatalf("failed saga: %d %v", resp.StatusCode, b)
	}
	h.txs.failed = ""
	h.txs.err = grpcserver.Error(codes.Unavailable, "RETRY_LATER", "ledger down")
	if resp, b := h.do(t, "POST", path, "ana", body, key); resp.StatusCode != 503 || errCode(b) != "RETRY_LATER" {
		t.Fatalf("unavailable: %d %v", resp.StatusCode, b)
	}
}

func TestLoginProxyAndAuthRateLimit(t *testing.T) {
	h := newHarness(t, Options{AuthLimiter: ratelimit.NewMemory(1, 5)})
	resp, body := h.do(t, "POST", "/v1/auth/login", "", `{"email":"ana@example.com","password":"correct horse battery"}`, nil)
	if resp.StatusCode != 200 || body["token_type"] != "Bearer" {
		t.Fatalf("login: %d %v", resp.StatusCode, body)
	}
	// The issued token works on protected routes.
	req, _ := http.NewRequest("GET", h.srv.URL+"/v1/accounts/"+h.account+"/balance", nil)
	req.Header.Set("Authorization", "Bearer "+body["access_token"].(string))
	if r, err := http.DefaultClient.Do(req); err != nil || r.StatusCode != 200 {
		t.Fatalf("using the token: %v %v", r.StatusCode, err)
	}
	if resp, body := h.do(t, "POST", "/v1/auth/login", "", `{"email":"ana@example.com","password":"nope"}`, nil); resp.StatusCode != 401 || errCode(body) != "INVALID_CREDENTIALS" {
		t.Fatalf("bad password: %d %v", resp.StatusCode, body)
	}
	limited := false
	for range 10 {
		if resp, _ := h.do(t, "POST", "/v1/auth/login", "", `{"email":"x@example.com","password":"nope"}`, nil); resp.StatusCode == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("login attempts must be rate limited per IP")
	}
}

func TestRouteLabel(t *testing.T) {
	h := newHarness(t, Options{})
	before := testutilCount(t, "GET /v1/accounts/{id}/balance")
	h.do(t, "GET", "/v1/accounts/"+h.account+"/balance", "ana", "", nil)
	if testutilCount(t, "GET /v1/accounts/{id}/balance") != before+1 {
		t.Fatal("metrics must be labelled with the route pattern, not the raw path")
	}
}

func testutilCount(t *testing.T, route string) float64 {
	t.Helper()
	return testutil.ToFloat64(httpRequests.WithLabelValues(route, "200"))
}

type fakeCards struct {
	cardv1.CardServiceClient
	account string
}

func (f *fakeCards) GetCard(_ context.Context, in *cardv1.GetCardRequest, _ ...grpc.CallOption) (*cardv1.GetCardResponse, error) {
	return &cardv1.GetCardResponse{Card: &cardv1.Card{Id: in.GetId(), AccountId: f.account}}, nil
}

func (f *fakeCards) RevealCardDetails(context.Context, *cardv1.RevealCardDetailsRequest, ...grpc.CallOption) (*cardv1.RevealCardDetailsResponse, error) {
	return &cardv1.RevealCardDetailsResponse{Pan: "9999990000000001", Cvv: "123"}, nil
}

func TestRevealRequiresStepUp(t *testing.T) {
	h := newHarness(t, Options{})
	path := "/v1/accounts/" + h.account + "/cards/" + uuid.NewString() + "/reveal"
	if resp, body := h.do(t, "POST", path, "ana", "", nil); resp.StatusCode != 403 || errCode(body) != "STEP_UP_REQUIRED" {
		t.Fatalf("without step-up: %d %v", resp.StatusCode, body)
	}
	if resp, _ := h.do(t, "POST", path, "ana", "", map[string]string{"X-Step-Up-Token": h.stepUp("bruno")}); resp.StatusCode != 403 {
		t.Fatalf("someone else's step-up token: %d", resp.StatusCode)
	}
	if resp, _ := h.do(t, "POST", path, "ana", "", map[string]string{"X-Step-Up-Token": h.sign("ana")}); resp.StatusCode != 403 {
		t.Fatalf("an access token used as step-up: %d", resp.StatusCode)
	}
	resp, body := h.do(t, "POST", path, "ana", "", map[string]string{"X-Step-Up-Token": h.stepUp("ana")})
	if resp.StatusCode != 200 || body["pan"] != "9999990000000001" || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("with step-up: %d %v", resp.StatusCode, body)
	}
}
