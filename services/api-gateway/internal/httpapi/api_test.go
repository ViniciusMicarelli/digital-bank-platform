package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/account/v1"
	commonv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/common/v1"
	txv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/transaction/v1"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/grpcserver"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/api-gateway/internal/auth"
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

type harness struct {
	srv     *httptest.Server
	authn   *auth.Authenticator
	txs     *fakeTxs
	account string
}

func newHarness(t *testing.T) harness {
	t.Helper()
	authn, _ := auth.New("0123456789abcdef0123456789abcdef", time.Minute)
	account := uuid.NewString()
	txs := &fakeTxs{}
	api := New(Clients{Accounts: &fakeAccounts{owners: map[string]string{account: "ana"}}, Txs: txs},
		authn, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{DevLogin: true, RateLimit: 1000})
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	return harness{srv: srv, authn: authn, txs: txs, account: account}
}

func (h harness) do(t *testing.T, method, path, customer, body string, headers map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, h.srv.URL+path, strings.NewReader(body))
	if customer != "" {
		tok, _, _ := h.authn.Issue(customer)
		req.Header.Set("Authorization", "Bearer "+tok)
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
	h := newHarness(t)
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
	h := newHarness(t)
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

func TestDevToken(t *testing.T) {
	h := newHarness(t)
	resp, body := h.do(t, "POST", "/v1/auth/token", "", `{"customer_id":"ana"}`, nil)
	if resp.StatusCode != 200 || body["access_token"] == "" {
		t.Fatalf("token: %d %v", resp.StatusCode, body)
	}
	if sub, err := h.authn.Verify(body["access_token"].(string)); err != nil || sub != "ana" {
		t.Fatalf("issued token: %q %v", sub, err)
	}
}

func TestRouteLabel(t *testing.T) {
	h := newHarness(t)
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
