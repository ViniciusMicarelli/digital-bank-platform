// Package ports declares what the back-office application needs.
package ports

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ViniciusMicarelli/digital-bank-platform/services/backoffice-service/internal/domain"
)

// AuditFilter narrows ListAudit; empty fields match everything.
type AuditFilter struct {
	TargetID, ActorID string
	Limit             int
}

type Store interface {
	InTx(ctx context.Context, fn func(Tx) error) error
	ListAudit(ctx context.Context, f AuditFilter) ([]domain.Event, error)
	// ScanAudit visits every event in chain order.
	ScanAudit(ctx context.Context, visit func(domain.Event) error) error
	Approval(ctx context.Context, id uuid.UUID) (domain.Approval, error)
	ListApprovals(ctx context.Context, status domain.Status, limit int) ([]domain.Approval, error)
	// ApprovedBefore lists approved actions still running (for recovery).
	ApprovedBefore(ctx context.Context, before time.Time) ([]domain.Approval, error)
}

type Tx interface {
	// AppendAudit chains e after the last event; appends are serialized.
	AppendAudit(ctx context.Context, e domain.Event, now time.Time) (domain.Event, error)
	// InsertApproval fails with a refusal (APPROVAL_ALREADY_PENDING) when
	// the same action is already open.
	InsertApproval(ctx context.Context, a domain.Approval) error
	ApprovalForUpdate(ctx context.Context, id uuid.UUID) (domain.Approval, error)
	SaveApproval(ctx context.Context, a domain.Approval) error
	// ExpirePending marks the pending requests past their deadline EXPIRED.
	ExpirePending(ctx context.Context, now time.Time) ([]domain.Approval, error)
}

// Executor runs an approved action. Permanent failures are domain
// refusals; any other error is retried.
type Executor interface {
	Execute(ctx context.Context, a domain.Approval) error
}
