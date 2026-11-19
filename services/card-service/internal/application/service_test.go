package application_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/internal/ports"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/card-service/migrations"
)

type fakeAccounts struct{ active map[uuid.UUID]bool }

func (f *fakeAccounts) Holder(_ context.Context, id uuid.UUID) (ports.Holder, error) {
	a, ok := f.active[id]
	if !ok {
		return ports.Holder{}, domain.ErrNotFound
	}
	return ports.Holder{Name: "Ana Souza", Active: a}, nil
}

// fakeTxs keeps one balance per account and honours idempotency keys.
type fakeTxs struct {
	mu        sync.Mutex
	balance   map[uuid.UUID]int64
	byKey     map[string]ports.TxResult
	held      map[uuid.UUID]int64 // tx id -> held amount
	reserves  int
	down      bool
	lostReply bool // reserve succeeds but the caller sees an error
	refunds   int
	refuse    string // refunds fail with this reason
}

func newTxs() *fakeTxs {
	return &fakeTxs{balance: map[uuid.UUID]int64{}, byKey: map[string]ports.TxResult{}, held: map[uuid.UUID]int64{}}
}

func (f *fakeTxs) Reserve(_ context.Context, key string, account uuid.UUID, amount money.Money, _ string, _ map[string]string) (ports.TxResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return ports.TxResult{}, errors.New("transaction-service unavailable")
	}
	if r, ok := f.byKey[key]; ok {
		return r, nil
	}
	f.reserves++
	r := ports.TxResult{ID: uuid.New(), Status: "RESERVED"}
	if f.balance[account] < amount.Minor() {
		r = ports.TxResult{ID: r.ID, Status: "FAILED", FailureReason: "INSUFFICIENT_FUNDS"}
	} else {
		f.balance[account] -= amount.Minor()
		f.held[r.ID] = amount.Minor()
	}
	f.byKey[key] = r
	if f.lostReply {
		f.lostReply = false
		return ports.TxResult{}, errors.New("deadline exceeded")
	}
	return r, nil
}

func (f *fakeTxs) setStatus(id uuid.UUID, st string) ports.TxResult {
	for k, r := range f.byKey {
		if r.ID == id {
			r.Status = st
			f.byKey[k] = r
			return r
		}
	}
	return ports.TxResult{}
}

func (f *fakeTxs) Complete(_ context.Context, id uuid.UUID, _ *money.Money) (ports.TxResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.setStatus(id, "COMPLETED"), nil
}

// account is the single account of these tests; Compensate gives money back to it.
var account = uuid.New()

func (f *fakeTxs) Compensate(_ context.Context, id uuid.UUID, _ string) (ports.TxResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.balance[account] += f.held[id]
	f.held[id] = 0
	return f.setStatus(id, "FAILED"), nil
}

// Refund gives money back to account, once per key; refuse makes it FAILED.
func (f *fakeTxs) Refund(_ context.Context, key string, acc uuid.UUID, amount money.Money, _ string, _ map[string]string) (ports.TxResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return ports.TxResult{}, errors.New("transaction-service unavailable")
	}
	if r, ok := f.byKey[key]; ok {
		return r, nil
	}
	r := ports.TxResult{ID: uuid.New(), Status: "COMPLETED"}
	if f.refuse != "" {
		r.Status, r.FailureReason = "FAILED", f.refuse
	} else {
		f.balance[acc] += amount.Minor()
		f.refunds++
	}
	f.byKey[key] = r
	return r, nil
}

type env struct {
	svc    *application.Service
	store  *pgstore.Store
	pool   *pgxpool.Pool
	txs    *fakeTxs
	credit *fakeTxs // the credit line, with the same contract
	accs   *fakeAccounts
	ctx    context.Context
}

func setup(t *testing.T) env {
	t.Helper()
	pool := pgtest.NewDB(t, migrations.FS, ".")
	keys, err := application.NewKeys(bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatal(err)
	}
	txs := newTxs()
	txs.balance[account] = 100_00
	accs := &fakeAccounts{active: map[uuid.UUID]bool{account: true}}
	store := pgstore.NewStore(pool)
	credit := newTxs()
	svc := application.New(store, accs, txs, credit, keys, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return env{svc: svc, store: store, pool: pool, txs: txs, credit: credit, accs: accs, ctx: context.Background()}
}

func (e env) issue(t *testing.T) (domain.Card, application.Details) {
	t.Helper()
	c, err := e.svc.Issue(e.ctx, "card-"+uuid.NewString(), account, domain.Virtual)
	if err != nil {
		t.Fatal(err)
	}
	d, err := e.svc.Reveal(e.ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return c, d
}

func purchase(d application.Details, amount string) domain.Purchase {
	return domain.Purchase{NetworkReference: uuid.NewString(), ExpiryMonth: d.ExpiryMonth, ExpiryYear: d.ExpiryYear, CVV: d.CVV,
		Amount: money.MustParse(amount, money.BRL), MerchantName: "PADARIA SAO JOSE", MCC: "5462", Country: "BR", Channel: domain.Online}
}

func TestIssueAndVault(t *testing.T) {
	e := setup(t)
	c, err := e.svc.Issue(e.ctx, "k1", account, domain.Physical)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := e.svc.Issue(e.ctx, "k1", account, domain.Physical)
	if again.ID != c.ID || c.HolderName != "ANA SOUZA" {
		t.Fatalf("issue = %+v / retry %+v", c, again)
	}
	d, _ := e.svc.Reveal(e.ctx, c.ID)
	if !domain.ValidPAN(d.PAN) || !strings.HasSuffix(d.PAN, c.Last4) || len(d.CVV) != 3 {
		t.Fatalf("details = %+v", d)
	}
	// Neither the card number nor the CVV appear in clear text anywhere in the database.
	var dump string
	_ = e.pool.QueryRow(e.ctx, `SELECT string_agg(t::text, ' ') FROM (SELECT row_to_json(c)::text t FROM cards c UNION ALL SELECT row_to_json(v)::text FROM card_vault v) x`).Scan(&dump)
	if strings.Contains(dump, d.PAN) || strings.Contains(dump, d.PAN[6:12]) {
		t.Fatal("card number stored in clear text")
	}
	e.accs.active[account] = false
	if _, err := e.svc.Issue(e.ctx, "k2", account, domain.Virtual); !errors.Is(err, domain.ErrAccountNotActive) {
		t.Fatalf("issuing for an inactive account err = %v", err)
	}
}

func TestAuthorizeClearAndReverse(t *testing.T) {
	e := setup(t)
	_, d := e.issue(t)

	p := purchase(d, "40")
	a, err := e.svc.Authorize(e.ctx, d.PAN, p)
	if err != nil || a.Status != domain.Approved || a.Code != "00" {
		t.Fatalf("authorize = %+v, %v", a, err)
	}
	// The network retries with the same reference: same answer, one reservation.
	again, _ := e.svc.Authorize(e.ctx, d.PAN, p)
	if again.ID != a.ID || e.txs.reserves != 1 {
		t.Fatalf("retry = %+v, reserves = %d", again, e.txs.reserves)
	}

	partial := money.MustParse("37.50", money.BRL)
	cleared, err := e.svc.Clear(e.ctx, a.ID, &partial)
	if err != nil || cleared.Status != domain.Cleared || cleared.Cleared != 37_50 {
		t.Fatalf("clear = %+v, %v", cleared, err)
	}
	if _, err := e.svc.Reverse(e.ctx, a.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("reversing a cleared purchase err = %v", err)
	}

	b, _ := e.svc.Authorize(e.ctx, d.PAN, purchase(d, "20"))
	reversed, err := e.svc.Reverse(e.ctx, b.ID)
	if err != nil || reversed.Status != domain.Reversed || e.txs.balance[account] != 100_00-40_00 {
		t.Fatalf("reverse = %+v, %v, balance %d", reversed, err, e.txs.balance[account])
	}
}

func TestDeclines(t *testing.T) {
	e := setup(t)
	c, d := e.issue(t)

	wrongCVV := purchase(d, "10")
	wrongCVV.CVV = "000"
	if wrongCVV.CVV == d.CVV {
		wrongCVV.CVV = "001"
	}
	if a, _ := e.svc.Authorize(e.ctx, d.PAN, wrongCVV); a.Status != domain.Declined || a.Code != domain.CodeWrongCVV {
		t.Fatalf("wrong CVV = %+v", a)
	}
	if a, _ := e.svc.Authorize(e.ctx, d.PAN, purchase(d, "100.01")); a.Code != domain.CodeInsufficientFunds {
		t.Fatalf("insufficient funds = %+v", a)
	}
	if a, _ := e.svc.Authorize(e.ctx, "4111111111111111", purchase(d, "1")); a.Code != domain.CodeInvalidCard {
		t.Fatalf("unknown card = %+v", a)
	}
	if e.txs.reserves != 1 {
		t.Fatalf("card-level declines must not reserve money, reserves = %d", e.txs.reserves)
	}

	// Account blocked -> its cards decline with 62; unblocked -> back to normal.
	blocked, _ := outbox.NewEvent(e.ctx, "account-service", "account.blocked", map[string]string{"account_id": account.String()})
	_ = e.store.HandleAccountEvent(e.ctx, blocked)
	if a, _ := e.svc.Authorize(e.ctx, d.PAN, purchase(d, "1")); a.Code != domain.CodeRestrictedCard {
		t.Fatalf("blocked account = %+v", a)
	}
	unblocked, _ := outbox.NewEvent(e.ctx, "account-service", "account.unblocked", map[string]string{"account_id": account.String()})
	_ = e.store.HandleAccountEvent(e.ctx, unblocked)
	if got, _ := e.svc.Card(e.ctx, c.ID); got.Status != domain.Active {
		t.Fatalf("card after unblock = %s", got.Status)
	}
	// A card the customer blocked stays blocked when the account is unblocked.
	_, _ = e.svc.Block(e.ctx, c.ID, "lost")
	_ = e.store.HandleAccountEvent(e.ctx, blocked)
	unblocked2, _ := outbox.NewEvent(e.ctx, "account-service", "account.unblocked", map[string]string{"account_id": account.String()})
	_ = e.store.HandleAccountEvent(e.ctx, unblocked2)
	if got, _ := e.svc.Card(e.ctx, c.ID); got.Status != domain.Blocked || got.StatusReason != "lost" {
		t.Fatalf("customer block overridden: %+v", got)
	}
}

func TestOrphanReservationIsReleased(t *testing.T) {
	e := setup(t)
	_, d := e.issue(t)
	e.txs.lostReply = true // the hold is placed, but we never hear back

	a, err := e.svc.Authorize(e.ctx, d.PAN, purchase(d, "30"))
	if err != nil || a.Code != domain.CodeIssuerUnavailable {
		t.Fatalf("authorize during outage = %+v, %v", a, err)
	}
	if e.txs.balance[account] != 70_00 {
		t.Fatalf("expected the lost reservation to hold money, balance %d", e.txs.balance[account])
	}
	e.svc.SetClock(func() time.Time { return time.Now().Add(time.Minute) })
	if n := e.svc.RecoverOnce(e.ctx, 30*time.Second); n != 1 {
		t.Fatalf("recovered %d", n)
	}
	got, _ := e.store.Authorization(e.ctx, a.ID)
	if got.Status != domain.Declined || got.Code != domain.CodeIssuerUnavailable || e.txs.balance[account] != 100_00 {
		t.Fatalf("after cleanup: %+v, balance %d", got, e.txs.balance[account])
	}
}

func TestTokenPurchases(t *testing.T) {
	e := setup(t)
	c, _ := e.issue(t)
	tok, tokenPAN, err := e.svc.Tokenize(e.ctx, c.ID, domain.ApplePay, "iPhone da Ana")
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Purchase{NetworkReference: uuid.NewString(), ExpiryMonth: tok.ExpiryMonth, ExpiryYear: tok.ExpiryYear,
		Amount: money.MustParse("12", money.BRL), MerchantName: "CAFE", MCC: "5814", Country: "BR", Channel: domain.Contactless}
	a, err := e.svc.Authorize(e.ctx, tokenPAN, p)
	if err != nil || a.Status != domain.Approved || a.TokenID != tok.ID || a.CardID != c.ID {
		t.Fatalf("token purchase = %+v, %v", a, err)
	}
	if _, err := e.svc.UpdateToken(e.ctx, c.ID, tok.ID, "SUSPEND"); err != nil {
		t.Fatal(err)
	}
	p.NetworkReference = uuid.NewString()
	if a, _ := e.svc.Authorize(e.ctx, tokenPAN, p); a.Code != domain.CodeRestrictedCard {
		t.Fatalf("suspended token = %+v", a)
	}
	// The card itself keeps working.
	_, d := e.issue(t)
	if a, _ := e.svc.Authorize(e.ctx, d.PAN, purchase(d, "5")); a.Status != domain.Approved {
		t.Fatalf("card after token suspension = %+v", a)
	}
	if _, err := e.svc.UpdateToken(e.ctx, uuid.New(), tok.ID, "RESUME"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("a token can only be managed through its card")
	}
	var dump string
	_ = e.pool.QueryRow(e.ctx, `SELECT row_to_json(t)::text FROM card_tokens t WHERE id = $1`, tok.ID).Scan(&dump)
	if strings.Contains(dump, tokenPAN) {
		t.Fatal("token number stored in clear text")
	}
}

func TestCreditFunction(t *testing.T) {
	e := setup(t)
	_, d := e.issue(t)
	e.txs.balance[account] = 0 // no money in checking: credit must not touch it
	p := purchase(d, "300")
	p.Function = domain.Credit
	e.credit.balance[account] = 500_00 // the available limit
	a, err := e.svc.Authorize(e.ctx, d.PAN, p)
	if err != nil || a.Status != domain.Approved || a.Purchase.Function != domain.Credit {
		t.Fatalf("credit purchase = %+v %v", a, err)
	}
	if e.credit.balance[account] != 200_00 || e.txs.reserves != 0 {
		t.Fatalf("limit left %d, debit reserves %d", e.credit.balance[account], e.txs.reserves)
	}
	if a, err = e.svc.Clear(e.ctx, a.ID, nil); err != nil || a.Status != domain.Cleared {
		t.Fatalf("clear = %+v %v", a, err)
	}
	if st := e.credit.byKey[a.TransactionKey()].Status; st != "COMPLETED" {
		t.Fatalf("credit hold after clearing = %s", st)
	}
	p.NetworkReference = uuid.NewString()
	if a, _ := e.svc.Authorize(e.ctx, d.PAN, p); a.Code != domain.CodeInsufficientFunds {
		t.Fatalf("over the credit limit = %+v", a)
	}
}

func reasonOf(err error) string {
	if r, ok := domain.AsRefused(err); ok {
		return r.Reason
	}
	return ""
}

func TestMerchantRefunds(t *testing.T) {
	e := setup(t)
	_, d := e.issue(t)
	a, _ := e.svc.Authorize(e.ctx, d.PAN, purchase(d, "60"))
	if _, err := e.svc.Refund(e.ctx, a.ID, "rf-1", 10_00); reasonOf(err) != "NOT_CLEARED" {
		t.Fatalf("refund before clearing: %v", err)
	}
	_, _ = e.svc.Clear(e.ctx, a.ID, nil) // balance 40
	r, err := e.svc.Refund(e.ctx, a.ID, "rf-1", 25_00)
	if err != nil || r.Status != domain.RefundCompleted || e.txs.balance[account] != 65_00 {
		t.Fatalf("refund = %+v %v (balance %d)", r, err, e.txs.balance[account])
	}
	if again, _ := e.svc.Refund(e.ctx, a.ID, "rf-1", 25_00); again.ID != r.ID || e.txs.refunds != 1 {
		t.Fatal("a network retry refunds once")
	}
	if _, err := e.svc.Refund(e.ctx, a.ID, "rf-2", 35_01); reasonOf(err) != "REFUND_TOO_LARGE" {
		t.Fatalf("more than cleared: %v", err)
	}

	// An outage: recorded as PENDING, the worker finishes it.
	e.txs.down = true
	r2, err := e.svc.Refund(e.ctx, a.ID, "rf-2", 5_00)
	if err != nil || r2.Status != domain.RefundPending {
		t.Fatalf("during the outage = %+v %v", r2, err)
	}
	e.txs.down = false
	e.svc.SetClock(func() time.Time { return time.Now().Add(time.Minute) })
	e.svc.RecoverOnce(e.ctx, 30*time.Second)
	if got, _ := e.store.Authorization(e.ctx, a.ID); got.Refunded != 30_00 || e.txs.balance[account] != 70_00 {
		t.Fatalf("after recovery: refunded %d, balance %d", got.Refunded, e.txs.balance[account])
	}

	// A refund the ledger refuses gives the amount back to what can be refunded.
	e.txs.refuse = "DESTINATION_NOT_ACTIVE"
	if r3, _ := e.svc.Refund(e.ctx, a.ID, "rf-3", 10_00); r3.Status != domain.RefundFailed || r3.FailureReason != "DESTINATION_NOT_ACTIVE" {
		t.Fatalf("refused refund = %+v", r3)
	}
	if got, _ := e.store.Authorization(e.ctx, a.ID); got.Refunded != 30_00 {
		t.Fatalf("refunded after a failure = %d", got.Refunded)
	}
}

func TestDisputes(t *testing.T) {
	e := setup(t)
	c, d := e.issue(t)
	a, _ := e.svc.Authorize(e.ctx, d.PAN, purchase(d, "80"))
	if _, err := e.svc.OpenDispute(e.ctx, uuid.Nil, a.ID, domain.DisputeNotReceived, 0, ""); reasonOf(err) != "NOT_CLEARED" {
		t.Fatalf("dispute before clearing: %v", err)
	}
	_, _ = e.svc.Clear(e.ctx, a.ID, nil) // balance 20

	// Not recognized: fraud, the card is blocked at once.
	if _, err := e.svc.OpenDispute(e.ctx, uuid.New(), a.ID, domain.DisputeNotRecognized, 0, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("a purchase of another card: %v", err)
	}
	dp, err := e.svc.OpenDispute(e.ctx, c.ID, a.ID, domain.DisputeNotRecognized, 0, "não fui eu")
	if err != nil || dp.Status != domain.DisputeOpen || dp.Amount != 80_00 || dp.MerchantName != "PADARIA SAO JOSE" {
		t.Fatalf("open = %+v %v", dp, err)
	}
	if card, _ := e.store.Card(e.ctx, c.ID); card.Status != domain.Blocked || card.StatusReason != domain.ReasonFraudSuspected {
		t.Fatalf("card after a fraud dispute = %+v", card)
	}
	if _, err := e.svc.Unblock(e.ctx, c.ID); reasonOf(err) != "CARD_COMPROMISED" {
		t.Fatalf("unblocking a card blocked for fraud: %v", err)
	}
	if _, err := e.svc.OpenDispute(e.ctx, uuid.Nil, a.ID, domain.DisputeDuplicate, 0, ""); reasonOf(err) != "DISPUTE_ALREADY_OPEN" {
		t.Fatalf("second dispute: %v", err)
	}
	// The merchant cannot refund what is under dispute.
	if _, err := e.svc.Refund(e.ctx, a.ID, "rf-1", 1_00); reasonOf(err) != "REFUND_TOO_LARGE" {
		t.Fatalf("refund under dispute: %v", err)
	}
	if list, _ := e.svc.Disputes(e.ctx, uuid.Nil, domain.DisputeOpen, 10); len(list) != 1 {
		t.Fatalf("open disputes = %+v", list)
	}
	purchases, _ := e.svc.Purchases(e.ctx, c.ID, 10)
	if len(purchases) != 1 || purchases[0].Dispute == nil || purchases[0].Dispute.ID != dp.ID {
		t.Fatalf("purchases = %+v", purchases)
	}

	// Won: the customer gets the money back, once.
	won, err := e.svc.DecideDispute(e.ctx, dp.ID, "analyst-1", true, "")
	if err != nil || won.Status != domain.DisputeWon || won.DecidedBy != "analyst-1" || e.txs.balance[account] != 100_00 {
		t.Fatalf("won = %+v %v (balance %d)", won, err, e.txs.balance[account])
	}
	if _, err := e.svc.DecideDispute(e.ctx, dp.ID, "analyst-2", false, "x"); reasonOf(err) != "DISPUTE_NOT_OPEN" {
		t.Fatalf("decided twice: %v", err)
	}
	if _, err := e.svc.OpenDispute(e.ctx, uuid.Nil, a.ID, domain.DisputeDuplicate, 0, ""); reasonOf(err) != "NOTHING_TO_DISPUTE" {
		t.Fatalf("dispute a refunded purchase: %v", err)
	}

	// Lost: needs a reason, nothing moves. (The first card is blocked: a new one.)
	_, d2 := e.issue(t)
	b, _ := e.svc.Authorize(e.ctx, d2.PAN, purchase(d2, "10"))
	_, _ = e.svc.Clear(e.ctx, b.ID, nil)
	lost, _ := e.svc.OpenDispute(e.ctx, uuid.Nil, b.ID, domain.DisputeNotReceived, 4_00, "")
	if _, err := e.svc.DecideDispute(e.ctx, lost.ID, "analyst-1", false, ""); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatal("a lost dispute needs a reason")
	}
	if l, err := e.svc.DecideDispute(e.ctx, lost.ID, "analyst-1", false, "Comprovante de entrega"); err != nil || l.Status != domain.DisputeLost || e.txs.balance[account] != 90_00 {
		t.Fatalf("lost = %+v %v (balance %d)", l, err, e.txs.balance[account])
	}
}

func TestCreditDisputeRefundsTheInvoice(t *testing.T) {
	e := setup(t)
	_, d := e.issue(t)
	p := purchase(d, "50")
	p.Function = domain.Credit
	e.credit.balance[account] = 500_00
	a, _ := e.svc.Authorize(e.ctx, d.PAN, p)
	_, _ = e.svc.Clear(e.ctx, a.ID, nil)
	dp, _ := e.svc.OpenDispute(e.ctx, uuid.Nil, a.ID, domain.DisputeWrongAmount, 20_00, "")
	if _, err := e.svc.DecideDispute(e.ctx, dp.ID, "analyst-1", true, ""); err != nil {
		t.Fatal(err)
	}
	if e.credit.refunds != 1 || e.txs.refunds != 0 {
		t.Fatalf("credit refunds %d, debit refunds %d", e.credit.refunds, e.txs.refunds)
	}
}
