// Package go-errors provides a standardized error handling and propagation system for the Digital Bank Platform.
// This enables consistent error handling across all microservices.
package errors

import (
	"errors"
	"fmt"
	"time"

	"github.com/open-feature/go-sdk/openfeature"
)

// ErrorCode represents the category of an error.
type ErrorCode string

const (
	// System codes
	CodeInvalidRequest ErrorCode = "INVALID_REQUEST"
	CodeUnauthorized   ErrorCode = "UNAUTHORIZED"
	CodeForbidden      ErrorCode = "FORBIDDEN"
	CodeNotFound       ErrorCode = "NOT_FOUND"
	CodeConflict       ErrorCode = "CONFLICT"
	CodeBadRequest     ErrorCode = "BAD_REQUEST"
	CodeInternal       ErrorCode = "INTERNAL"
	CodeTimeout        ErrorCode = "TIMEOUT"
	CodeRateLimited    ErrorCode = "RATE_LIMITED"
	CodeServiceBusy    ErrorCode = "SERVICE_BUSY"

	// Domain codes
	CodeAccountNotFound        ErrorCode = "ACCOUNT_NOT_FOUND"
	CodeUserNotFound           ErrorCode = "USER_NOT_FOUND"
	CodeInvalidPin             ErrorCode = "INVALID_PIN"
	CodeCardNotFound           ErrorCode = "CARD_NOT_FOUND"
	CodeCardBlocked            ErrorCode = "CARD_BLOCKED"
	CodeLimitExceeded          ErrorCode = "LIMIT_EXCEEDED"
	CodeInsufficientBalance    ErrorCode = "INSUFFICIENT_BALANCE"
	CodePixCodeInvalid        ErrorCode = "PIX_CODE_INVALID"
	CodeTransactionNotFound    ErrorCode = "TRANSACTION_NOT_FOUND"
	CodeLedgerEntryNotFound    ErrorCode = "LEDGER_ENTRY_NOT_FOUND"
	CodeStatementNotFound      ErrorCode = "STATEMENT_NOT_FOUND"
	CodeNotificationDisabled   ErrorCode = "NOTIFICATION_DISABLED"
	CodeComplianceViolation   ErrorCode = "COMPLIANCE_VIOLATION"
	CodeSecurityViolation     ErrorCode = "SECURITY_VIOLATION"
)

// ErrCode is the error code type alias.
type ErrCode = ErrorCode

// ErrorCodeGroup defines error codes for a specific service.
type ErrorCodeGroup string

const (
	GroupIdentity     ErrorCodeGroup = "IDENTITY"
	GroupAuth         ErrorCodeGroup = "AUTH"
	GroupPayment      ErrorCodeGroup = "PAYMENT"
	GroupTransaction  ErrorCodeGroup = "TRANSACTION"
	GroupAccount      ErrorCodeGroup = "ACCOUNT"
	GroupCard         ErrorCodeGroup = "CARD"
	GroupCredit       ErrorCodeGroup = "CREDIT"
	GroupNotification ErrorCodeGroup = "NOTIFICATION"
	GroupLedger       ErrorCodeGroup = "LEDGER"
	GroupStatement    ErrorCodeGroup = "STATEMENT"
	GroupAPI          ErrorCodeGroup = "API"
	GroupGateway      ErrorCodeGroup = "GATEWAY"
)

// Error is the base error type for the platform.
type Error struct {
	// System-defined properties
	Code       ErrCode    `json:"code"`
	Message    string      `json:"message"`
	Detail     string      `json:"detail,omitempty"`
	Stack      string      `json:"stack,omitempty"`
	Timestamp  string      `json:"timestamp"`
	TraceID    string      `json:"trace_id"`
	Source     string      `json:"source,omitempty"`
	SourceLine string      `json:"source_line,omitempty"`

	// Domain-defined properties
	Service      ErrorCodeGroup `json:"service"`
	Cause        Error         `json:"cause,omitempty"`
	Errors       []Error       `json:"errors,omitempty"`

	// Metadata for internal use
	ErrorCode   ErrorCode `json:"error_code,omitempty"`
	CodeGroup   ErrorCodeGroup `json:"code_group,omitempty"`
	CodeDomain  ErrorCodeGroup `json:"code_domain,omitempty"`

	// Observability
	Severity string `json:"severity"`
}

// error is the internal error type.
type error struct {
	Code       ErrCode    `json:"code"`
	Message    string      `json:"message"`
	Detail     string      `json:"detail,omitempty"`
	Stack      string      `json:"stack,omitempty"`
	Timestamp  string      `json:"timestamp"`
	TraceID    string      `json:"trace_id"`
	Source     string      `json:"source,omitempty"`
	SourceLine string      `json:"source_line,omitempty"`

	Service      ErrorCodeGroup `json:"service"`
	Cause        error         `json:"cause,omitempty"`
	Errors       []error       `json:"errors,omitempty"`
}

// Wrap adds a wrapper around an existing error.
func Wrap(err error, service ErrorCodeGroup, code ErrCode) error {
	if err == nil {
		return nil
	}
	return &error{
		Service:  service,
		Code:     code,
		Message:  fmt.Sprintf("[%s] %s", code, err.Error()),
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

// WithTraceID adds a trace ID to an error.
func WithTraceID(err error, traceID string) error {
	if err == nil {
		return nil
	}
	return &error{
		Service:  err.Code,
		Code:     err.Code,
		Message:  err.Message,
		TraceID:  traceID,
		Timestamp: err.Timestamp,
	}
}

// Wrapf creates an error with a formatted message.
func Wrapf(service ErrorCodeGroup, code ErrCode, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	return &error{
		Service:  service,
		Code:     code,
		Message:  msg,
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

// WithCause wraps an error with the original error as the cause.
func WithCause(wrapped, cause error) error {
	return &error{
		Service:  wrapped.Service,
		Code:     wrapped.Code,
		Message:  wrapped.Message,
		Timestamp: wrapped.Timestamp,
		Cause:    cause,
	}
}

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("[%s] %s (code: %s, source: %s)",
		e.Service, e.Message, e.Code, e.Source)
}

// Is returns true if the error matches the given error code.
func (e *Error) Is(target error) bool {
	if t, ok := target.(*Error); ok {
		return e.Code == t.Code
	}
	return false
}

// IsCode returns true if the error code matches the given code.
func (e *Error) IsCode(code ErrCode) bool {
	return e.Code == code
}

// IsSystem returns true if the error is a system error (not domain-specific).
func (e *Error) IsSystem() bool {
	return e.Service == "" || e.Service == ErrorCodeGroup(API)
}

// IsDomain returns true if the error is a domain-specific error.
func (e *Error) IsDomain() bool {
	return e.Service != "" && e.Service != ErrorCodeGroup(API)
}

// IsClientError returns true if the error is a client-facing error (4xx).
func (e *Error) IsClientError() bool {
	return e.Code == CodeUnauthorized ||
		e.Code == CodeForbidden ||
		e.Code == CodeNotFound ||
		e.Code == CodeBadRequest ||
		e.Code == CodeRateLimited ||
		e.Code == CodeInvalidRequest
}

// IsServerError returns true if the error is a server-side error (5xx).
func (e *Error) IsServerError() bool {
	return e.Code == CodeInternal ||
		e.Code == CodeTimeout ||
		e.Code == CodeServiceBusy
}

// HasCause returns true if the error has a cause.
func (e *Error) HasCause() bool {
	return e.Cause != nil
}

// GetCause returns the cause error, or nil if there is none.
func (e *Error) GetCause() error {
	return e.Cause
}

// Chain appends errors to this error's error chain.
func (e *Error) Chain(newErrors ...error) error {
	e.Errors = append(e.Errors, newErrors...)
	return e
}

// WithDetail adds detail to the error.
func (e *Error) WithDetail(detail string) error {
	return &Error{
		Service: e.Service,
		Code: e.Code,
		Message: e.Message,
		Detail: detail,
		Source: e.Source,
		SourceLine: e.SourceLine,
		Timestamp: e.Timestamp,
		TraceID: e.TraceID,
		Cause: e.Cause,
		Errors: append([]Error(nil), e.Errors...),
	}
}

// IsOpenFeatureError returns true if the error is from OpenFeature (feature flags).
func (e *Error) IsOpenFeatureError() bool {
	return e.Code == CodeServiceBusy
}

// FeatureErrorCode returns the error code if the error is from OpenFeature.
func (e *Error) FeatureErrorCode() string {
	// OpenFeature errors are identified by their code
	return string(CodeServiceBusy)
}

// NewError creates a new error with the given service and code.
func NewError(service ErrorCodeGroup, code ErrCode) error {
	return Wrapf(service, code, "internal error")
}

// FromError converts an interface error to our Error type.
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return &Error{
		Message: err.Error(),
		Code:    CodeInternal,
		Source:  "unknown",
		Timestamp: time.Now().Format(time.RFC3339),
		Cause:   err,
	}
}
