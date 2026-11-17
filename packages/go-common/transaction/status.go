package transaction

// TransactionStatus represents the status of a transaction.
type TransactionStatus string

const (
	// Pending is the initial status for a transaction.
	Pending TransactionStatus = "PENDING"
	
	// Authorized is the status when payment is authorized.
	Authorized TransactionStatus = "AUTHORIZED"
	
	// Approved is the status when payment is approved.
	Approved TransactionStatus = "APPROVED"
	
	// Declined is the status when payment is declined.
	Declined TransactionStatus = "DECLINED"
	
	// Captured is the status when payment is captured.
	Captured TransactionStatus = "CAPTURED"
	
	// Completed is the status when payment is completed.
	Completed TransactionStatus = "COMPLETED"
	
	// Settled is the status when payment is settled.
	Settled TransactionStatus = "SETTLED"
	
	// Reversing is the status when a transaction is being reversed.
	Reversing TransactionStatus = "REVERSING"
	
	// Reversed is the status when a transaction is reversed.
	Reversed TransactionStatus = "REVERSED"
	
	// Voided is the status when a transaction is voided.
	Voided TransactionStatus = "VOIDED"
	
	// Refunded is the status when a transaction is refunded.
	Refunded TransactionStatus = "REFUNDED"
	
	// Expired is the status when a Pix transaction expires.
	Expired TransactionStatus = "EXPIRED"
	
	// InsufficientFunds is the status when there are insufficient funds.
	InsufficientFunds TransactionStatus = "INSUFFICIENT_FUNDS"
	
	// Blocked is the status when the transaction is blocked.
	Blocked TransactionStatus = "BLOCKED"
)

// GetNextStatus returns the next status in the transaction lifecycle.
func (s TransactionStatus) GetNextStatus() TransactionStatus {
	switch s {
	case Pending:
		return Authorized
	case Authorized:
		return Approved
	case Approved:
		return Captured
	case Captured:
		return Completed
	case Completed:
		return Settled
	default:
		return s
	}
}

// CanTransitionTo returns true if the transaction can transition to the given status.
func (s TransactionStatus) CanTransitionTo(t TransactionStatus) bool {
	switch s {
	case Pending:
		return t == Authorized || t == Declined || t == Expired || t == Blocked
	case Authorized:
		return t == Approved || t == Declined
	case Approved:
		return t == Captured || t == Declined
	case Captured:
		return t == Completed || t == Reversing || t == Voided
	case Completed:
		return t == Settled || t == Reversing || t == Refunded
	case Settled:
		return t == Reversed
	default:
		return false
	}
}

// GetReverseStatus returns the reverse transaction status.
func (s TransactionStatus) GetReverseStatus() TransactionStatus {
	switch s {
	case Pending:
		return Pending
	case Authorized:
		return Reversed
	case Approved:
		return Reversed
	case Captured:
		return Reversed
	case Completed:
		return Refunded
	case Settled:
		return Reversed
	default:
		return Pending
	}
}

// IsPending returns true if the status is in the pending phase.
func (s TransactionStatus) IsPending() bool {
	return s == Pending
}

// IsAuthorized returns true if the status is in the authorized phase.
func (s TransactionStatus) IsAuthorized() bool {
	return s == Authorized
}

// IsSettled returns true if the status is in the settled phase.
func (s TransactionStatus) IsSettled() bool {
	return s == Settled
}

// IsFinal returns true if the status is a final status.
func (s TransactionStatus) IsFinal() bool {
	return s == Reversed || s == Refunded || s == Voided
}

// IsCompleted returns true if the status is completed.
func (s TransactionStatus) IsCompleted() bool {
	return s == Completed
}