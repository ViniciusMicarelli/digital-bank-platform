package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/adapters/simulator"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/ports"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/migrations"
)

type fakeAccounts struct{ holders map[uuid.UUID]ports.Holder }

func (f *fakeAccounts) Holder(_ context.Context, id uuid.UUID) (ports.Holder, error) {
	h, ok := f.holders[id]
	if !ok {
		return ports.Holder{}, domain.ErrNotFound
	}
	return h, nil
}

// fakeTxs simulates transaction-service with in-memory balances.
type fakeTxs struct {
	mu           sync.Mutex
	balance      map[uuid.UUID]int64
	txs          map[string]ports.TxResult // by idempotency key
	byID         map[uuid.UUID]string
	completeErr  error
	lastMetadata map[string]string
}

func newTxs() *fakeTxs {
	return &fakeTxs{balance: map[uuid.UUID]int64{}, txs: map[string]ports.TxResult{}, byID: map[uuid.UUID]string{}}
}

func (f *fakeTxs) record(key string, r ports.TxResult) ports.TxResult {
	f.txs[key], f.byID[r.ID] = r, key
	return r
}

func (f *fakeTxs) Transfer(_ context.Context, key string, from, to uuid.UUID, amount money.Money, _ string, md map[string]string) (ports.TxResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastMetadata = md
	if r, ok := f.txs[key]; ok {
		return r, nil
	}
	if f.balance[from] < amount.Minor() {
		return f.record(key, ports.TxResult{ID: uuid.New(), Status: "FAILED", FailureReason: "INSUFFICIENT_FUNDS"}), nil
	}
	f.balance[from] -= amount.Minor()
	f.balance[to] += amount.Minor()
	return f.record(key, ports.TxResult{ID: uuid.New(), Status: "COMPLETED"}), nil
}

func (f *fakeTxs) ReserveOutgoing(_ context.Context, key string, from uuid.UUID, amount money.Money, _ string, _ map[string]string) (ports.TxResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.txs[key]; ok {
		return r, nil
	}
	if f.balance[from] < amount.Minor() {
		return f.record(key, ports.TxResult{ID: uuid.New(), Status: "FAILED", FailureReason: "INSUFFICIENT_FUNDS"}), nil
	}
	f.balance[from] -= amount.Minor()
	return f.record(key, ports.TxResult{ID: uuid.New(), Status: "RESERVED"}), nil
}

func (f *fakeTxs) setStatus(id uuid.UUID, st string) ports.TxResult {
	key := f.byID[id]
	r := f.txs[key]
	r.Status = st
	f.txs[key] = r
	return r
}

func (f *fakeTxs) Complete(_ context.Context, id uuid.UUID) (ports.TxResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.completeErr != nil {
		return ports.TxResult{}, f.completeErr
	}
	return f.setStatus(id, "COMPLETED"), nil
}

func (f *fakeTxs) Compensate(_ context.Context, id uuid.UUID, reason string) (ports.TxResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.setStatus(id, "FAILED")
	r.FailureReason = reason
	return r, nil
}

func (f *fakeTxs) CreditIncoming(_ context.Context, key string, to uuid.UUID, amount money.Money, _ string, _ map[string]string) (ports.TxResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.txs[key]; ok {
		return r, nil
	}
	f.balance[to] += amount.Minor()
	return f.record(key, ports.TxResult{ID: uuid.New(), Status: "COMPLETED"}), nil
}

type env struct {
	svc   *application.Service
	store *pgstore.Store
	pool  *pgxpool.Pool
	txs   *fakeTxs
	ana   uuid.UUID
	bruno uuid.UUID
	ctx   context.Context
}

func setup(t *testing.T) env {
	t.Helper()
	pool := pgtest.NewDB(t, migrations.FS, ".")
	ana, bruno := uuid.New(), uuid.New()
	accounts := &fakeAccounts{holders: map[uuid.UUID]ports.Holder{
		ana:   {Name: "Ana Silva", Document: "52998224725", Active: true},
		bruno: {Name: "Bruno Lima", Document: "11144477735", Active: true},
	}}
	txs := newTxs()
	txs.balance[ana] = 100_00
	store := pgstore.NewStore(pool)
	svc := application.New(store, accounts, txs, simulator.NewDirectory(), simulator.NewSPI(),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	return env{svc: svc, store: store, pool: pool, txs: txs, ana: ana, bruno: bruno, ctx: context.Background()}
}

func brl(s string) money.Money { return money.MustParse(s, money.BRL) }

func TestRegisterKeys(t *testing.T) {
	e := setup(t)
	if _, err := e.svc.RegisterKey(e.ctx, e.ana, domain.KeyCPF, "111.444.777-35"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("someone else's CPF err = %v", err)
	}
	if _, err := e.svc.RegisterKey(e.ctx, e.ana, domain.KeyCPF, "529.982.247-25"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RegisterKey(e.ctx, e.ana, domain.KeyEmail, "ana@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RegisterKey(e.ctx, e.bruno, domain.KeyEmail, "ANA@example.com"); !errors.Is(err, domain.ErrKeyTaken) {
		t.Fatalf("taken key err = %v", err)
	}
	for range 3 {
		if _, err := e.svc.RegisterKey(e.ctx, e.ana, domain.KeyEVP, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.RegisterKey(e.ctx, e.ana, domain.KeyEVP, ""); !errors.Is(err, domain.ErrTooManyKeys) {
		t.Fatalf("sixth key err = %v", err)
	}
	keys, _ := e.svc.Keys(e.ctx, e.ana)
	if len(keys) != 5 {
		t.Fatalf("keys = %d", len(keys))
	}
	if err := e.svc.DeleteKey(e.ctx, e.bruno, keys[0].ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleting someone else's key err = %v", err)
	}
	if err := e.svc.DeleteKey(e.ctx, e.ana, keys[0].ID); err != nil {
		t.Fatal(err)
	}
}

func TestLookup(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.RegisterKey(e.ctx, e.bruno, domain.KeyEmail, "bruno@example.com")
	_, _, owner, account, err := e.svc.Lookup(e.ctx, "bruno@example.com")
	if err != nil || owner.Name != "Bruno Lima" || owner.DocumentMasked != "***.444.777-**" || account != e.bruno {
		t.Fatalf("local lookup = %+v %v %v", owner, account, err)
	}
	_, _, owner, account, err = e.svc.Lookup(e.ctx, "maria@outrobanco.com.br")
	if err != nil || owner.ISPB != "12345678" || account != uuid.Nil {
		t.Fatalf("external lookup = %+v %v", owner, err)
	}
	if _, _, _, _, err := e.svc.Lookup(e.ctx, "nobody@nowhere.com"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown key err = %v", err)
	}
}

func TestSendInternal(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.RegisterKey(e.ctx, e.bruno, domain.KeyPhone, "+5521998765432")
	tr, err := e.svc.Send(e.ctx, "send-1", e.ana, "+55 21 99876-5432", brl("25"), "almoço")
	if err != nil || tr.Status != domain.StatusCompleted || !tr.Internal() {
		t.Fatalf("send = %+v, %v", tr, err)
	}
	if e.txs.balance[e.bruno] != 25_00 || e.txs.lastMetadata["end_to_end_id"] != tr.EndToEndID {
		t.Fatalf("bruno = %d metadata = %v", e.txs.balance[e.bruno], e.txs.lastMetadata)
	}
	again, err := e.svc.Send(e.ctx, "send-1", e.ana, "+5521998765432", brl("25"), "almoço")
	if err != nil || again.ID != tr.ID || e.txs.balance[e.bruno] != 25_00 {
		t.Fatalf("idempotent retry = %+v, %v", again, err)
	}
	poor, err := e.svc.Send(e.ctx, "send-2", e.ana, "+5521998765432", brl("1000"), "")
	if err != nil || poor.Status != domain.StatusRejected || poor.FailureReason != "INSUFFICIENT_FUNDS" {
		t.Fatalf("insufficient = %+v, %v", poor, err)
	}
}

func TestSendExternal(t *testing.T) {
	e := setup(t)
	tr, err := e.svc.Send(e.ctx, "ext-1", e.ana, "maria@outrobanco.com.br", brl("40"), "")
	if err != nil || tr.Status != domain.StatusCompleted || tr.Internal() || tr.Counterparty.Name != "Maria Oliveira" {
		t.Fatalf("external send = %+v, %v", tr, err)
	}
	refused, err := e.svc.Send(e.ctx, "ext-2", e.ana, "recusa@outrobanco.com.br", brl("10"), "")
	if err != nil || refused.Status != domain.StatusFailed || refused.FailureReason != "RECEIVER_BANK_REJECTED" {
		t.Fatalf("refused = %+v, %v", refused, err)
	}
}

func TestSendRecoversAfterOutage(t *testing.T) {
	e := setup(t)
	e.txs.completeErr = errors.New("transaction-service unavailable")
	tr, err := e.svc.Send(e.ctx, "ext-1", e.ana, "maria@outrobanco.com.br", brl("40"), "")
	if err == nil || tr.Status != domain.StatusSentToSPI {
		t.Fatalf("during outage: %+v, %v", tr, err)
	}
	e.txs.completeErr = nil
	if n := e.svc.RecoverOnce(e.ctx, 0); n != 1 {
		t.Fatalf("recovered %d", n)
	}
	got, _ := e.svc.Transfer(e.ctx, tr.ID)
	if got.Status != domain.StatusCompleted {
		t.Fatalf("after recovery: %+v", got)
	}
}

func TestReceive(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.RegisterKey(e.ctx, e.bruno, domain.KeyEmail, "bruno@example.com")
	e2e := domain.NewEndToEndID("12345678", time.Now())
	payer := domain.Party{Name: "Maria Oliveira", ISPB: "12345678", BankName: "Banco Exemplo"}
	tr, err := e.svc.Receive(e.ctx, e2e, "bruno@example.com", brl("15"), payer, "presente")
	if err != nil || tr.Status != domain.StatusCompleted || tr.Direction != domain.Incoming {
		t.Fatalf("receive = %+v, %v", tr, err)
	}
	again, err := e.svc.Receive(e.ctx, e2e, "bruno@example.com", brl("15"), payer, "presente")
	if err != nil || again.ID != tr.ID || e.txs.balance[e.bruno] != 15_00 {
		t.Fatalf("duplicate delivery credited twice: %d", e.txs.balance[e.bruno])
	}
	if _, err := e.svc.Receive(e.ctx, domain.NewEndToEndID("12345678", time.Now()), "ghost@example.com", brl("1"), payer, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown key err = %v", err)
	}
	if _, err := e.svc.Receive(e.ctx, "bad", "bruno@example.com", brl("1"), payer, ""); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("bad e2e err = %v", err)
	}
}

func TestAccountClosedRemovesKeys(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.RegisterKey(e.ctx, e.ana, domain.KeyEmail, "ana@example.com")
	ev, _ := outbox.NewEvent(e.ctx, "account-service", "account.closed", map[string]string{"account_id": e.ana.String()})
	for range 2 { // redelivery is harmless
		if err := e.store.HandleAccountEvent(e.ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	keys, _ := e.svc.Keys(e.ctx, e.ana)
	if len(keys) != 0 {
		t.Fatalf("keys left: %d", len(keys))
	}
	other, _ := outbox.NewEvent(e.ctx, "account-service", "account.blocked", json.RawMessage(`{}`))
	if err := e.store.HandleAccountEvent(e.ctx, other); err != nil {
		t.Fatal(err)
	}
}

func TestQRCodeOneTimeCharge(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.RegisterKey(e.ctx, e.bruno, domain.KeyEmail, "bruno@example.com")
	amount := brl("42.50")
	q, payload, err := e.svc.CreateQRCode(e.ctx, e.bruno, "bruno@example.com", &amount, "Pedido 7", true, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	d, err := e.svc.DecodeQRCode(e.ctx, payload)
	if err != nil || d.Receiver.Name != "Bruno Lima" || d.Charge == nil || d.Charge.TxID != q.TxID || *d.Code.Amount != amount {
		t.Fatalf("decode = %+v, %v", d, err)
	}

	paid, err := e.svc.PayQRCode(e.ctx, "pay-1", e.ana, payload, nil, "")
	if err != nil || paid.Status != domain.StatusCompleted || paid.QRTxID != q.TxID || e.txs.balance[e.bruno] != 42_50 {
		t.Fatalf("pay = %+v, %v (bruno %d)", paid, err, e.txs.balance[e.bruno])
	}
	// A retry with the same key returns the same payment.
	if again, err := e.svc.PayQRCode(e.ctx, "pay-1", e.ana, payload, nil, ""); err != nil || again.ID != paid.ID {
		t.Fatalf("retry = %+v, %v", again, err)
	}
	// Anyone else paying the same one-time code is refused before money moves.
	if _, err := e.svc.PayQRCode(e.ctx, "pay-2", e.ana, payload, nil, ""); !errors.Is(err, domain.ErrQRUnavailable) {
		t.Fatalf("second payment err = %v", err)
	}
	if e.txs.balance[e.bruno] != 42_50 {
		t.Fatal("a one-time code was paid twice")
	}

	other := brl("1.00")
	if _, err := e.svc.PayQRCode(e.ctx, "pay-3", e.ana, payload, &other, ""); err == nil {
		t.Fatal("paying a different amount than the code's must fail")
	}
}

func TestQRCodeClaimReleasedWhenPaymentFails(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.RegisterKey(e.ctx, e.bruno, domain.KeyEmail, "bruno@example.com")
	amount := brl("500")
	_, payload, _ := e.svc.CreateQRCode(e.ctx, e.bruno, "bruno@example.com", &amount, "", true, 0)

	poor, err := e.svc.PayQRCode(e.ctx, "p1", e.ana, payload, nil, "") // Ana has R$ 100
	if err != nil || poor.Status != domain.StatusRejected {
		t.Fatalf("payment without funds = %+v, %v", poor, err)
	}
	e.txs.balance[e.ana] = 1_000_00
	if ok, err := e.svc.PayQRCode(e.ctx, "p2", e.ana, payload, nil, ""); err != nil || ok.Status != domain.StatusCompleted {
		t.Fatalf("the failed payment must release the code: %+v, %v", ok, err)
	}
}

func TestQRCodeOpenAmountAndExpiry(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.RegisterKey(e.ctx, e.bruno, domain.KeyEmail, "bruno@example.com")
	_, payload, err := e.svc.CreateQRCode(e.ctx, e.bruno, "bruno@example.com", nil, "Doações", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.PayQRCode(e.ctx, "d1", e.ana, payload, nil, ""); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("open amount without amount err = %v", err)
	}
	for i, v := range []string{"5", "7"} { // reusable: several payers
		m := brl(v)
		if tr, err := e.svc.PayQRCode(e.ctx, fmt.Sprint("d-", i), e.ana, payload, &m, ""); err != nil || tr.Status != domain.StatusCompleted {
			t.Fatalf("donation %d = %+v, %v", i, tr, err)
		}
	}

	if _, _, err := e.svc.CreateQRCode(e.ctx, e.ana, "bruno@example.com", nil, "", false, 0); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("someone else's key err = %v", err)
	}
	_, expiring, _ := e.svc.CreateQRCode(e.ctx, e.bruno, "bruno@example.com", nil, "", false, time.Minute)
	e.svc.SetClock(func() time.Time { return time.Now().Add(2 * time.Minute) })
	if _, err := e.svc.DecodeQRCode(e.ctx, expiring); !errors.Is(err, domain.ErrQRUnavailable) {
		t.Fatalf("expired code err = %v", err)
	}
}

func receive(t *testing.T, e env, key string, amount string) domain.Transfer {
	t.Helper()
	tr, err := e.svc.Receive(e.ctx, domain.NewEndToEndID("12345678", time.Now()), key, brl(amount),
		domain.Party{Name: "Maria Oliveira", ISPB: "12345678", BankName: "Banco Exemplo"}, "")
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestRefunds(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.RegisterKey(e.ctx, e.bruno, domain.KeyEmail, "bruno@example.com")
	in := receive(t, e, "bruno@example.com", "100")

	r1, err := e.svc.Refund(e.ctx, "r1", e.bruno, in.ID, brl("30"), "valor errado")
	if err != nil || r1.Status != domain.StatusCompleted || r1.RefundOf != in.ID || r1.Counterparty.Name != "Maria Oliveira" {
		t.Fatalf("partial refund = %+v, %v", r1, err)
	}
	if again, err := e.svc.Refund(e.ctx, "r1", e.bruno, in.ID, brl("30"), "valor errado"); err != nil || again.ID != r1.ID {
		t.Fatalf("retry = %+v, %v", again, err)
	}
	if _, err := e.svc.Refund(e.ctx, "r2", e.bruno, in.ID, brl("70.01"), ""); !errors.Is(err, domain.ErrRefundTooLarge) {
		t.Fatalf("over-refund err = %v", err)
	}
	if _, err := e.svc.Refund(e.ctx, "r3", e.ana, in.ID, brl("1"), ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("refunding someone else's Pix err = %v", err)
	}
	got, _ := e.svc.Transfer(e.ctx, in.ID)
	if got.Refunded != 30_00 {
		t.Fatalf("refunded = %d", got.Refunded)
	}

	// A refund that fails (no funds) gives its reservation back.
	e.txs.balance[e.bruno] = 0
	failed, err := e.svc.Refund(e.ctx, "r4", e.bruno, in.ID, brl("70"), "")
	if err != nil || failed.Status != domain.StatusRejected {
		t.Fatalf("refund without funds = %+v, %v", failed, err)
	}
	if got, _ := e.svc.Transfer(e.ctx, in.ID); got.Refunded != 30_00 {
		t.Fatalf("failed refund kept its reservation: %d", got.Refunded)
	}

	sent, _ := e.svc.Send(e.ctx, "out", e.ana, "maria@outrobanco.com.br", brl("1"), "")
	if _, err := e.svc.Refund(e.ctx, "r5", e.ana, sent.ID, brl("1"), ""); !errors.Is(err, domain.ErrNotRefundable) {
		t.Fatalf("refunding an outgoing Pix err = %v", err)
	}
}

func TestConcurrentRefundsNeverExceedTheAmount(t *testing.T) {
	e := setup(t)
	_, _ = e.svc.RegisterKey(e.ctx, e.bruno, domain.KeyEmail, "bruno@example.com")
	in := receive(t, e, "bruno@example.com", "100")
	e.txs.balance[e.bruno] = 1_000_00

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() { _, _ = e.svc.Refund(e.ctx, fmt.Sprint("c", i), e.bruno, in.ID, brl("30"), "") })
	}
	wg.Wait()
	got, _ := e.svc.Transfer(e.ctx, in.ID)
	var completed int64
	_ = e.pool.QueryRow(e.ctx, `SELECT COALESCE(SUM(amount), 0) FROM pix_transfers WHERE refund_of = $1 AND status = 'COMPLETED'`, in.ID).Scan(&completed)
	if completed != 90_00 || got.Refunded != 90_00 {
		t.Fatalf("refunded %d (completed %d) of 100, want 90 (three refunds of 30)", got.Refunded, completed)
	}
}
