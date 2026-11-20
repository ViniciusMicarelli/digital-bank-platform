package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Kind is a sensitive action that needs a second analyst.
type Kind string

const (
	KindUnblockAccount Kind = "UNBLOCK_ACCOUNT"
	// KindLimitIncrease grants a credit limit above what one analyst may
	// grant alone. Target: the limit request; params: account_id,
	// granted_minor (the limit to grant, in centavos).
	KindLimitIncrease Kind = "LIMIT_INCREASE"
)

func (k Kind) Valid() bool { return k == KindUnblockAccount || k == KindLimitIncrease }

// Status of an approval.
type Status string

const (
	StatusPending  Status = "PENDING"
	StatusApproved Status = "APPROVED" // approved, the action is running
	StatusExecuted Status = "EXECUTED"
	StatusRejected Status = "REJECTED"
	StatusFailed   Status = "FAILED" // approved, but the action failed
	StatusExpired  Status = "EXPIRED"
)

// ApprovalTTL is how long a request waits for the second analyst.
const ApprovalTTL = 24 * time.Hour

// Approval is a four-eyes request.
type Approval struct {
	ID          uuid.UUID
	Kind        Kind
	TargetID    string
	Reason      string
	RequestedBy string
	// Params are what the action needs besides its target.
	Params         map[string]string
	Status         Status
	DecidedBy      string
	DecisionReason string
	FailureReason  string
	CreatedAt      time.Time
	DecidedAt      *time.Time
	ExpiresAt      time.Time
}

// NewApproval opens a pending request.
func NewApproval(kind Kind, target, reason, requestedBy string, params map[string]string, now time.Time) (Approval, error) {
	switch {
	case !kind.Valid():
		return Approval{}, invalid("unknown approval kind")
	case strings.TrimSpace(target) == "":
		return Approval{}, invalid("target_id is required")
	case strings.TrimSpace(reason) == "":
		return Approval{}, invalid("a reason is required")
	case strings.TrimSpace(requestedBy) == "":
		return Approval{}, invalid("requested_by is required")
	case kind == KindLimitIncrease && (params["account_id"] == "" || params["granted_minor"] == ""):
		return Approval{}, invalid("a limit increase needs account_id and granted_minor")
	}
	if params == nil {
		params = map[string]string{}
	}
	return Approval{ID: uuid.Must(uuid.NewV7()), Kind: kind, TargetID: target, Reason: strings.TrimSpace(reason),
		RequestedBy: requestedBy, Params: params, Status: StatusPending, CreatedAt: now, ExpiresAt: now.Add(ApprovalTTL)}, nil
}

// Expired reports whether a pending request ran out of time.
func (a Approval) Expired(now time.Time) bool {
	return a.Status == StatusPending && !now.Before(a.ExpiresAt)
}

// Decide records the second analyst's decision. The requester can never
// approve their own request.
func (a *Approval) Decide(approver string, approve bool, reason string, now time.Time) error {
	switch {
	case strings.TrimSpace(approver) == "":
		return invalid("approver_id is required")
	case a.Status != StatusPending:
		return Refuse("APPROVAL_NOT_PENDING", "this request is "+string(a.Status))
	case a.Expired(now):
		return Refuse("APPROVAL_EXPIRED", "this request expired; open a new one")
	case approver == a.RequestedBy:
		return Refuse("SAME_ANALYST", "a different analyst must decide (four-eyes)")
	case !approve && strings.TrimSpace(reason) == "":
		return invalid("a rejection needs a reason")
	}
	a.Status, a.DecidedBy, a.DecisionReason, a.DecidedAt = StatusRejected, approver, strings.TrimSpace(reason), &now
	if approve {
		a.Status = StatusApproved
	}
	return nil
}

// Settle records how the approved action ended.
func (a *Approval) Settle(failure string) {
	if failure == "" {
		a.Status = StatusExecuted
		return
	}
	a.Status, a.FailureReason = StatusFailed, failure
}
