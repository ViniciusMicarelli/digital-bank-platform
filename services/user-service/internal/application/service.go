// Package application runs onboarding: the customer profile and its KYC
// verification against the identity bureau.
package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/outbox"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/user-service/internal/ports"
)

const (
	source = "user-service"
	topic  = "bank.user.events"
)

var saoPaulo = func() *time.Location {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("BRT", -3*3600)
	}
	return loc
}()

type Service struct {
	store  ports.Store
	bureau ports.Bureau
	log    *slog.Logger
	now    func() time.Time
}

func New(store ports.Store, bureau ports.Bureau, log *slog.Logger) *Service {
	return &Service{store: store, bureau: bureau, log: log, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) today() time.Time {
	y, m, d := s.now().In(saoPaulo).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func (s *Service) Profile(ctx context.Context, customer string) (domain.Profile, error) {
	return s.store.Profile(ctx, customer)
}

// PutProfile creates or updates the customer's profile.
func (s *Service) PutProfile(ctx context.Context, customer string, in domain.Profile) (domain.Profile, error) {
	in.CustomerID = customer
	if err := in.Normalize(); err != nil {
		return domain.Profile{}, err
	}
	p, err := s.store.Profile(ctx, customer)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		p = in
		p.KYCStatus = domain.KYCNotStarted
	case err != nil:
		return domain.Profile{}, err
	default:
		if err := p.Update(in); err != nil {
			return domain.Profile{}, err
		}
	}
	return p, s.store.Save(ctx, p)
}

// SubmitKYC starts verification and asks the bureau. If the bureau does not
// answer, the verification stays PENDING and the worker retries it.
func (s *Service) SubmitKYC(ctx context.Context, customer string) (domain.Profile, error) {
	p, err := s.store.Profile(ctx, customer)
	if err != nil {
		return domain.Profile{}, err
	}
	before := p.KYCStatus
	if err := p.Submit(s.today()); err != nil {
		return domain.Profile{}, err
	}
	if p.KYCStatus == before {
		return p, nil // already submitted
	}
	if p.KYCStatus == domain.KYCRejected { // decided locally (age)
		return p, s.save(ctx, p)
	}
	if err := s.store.Save(ctx, p); err != nil {
		return domain.Profile{}, err
	}
	if err := s.verify(ctx, &p); err != nil {
		s.log.WarnContext(ctx, "bureau unavailable, verification stays pending", "customer_id", customer, "error", err)
	}
	return p, nil
}

func (s *Service) verify(ctx context.Context, p *domain.Profile) error {
	d, reason, err := s.bureau.Check(ctx, *p)
	if err != nil {
		return err
	}
	if err := p.Decide(d, reason, s.now()); err != nil {
		return err
	}
	return s.save(ctx, *p)
}

// Review records an analyst's decision on a verification in manual review
// (the sandbox exposes it to the customer).
func (s *Service) Review(ctx context.Context, customer string, approve bool, reason, reviewer string) (domain.Profile, error) {
	p, err := s.store.Profile(ctx, customer)
	if err != nil {
		return domain.Profile{}, err
	}
	if p.KYCStatus != domain.KYCManualReview {
		return domain.Profile{}, domain.Refuse("KYC_NOT_IN_REVIEW", "the verification is "+string(p.KYCStatus))
	}
	d := domain.BureauReject
	if approve {
		d, reason = domain.BureauApprove, ""
	}
	if err := p.Decide(d, reason, s.now()); err != nil {
		return domain.Profile{}, err
	}
	p.ReviewedBy = reviewer
	return p, s.save(ctx, p)
}

// Reviews lists the verifications waiting for an analyst, oldest first.
func (s *Service) Reviews(ctx context.Context, limit int) ([]domain.Profile, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	ids, err := s.store.InReview(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Profile, 0, len(ids))
	for _, id := range ids {
		p, err := s.store.Profile(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Service) save(ctx context.Context, p domain.Profile) error {
	ev, err := outbox.NewEvent(ctx, source, "user.kyc."+map[domain.KYCStatus]string{
		domain.KYCApproved: "approved", domain.KYCRejected: "rejected", domain.KYCManualReview: "review",
	}[p.KYCStatus], map[string]any{"customer_id": p.CustomerID, "status": p.KYCStatus, "reason": p.KYCReason, "reviewed_by": p.ReviewedBy})
	if err != nil {
		return err
	}
	return s.store.Save(ctx, p, outbox.Message{Topic: topic, Key: p.CustomerID, Event: ev})
}

// RetryOnce asks the bureau again for verifications left pending.
func (s *Service) RetryOnce(ctx context.Context, olderThan time.Duration) int {
	ids, err := s.store.Pending(ctx, time.Now().Add(-olderThan), 100)
	if err != nil {
		s.log.ErrorContext(ctx, "pending verifications", "error", err)
		return 0
	}
	n := 0
	for _, id := range ids {
		p, err := s.store.Profile(ctx, id)
		if err == nil {
			err = s.verify(ctx, &p)
		}
		if err != nil {
			s.log.WarnContext(ctx, "verification retry failed", "customer_id", id, "error", err)
			continue
		}
		n++
	}
	return n
}

func (s *Service) RunRetries(ctx context.Context, interval, olderThan time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RetryOnce(ctx, olderThan)
		}
	}
}
