package application_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/adapters/bureau"
	pgstore "github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/adapters/postgres"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/application"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/migrations"
)

type flakyBureau struct {
	down bool
	bureau.Simulator
}

func (f *flakyBureau) Check(ctx context.Context, p domain.Profile) (domain.BureauDecision, string, error) {
	if f.down {
		return "", "", errors.New("bureau timeout")
	}
	return f.Simulator.Check(ctx, p)
}

func profile(cpf string) domain.Profile {
	return domain.Profile{FullName: "Ana Souza", CPF: cpf, BirthDate: time.Date(1990, 5, 20, 0, 0, 0, 0, time.UTC),
		Address: domain.Address{PostalCode: "01310100", Street: "Av. Paulista", Number: "1000", City: "São Paulo", State: "SP"}}
}

func reason(err error) string {
	var r *domain.RefusedError
	if errors.As(err, &r) {
		return r.Reason
	}
	return ""
}

func TestOnboarding(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.NewDB(t, migrations.FS, ".")
	store, err := pgstore.NewStore(pool, bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	b := &flakyBureau{}
	svc := application.New(store, b, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if _, err := svc.SubmitKYC(ctx, "ana"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("no profile, nothing to verify")
	}
	if _, err := svc.PutProfile(ctx, "ana", profile("529.982.247-25")); err != nil {
		t.Fatal(err)
	}
	p, err := svc.SubmitKYC(ctx, "ana")
	if err != nil || p.KYCStatus != domain.KYCApproved {
		t.Fatalf("submit = %+v %v", p, err)
	}
	var dump string
	_ = pool.QueryRow(ctx, `SELECT row_to_json(p)::text FROM profiles p`).Scan(&dump)
	if bytes.Contains([]byte(dump), []byte("52998224725")) {
		t.Fatal("CPF stored in clear text")
	}

	// The same person cannot be a second customer.
	_, _ = svc.PutProfile(ctx, "impostor", profile("52998224725"))
	if _, err := svc.SubmitKYC(ctx, "impostor"); reason(err) != "DOCUMENT_IN_USE" {
		t.Fatalf("second customer with the same CPF: %v", err)
	}

	// Politically exposed: manual review, then approved by an analyst.
	_, _ = svc.PutProfile(ctx, "pep", profile("222.333.444-05"))
	if p, _ := svc.SubmitKYC(ctx, "pep"); p.KYCStatus != domain.KYCManualReview || p.KYCReason != "PEP" {
		t.Fatalf("pep = %+v", p)
	}
	if p, err := svc.Review(ctx, "pep", true, ""); err != nil || p.KYCStatus != domain.KYCApproved {
		t.Fatalf("review = %+v %v", p, err)
	}

	// Irregular CPF: rejected; correcting the data allows a new attempt.
	_, _ = svc.PutProfile(ctx, "irr", profile("111.444.777-35"))
	if p, _ := svc.SubmitKYC(ctx, "irr"); p.KYCStatus != domain.KYCRejected || p.KYCReason != "CPF_IRREGULAR" {
		t.Fatalf("irregular = %+v", p)
	}

	// The bureau is down: pending, then the worker finishes it.
	b.down = true
	_, _ = svc.PutProfile(ctx, "late", profile("390.533.447-05"))
	if p, _ := svc.SubmitKYC(ctx, "late"); p.KYCStatus != domain.KYCPending {
		t.Fatalf("bureau down = %+v", p)
	}
	b.down = false
	svc.RetryOnce(ctx, -time.Second)
	if p, _ := svc.Profile(ctx, "late"); p.KYCStatus != domain.KYCApproved {
		t.Fatalf("after retry = %+v", p)
	}
	var events int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&events)
	if events != 5 { // ana, pep review, pep approved, irr rejected, late approved
		t.Fatalf("events = %d", events)
	}
}
