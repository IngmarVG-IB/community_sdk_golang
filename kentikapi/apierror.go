package kentikapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Error is returned by every v6 RPC made through a [Client]. It wraps the gRPC status returned by the
// server (or synthesised by the client for timeouts and cancellation) and adds context.
//
// Error implements GRPCStatus, so status.FromError, status.Code and errors.Is/As keep working:
//
//	var apiErr *kentikapi.Error
//	if errors.As(err, &apiErr) && apiErr.Code == codes.NotFound { ... }
type Error struct {
	// Service is the fully qualified gRPC service, e.g. "kentik.synthetics.v202309.SyntheticsAdminService".
	Service string
	// Method is the RPC name, e.g. "ListTests".
	Method string
	// Code is the gRPC status code.
	Code codes.Code
	// Message is the status message from the server.
	Message string
	// RequestID is the server-assigned request id, when the response carried one (see [WithRequestIDHeaders]).
	RequestID string
	// Attempts is the number of attempts made, including retries.
	Attempts int

	status *status.Status
	cause  error
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("kentikapi: ")
	b.WriteString(shortService(e.Service))
	b.WriteByte('/')
	b.WriteString(e.Method)
	b.WriteString(": ")
	b.WriteString(e.Code.String())
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " (request id %s)", e.RequestID)
	}
	if e.Attempts > 1 {
		fmt.Fprintf(&b, " after %d attempts", e.Attempts)
	}
	return b.String()
}

// Unwrap returns the underlying error (the gRPC status error, or a context error).
func (e *Error) Unwrap() error { return e.cause }

// GRPCStatus returns the gRPC status. It makes status.FromError and status.Code work on *Error.
func (e *Error) GRPCStatus() *status.Status { return e.status }

// Details returns the status details (e.g. errdetails.BadRequest), if the server sent any.
func (e *Error) Details() []any { return e.status.Details() }

// Temporary reports whether retrying the call later may succeed.
func (e *Error) Temporary() bool {
	switch e.Code {
	case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded, codes.Aborted:
		return true
	default:
		return false
	}
}

// newError wraps err returned for fullMethod.
func newError(fullMethod string, err error, requestID string, attempts int) *Error {
	var existing *Error
	if errors.As(err, &existing) {
		return existing
	}
	var st *status.Status
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		st = status.FromContextError(err)
	default:
		st = status.Convert(err)
	}
	svc, method := splitMethod(fullMethod)
	return &Error{
		Service:   svc,
		Method:    method,
		Code:      st.Code(),
		Message:   st.Message(),
		RequestID: requestID,
		Attempts:  attempts,
		status:    st,
		cause:     err,
	}
}

// splitMethod splits "/pkg.Service/Method" into "pkg.Service" and "Method".
func splitMethod(fullMethod string) (service, method string) {
	s := strings.TrimPrefix(fullMethod, "/")
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}

// shortService turns "kentik.synthetics.v202309.SyntheticsAdminService" into
// "synthetics.v202309.SyntheticsAdminService" for compact messages.
func shortService(svc string) string {
	return strings.TrimPrefix(svc, "kentik.")
}
