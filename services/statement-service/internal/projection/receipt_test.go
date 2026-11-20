package projection_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/pgtest"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/statement-service/internal/projection"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/statement-service/migrations"
)

func TestReceipts(t *testing.T) {
	pool := pgtest.NewDB(t, migrations.FS, ".")
	p := projection.New(pool)
	p.SetReceiptKey([]byte("test key"))
	ctx := context.Background()
	ana, bruno := uuid.New(), uuid.New()
	t0 := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	for _, ev := range []outbox.Event{
		opened(t, ana, "Ana Souza"), opened(t, bruno, "Bruno Lima"),
		completed(t, "INTERNAL_TRANSFER", ana, bruno, 42_00, t0, nil),
		completed(t, "INTERNAL_TRANSFER", ana, bruno, 1_00, t0.AddDate(0, -1, 0), nil),
	} {
		if err := p.Handle(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	// The period filter keeps November only.
	items, _, err := p.FeedBetween(ctx, ana, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), "", 10)
	if err != nil || len(items) != 1 || items[0].AmountMinor != 42_00 {
		t.Fatalf("november = %+v %v", items, err)
	}

	r, err := p.Receipt(ctx, ana, items[0].ID)
	if err != nil || r.HolderName != "Ana Souza" || r.CounterpartyName != "Bruno Lima" || r.CounterpartyNumber == "" || len(r.Code) < 40 {
		t.Fatalf("receipt = %+v %v", r, err)
	}
	if _, err := p.Receipt(ctx, bruno, items[0].ID); !errors.Is(err, projection.ErrNotFound) {
		t.Fatal("another account's item")
	}
	got, err := p.Verify(ctx, strings.ToLower(r.Code))
	if err != nil || got.ID != r.ID {
		t.Fatalf("verify = %+v %v", got, err)
	}
	// A tampered code, or one signed with another key, is not genuine.
	// (a character in the middle of the tag: the last one carries padding bits)
	tampered := []byte(r.Code)
	i := len(tampered) - 6
	if tampered[i] == 'A' {
		tampered[i] = 'B'
	} else {
		tampered[i] = 'A'
	}
	if _, err := p.Verify(ctx, string(tampered)); !errors.Is(err, projection.ErrNotFound) {
		t.Fatal("tampered code accepted")
	}
	other := projection.New(pool)
	other.SetReceiptKey([]byte("another key"))
	if _, err := other.Verify(ctx, r.Code); !errors.Is(err, projection.ErrNotFound) {
		t.Fatal("a code from another key accepted")
	}
	if _, err := p.Verify(ctx, "not a code"); !errors.Is(err, projection.ErrNotFound) {
		t.Fatal("garbage accepted")
	}
	if projection.MaskName("Ana Souza Lima") != "Ana S. L." || projection.MaskName("Ágata Ávila") != "Ágata Á." {
		t.Fatal(projection.MaskName("Ana Souza Lima"))
	}
}
