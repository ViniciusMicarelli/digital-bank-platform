package domain

import "errors"

// Errors returned by the domain and application layers. Adapters map them to
// transport codes (gRPC status, HTTP status).
var (
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrNotFound            = errors.New("not found")
	ErrInsufficientFunds   = errors.New("insufficient funds")
	ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")
	ErrAlreadyReversed     = errors.New("entry already reversed")
	ErrHoldNotActive       = errors.New("hold is not active")
	ErrAccountConflict     = errors.New("account already exists with different attributes")
)
