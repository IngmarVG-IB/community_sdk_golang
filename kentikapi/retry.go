package kentikapi

import (
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// RetryPolicy controls how failed unary RPCs are retried. Streaming RPCs are never retried.
//
// The delay before retry n (1-based) is min(MaxBackoff, InitialBackoff * Multiplier^(n-1)), reduced by a
// random fraction of up to Jitter. A server hint (google.rpc.RetryInfo detail or grpc-retry-pushback-ms
// trailer) takes precedence when it is longer.
type RetryPolicy struct {
	// MaxRetries is the number of retries after the first attempt. 0 disables retries. Default 4.
	MaxRetries int
	// InitialBackoff is the delay before the first retry. Default 1s.
	InitialBackoff time.Duration
	// MaxBackoff caps the delay between attempts. Default 30s.
	MaxBackoff time.Duration
	// Multiplier is the backoff growth factor. Default 2.
	Multiplier float64
	// Jitter is the random fraction (0..1) taken off each delay. Default 0.2.
	Jitter float64
	// Codes are the status codes that trigger a retry.
	// Default: Unavailable, ResourceExhausted, DeadlineExceeded.
	Codes []codes.Code
	// Retryable decides whether a method may be retried at all. fullMethod has the form
	// "/kentik.synthetics.v202309.SyntheticsAdminService/ListTests". The default, [IsIdempotent], allows
	// only RPCs that are safe to repeat (HTTP GET mapping or idempotency_level set in the proto).
	// Return true for everything to also retry mutating RPCs.
	Retryable func(fullMethod string) bool
}

// Default retry settings.
const (
	DefaultMaxRetries     = 4
	DefaultInitialBackoff = 1 * time.Second
	DefaultMaxBackoff     = 30 * time.Second
	DefaultMultiplier     = 2.0
	DefaultJitter         = 0.2
)

// DefaultRetryPolicy returns the policy used when no retry option is given.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:     DefaultMaxRetries,
		InitialBackoff: DefaultInitialBackoff,
		MaxBackoff:     DefaultMaxBackoff,
		Multiplier:     DefaultMultiplier,
		Jitter:         DefaultJitter,
		Codes:          []codes.Code{codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded},
		Retryable:      IsIdempotent,
	}
}

// normalized fills zero fields with defaults so that a partially populated policy behaves sensibly.
func (p RetryPolicy) normalized() RetryPolicy {
	d := DefaultRetryPolicy()
	if p.MaxRetries < 0 {
		p.MaxRetries = 0
	}
	if p.InitialBackoff <= 0 {
		p.InitialBackoff = d.InitialBackoff
	}
	if p.MaxBackoff <= 0 {
		p.MaxBackoff = d.MaxBackoff
	}
	if p.MaxBackoff < p.InitialBackoff {
		p.MaxBackoff = p.InitialBackoff
	}
	if p.Multiplier < 1 {
		p.Multiplier = d.Multiplier
	}
	p.Jitter = min(max(p.Jitter, 0), 1)
	if p.Codes == nil {
		p.Codes = d.Codes
	}
	if p.Retryable == nil {
		p.Retryable = d.Retryable
	}
	return p
}

func (p RetryPolicy) retryOn(c codes.Code) bool { return slices.Contains(p.Codes, c) }

// backoff returns the delay before retry number n (1-based).
func (p RetryPolicy) backoff(n int, rnd func() float64) time.Duration {
	d := float64(p.InitialBackoff) * math.Pow(p.Multiplier, float64(n-1))
	d = min(d, float64(p.MaxBackoff))
	d -= d * p.Jitter * rnd()
	return time.Duration(d)
}

func defaultRand() float64 { return rand.Float64() } //nolint:gosec // jitter, not security sensitive

// serverRetryDelay extracts a server-requested delay, if any.
func serverRetryDelay(err error, trailer metadata.MD) (time.Duration, bool) {
	if st, ok := status.FromError(err); ok {
		for _, d := range st.Details() {
			if ri, ok := d.(*errdetails.RetryInfo); ok && ri.GetRetryDelay() != nil {
				return ri.GetRetryDelay().AsDuration(), true
			}
		}
	}
	if v := trailer.Get("grpc-retry-pushback-ms"); len(v) > 0 {
		if ms, err := strconv.ParseInt(strings.TrimSpace(v[0]), 10, 64); err == nil && ms >= 0 {
			return time.Duration(ms) * time.Millisecond, true
		}
	}
	return 0, false
}

// IsIdempotent reports whether fullMethod is safe to retry: its google.api.http mapping is a GET, or the
// proto sets idempotency_level. Methods unknown to the SDK's registry fall back to a name heuristic
// (Get*, List*, Search*, Query*, Fetch* are treated as read-only).
func IsIdempotent(fullMethod string) bool {
	if m, ok := LookupMethod(fullMethod); ok {
		return m.Idempotent
	}
	_, name := splitMethod(fullMethod)
	for _, prefix := range []string{"Get", "List", "Search", "Query", "Fetch"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
