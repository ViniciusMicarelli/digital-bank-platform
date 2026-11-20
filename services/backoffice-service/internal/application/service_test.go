package application_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/ports"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/migrations"
)

// fakeExec unblocks accounts; it can be down or refuse.
type fakeExec struct {
	mu       sync.Mutex
	down     bool
	refuse   string
	executed []string
}

func (f *fakeExec) Execute(_ context.Context, a domain.Approval) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.down:
		return errors.New("account-service unavailable")
	case f.refuse != "":
		return domain.Refuse(f.refuse, "no")
	}
	f.executed = append(f.executed, a.TargetID)
	return nil
}

type env struct {
	ctx  context.Context
	svc  *application.Service
	pool *pgxpool.Pool
	exec *fakeExec
	now  time.Time
}

func setup(t *testing.T) *env {
	t.Helper()
	pool := pgtest.NewDB(t, migrations.FS, ".")
	e := &env{ctx: context.Background(), pool: pool, exec: &fakeExec{}, now: time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)}
	e.svc = application.New(pgstore.NewStore(pool), e.exec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.svc.SetClock(func() time.Time { return e.now })
	return e
}

func reason(err error) string {
	if r, ok := domain.AsRefused(err); ok {
		return r.Reason
	}
	return ""
}

func TestAuditChain(t *testing.T) {
	e := setup(t)
	if _, err := e.svc.Record(e.ctx, domain.Event{Action: "X"}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatal("an event needs an actor")
	}
	for i, action := range []string{"CUSTOMER_FILE_VIEWED", "KYC_APPROVED", "ACCOUNT_BLOCKED"} {
		ev, err := e.svc.Record(e.ctx, domain.Event{ActorID: "a1", Action: action, TargetType: "customer", TargetID: "c1",
			Details: map[string]string{"n": string(rune('0' + i))}})
		if err != nil || ev.Seq != int64(i+1) {
			t.Fatalf("record %d = %+v %v", i, ev, err)
		}
	}
	_, _ = e.svc.Record(e.ctx, domain.Event{ActorID: "a2", Action: "CUSTOMER_FILE_VIEWED", TargetID: "c2"})
	list, _ := e.svc.Audit(e.ctx, ports.AuditFilter{TargetID: "c1"})
	if len(list) != 3 || list[0].Action != "ACCOUNT_BLOCKED" || list[0].Details["n"] != "2" {
		t.Fatalf("by target = %+v", list)
	}
	if list, _ := e.svc.Audit(e.ctx, ports.AuditFilter{ActorID: "a2"}); len(list) != 1 {
		t.Fatalf("by actor = %+v", list)
	}
	if v, err := e.svc.Verify(e.ctx); err != nil || !v.Intact || v.Events != 4 {
		t.Fatalf("verify = %+v %v", v, err)
	}

	// The table refuses edits and deletes.
	if _, err := e.pool.Exec(e.ctx, `UPDATE audit_events SET actor_id = 'someone-else' WHERE seq = 2`); err == nil {
		t.Fatal("audit events must be append-only")
	}
	if _, err := e.pool.Exec(e.ctx, `DELETE FROM audit_events WHERE seq = 4`); err == nil {
		t.Fatal("audit events cannot be deleted")
	}
	// Someone with enough privileges to bypass the trigger still breaks the chain.
	for _, q := range []string{
		`ALTER TABLE audit_events DISABLE TRIGGER audit_events_append_only`,
		`UPDATE audit_events SET details = '{"n":"9"}' WHERE seq = 2`,
	} {
		if _, err := e.pool.Exec(e.ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if v, _ := e.svc.Verify(e.ctx); v.Intact || v.BrokenAt != 2 {
		t.Fatalf("tampered = %+v", v)
	}
}

func TestFourEyes(t *testing.T) {
	e := setup(t)
	a, err := e.svc.Request(e.ctx, domain.KindUnblockAccount, "acc-1", "cliente confirmou a identidade", "a1", nil)
	if err != nil || a.Status != domain.StatusPending {
		t.Fatalf("request = %+v %v", a, err)
	}
	if _, err := e.svc.Request(e.ctx, domain.KindUnblockAccount, "acc-1", "de novo", "a2", nil); reason(err) != "APPROVAL_ALREADY_PENDING" {
		t.Fatalf("second request: %v", err)
	}
	if _, err := e.svc.Decide(e.ctx, a.ID, "a1", true, ""); reason(err) != "SAME_ANALYST" {
		t.Fatalf("self approval: %v", err)
	}
	done, err := e.svc.Decide(e.ctx, a.ID, "a2", true, "")
	if err != nil || done.Status != domain.StatusExecuted || done.DecidedBy != "a2" || len(e.exec.executed) != 1 {
		t.Fatalf("approve = %+v %v (%v)", done, err, e.exec.executed)
	}
	if _, err := e.svc.Decide(e.ctx, a.ID, "a3", true, ""); reason(err) != "APPROVAL_NOT_PENDING" {
		t.Fatalf("decide twice: %v", err)
	}

	// Rejected: nothing runs.
	r, _ := e.svc.Request(e.ctx, domain.KindUnblockAccount, "acc-2", "pedido do cliente", "a1", nil)
	if _, err := e.svc.Decide(e.ctx, r.ID, "a2", false, ""); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatal("a rejection needs a reason")
	}
	if r, err := e.svc.Decide(e.ctx, r.ID, "a2", false, "indícios de fraude"); err != nil || r.Status != domain.StatusRejected {
		t.Fatalf("reject = %+v %v", r, err)
	}

	// The action itself refuses: FAILED with the reason.
	e.exec.refuse = "ACCOUNT_NOT_BLOCKED"
	f, _ := e.svc.Request(e.ctx, domain.KindUnblockAccount, "acc-3", "pedido", "a1", nil)
	if f, _ := e.svc.Decide(e.ctx, f.ID, "a2", true, ""); f.Status != domain.StatusFailed || f.FailureReason != "ACCOUNT_NOT_BLOCKED" {
		t.Fatalf("refused action = %+v", f)
	}

	// Every step is in the trail, by the analyst who took it.
	events, _ := e.svc.Audit(e.ctx, ports.AuditFilter{TargetID: "acc-1"})
	want := []string{"APPROVAL_EXECUTED", "APPROVAL_APPROVED", "APPROVAL_REQUESTED"}
	if len(events) != 3 {
		t.Fatalf("trail = %+v", events)
	}
	for i, ev := range events {
		if ev.Action != want[i] {
			t.Fatalf("trail[%d] = %s, want %s", i, ev.Action, want[i])
		}
	}
	if events[1].ActorID != "a2" || events[2].ActorID != "a1" {
		t.Fatalf("actors = %s, %s", events[1].ActorID, events[2].ActorID)
	}
	if v, _ := e.svc.Verify(e.ctx); !v.Intact {
		t.Fatalf("verify = %+v", v)
	}
}

func TestRecoveryAndExpiry(t *testing.T) {
	e := setup(t)
	a, _ := e.svc.Request(e.ctx, domain.KindUnblockAccount, "acc-1", "pedido", "a1", nil)
	e.exec.down = true
	if a, err := e.svc.Decide(e.ctx, a.ID, "a2", true, ""); err != nil || a.Status != domain.StatusApproved {
		t.Fatalf("approved during an outage = %+v %v", a, err)
	}
	e.exec.down = false
	e.now = e.now.Add(time.Minute)
	e.svc.RecoverOnce(e.ctx, 30*time.Second)
	if a, _ := e.svc.Approval(e.ctx, a.ID); a.Status != domain.StatusExecuted {
		t.Fatalf("recovered = %+v", a)
	}

	old, _ := e.svc.Request(e.ctx, domain.KindUnblockAccount, "acc-2", "pedido", "a1", nil)
	late, _ := e.svc.Request(e.ctx, domain.KindUnblockAccount, "acc-3", "pedido", "a1", nil)
	e.now = e.now.Add(domain.ApprovalTTL)
	if _, err := e.svc.Decide(e.ctx, late.ID, "a2", true, ""); reason(err) != "APPROVAL_EXPIRED" {
		t.Fatalf("late decision: %v", err)
	}
	e.svc.RecoverOnce(e.ctx, 30*time.Second)
	if a, _ := e.svc.Approval(e.ctx, old.ID); a.Status != domain.StatusExpired {
		t.Fatalf("stale request = %+v", a)
	}
	if a, _ := e.svc.Approval(e.ctx, late.ID); a.Status != domain.StatusExpired {
		t.Fatalf("late request = %+v", a)
	}
	// An expired request no longer blocks a new one.
	if _, err := e.svc.Request(e.ctx, domain.KindUnblockAccount, "acc-2", "pedido", "a1", nil); err != nil {
		t.Fatal(err)
	}
	if list, _ := e.svc.Approvals(e.ctx, domain.StatusPending, 10); len(list) != 1 {
		t.Fatalf("pending = %+v", list)
	}
}
