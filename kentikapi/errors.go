package kentikapi

import (
	"errors"

	"google.golang.org/grpc/status"

	kentikerrors "github.com/kentik/community_sdk_golang/kentikapi/internal/errors"
)

// The Is*Error helpers classify errors from both the v6 gRPC calls (*Error, or any error carrying a gRPC
// status) and the legacy v5 REST resources.

// IsAuthError returns true if passed error is an authentication/authorization error.
func IsAuthError(err error) bool { return classify(err) == kentikerrors.AuthError }

// IsInvalidRequestError returns true if passed error is due to invalid request.
// It might be returned if user passed invalid data or there is a resource conflict.
func IsInvalidRequestError(err error) bool { return classify(err) == kentikerrors.InvalidRequest }

// IsNotFoundError returns true if passed error indicates that resource was not found.
func IsNotFoundError(err error) bool { return classify(err) == kentikerrors.NotFound }

// IsRateLimitExhaustedError returns true if passed error indicates that the rate limit for a resource
// has been exhausted.
func IsRateLimitExhaustedError(err error) bool {
	return classify(err) == kentikerrors.RateLimitExhausted
}

// IsTemporaryError returns true if passed error is temporary, e.g. it is due to a timeout,
// rate limit exhaustion or service unavailability.
func IsTemporaryError(err error) bool {
	switch classify(err) {
	case kentikerrors.Timeout, kentikerrors.Unavailable, kentikerrors.RateLimitExhausted:
		return true
	default:
		return false
	}
}

// IsTimeoutError returns true if passed error is a timeout error.
func IsTimeoutError(err error) bool { return classify(err) == kentikerrors.Timeout }

func classify(err error) kentikerrors.Code {
	if err == nil {
		return kentikerrors.Unknown
	}
	var ke kentikerrors.StatusError
	if errors.As(err, &ke) {
		return ke.Code()
	}
	var gs interface{ GRPCStatus() *status.Status }
	if errors.As(err, &gs) {
		var se kentikerrors.StatusError
		if errors.As(kentikerrors.StatusErrorFromGRPC(err), &se) {
			return se.Code()
		}
	}
	return kentikerrors.Unknown
}
