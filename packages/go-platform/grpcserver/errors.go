package grpcserver

import (
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorDomain tags ErrorInfo details produced by this platform.
const ErrorDomain = "digitalbank"

// Error builds a gRPC status error with a machine-readable reason
// (e.g. "INSUFFICIENT_FUNDS") attached as google.rpc.ErrorInfo.
func Error(code codes.Code, reason, msg string) error {
	st := status.New(code, msg)
	if reason == "" {
		return st.Err()
	}
	withDetails, err := st.WithDetails(&errdetails.ErrorInfo{Reason: reason, Domain: ErrorDomain})
	if err != nil {
		return st.Err()
	}
	return withDetails.Err()
}

// Reason extracts the ErrorInfo reason from a gRPC error, or "".
func Reason(err error) string {
	for _, d := range status.Convert(err).Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok {
			return info.Reason
		}
	}
	return ""
}
