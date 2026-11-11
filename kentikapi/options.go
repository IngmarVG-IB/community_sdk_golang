package kentikapi

import (
	"log/slog"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/kentik/community_sdk_golang/kentikapi/internal/httputil"
)

// Option configures a [Client]. Options are applied in order; later options override earlier ones.
type Option func(*config)

// ClientOption is the former name of [Option].
//
// Deprecated: use Option.
type ClientOption = Option

// RetryConfig configures retries of the legacy v5 REST client.
//
// Deprecated: the v5 REST API is legacy. For v6 gRPC calls use [RetryPolicy] and [WithRetryPolicy].
type RetryConfig = httputil.RetryConfig

const (
	defaultTimeout = 100 * time.Second
)

// DefaultRequestIDHeaders are the response metadata keys searched for a request id.
func DefaultRequestIDHeaders() []string { return []string{"x-request-id"} }

type config struct {
	region   Region
	endpoint string // gRPC target override
	apiURL   string // v5 REST base URL override
	insecure bool

	authEmail, authToken string
	perRPC               credentials.PerRPCCredentials

	userAgent        string
	timeout          time.Duration
	attemptTimeout   time.Duration
	retry            RetryPolicy
	legacyRetry      httputil.RetryConfig
	rateLimit        rate.Limit
	rateBurst        int
	logger           *slog.Logger
	logPayloads      bool
	requestIDHeaders []string
	dialOptions      []grpc.DialOption
	eagerConnect     bool
}

func defaultConfig() config {
	return config{
		region:           RegionUS,
		timeout:          defaultTimeout,
		retry:            DefaultRetryPolicy(),
		requestIDHeaders: DefaultRequestIDHeaders(),
	}
}

// WithRegion selects the Kentik region. Default [RegionUS].
func WithRegion(r Region) Option {
	return func(c *config) { c.region = r }
}

// WithEndpoint overrides the gRPC target, e.g. "grpc.api.kentik.com:443" or a proxy. It takes precedence over
// [WithRegion] for v6 calls.
func WithEndpoint(target string) Option {
	return func(c *config) { c.endpoint = target }
}

// WithInsecure disables TLS. Credentials are then sent in clear text, so use it only with local fakes such as
// package kentikapitest.
func WithInsecure() Option {
	return func(c *config) { c.insecure = true }
}

// WithAPIURL sets the v5 REST base URL, e.g. "https://api.kentik.eu". For backwards compatibility it also
// derives the gRPC endpoint ("grpc." + host, port 443 for https) unless [WithEndpoint] is given, and an
// "http://" URL disables TLS.
//
// Deprecated: use [WithRegion] or [WithEndpoint].
func WithAPIURL(apiURL string) Option {
	return func(c *config) { c.apiURL = apiURL }
}

// WithCredentials sets the Kentik auth email and API token, sent with every call.
// See [GetCredentials] for reading them from the environment or a profile file.
func WithCredentials(authEmail, authToken string) Option {
	return func(c *config) {
		c.authEmail = authEmail
		c.authToken = authToken
	}
}

// WithPerRPCCredentials replaces the email and token credentials for v6 calls with custom ones.
func WithPerRPCCredentials(creds credentials.PerRPCCredentials) Option {
	return func(c *config) { c.perRPC = creds }
}

// WithUserAgent prepends ua to the SDK's user agent ("kentik-community-sdk-golang/<version>").
func WithUserAgent(ua string) Option {
	return func(c *config) { c.userAgent = ua }
}

// WithTimeout limits the total time of a single unary call, retries included. Default 100 seconds.
// 0 keeps the default (as in v0.2); a negative value disables the limit. The caller's context deadline
// always applies. Streaming calls are not limited.
func WithTimeout(timeout time.Duration) Option {
	return func(c *config) {
		if timeout != 0 {
			c.timeout = timeout
		}
	}
}

// WithAttemptTimeout limits each attempt of a unary call. An attempt that times out fails with
// DeadlineExceeded and is retried (if the method is retryable) while the overall timeout allows.
// Default: no per-attempt limit.
func WithAttemptTimeout(timeout time.Duration) Option {
	return func(c *config) { c.attemptTimeout = timeout }
}

// WithRetryPolicy sets the v6 retry policy. Zero fields take their defaults, except MaxRetries (0 disables
// retries). See [DefaultRetryPolicy].
func WithRetryPolicy(p RetryPolicy) Option {
	return func(c *config) { c.retry = p }
}

// WithoutRetries disables retries for both v6 and v5 calls.
func WithoutRetries() Option {
	return func(c *config) {
		c.retry.MaxRetries = 0
		zero := uint(0)
		c.legacyRetry.MaxAttempts = &zero
	}
}

// WithRetryMaxAttempts sets the maximum number of retries (not counting the first attempt) for v6 and v5
// calls. 0 disables retrying. Default 4.
func WithRetryMaxAttempts(maxAttempts uint) Option {
	return func(c *config) {
		c.retry.MaxRetries = int(min(maxAttempts, 1<<16))
		c.legacyRetry.MaxAttempts = &maxAttempts
	}
}

// WithRetryMinDelay sets the delay before the first retry for v6 and v5 calls. Default 1 second.
func WithRetryMinDelay(minDelay time.Duration) Option {
	return func(c *config) {
		c.retry.InitialBackoff = minDelay
		c.legacyRetry.MinDelay = &minDelay
	}
}

// WithRetryMaxDelay caps the delay between retries for v6 and v5 calls. Default 30 seconds.
func WithRetryMaxDelay(maxDelay time.Duration) Option {
	return func(c *config) {
		c.retry.MaxBackoff = maxDelay
		c.legacyRetry.MaxDelay = &maxDelay
	}
}

// WithRateLimit enables a client-side token bucket: at most rps requests per second on average with bursts
// of up to burst requests. Every attempt (including retries and stream opens) takes a token and waits for
// one, honouring the call's context. rps <= 0 disables the limiter (default).
func WithRateLimit(rps float64, burst int) Option {
	return func(c *config) {
		c.rateLimit = rate.Limit(rps)
		c.rateBurst = max(burst, 1)
	}
}

// WithLogger sets the structured logger. Calls are logged at Debug, retries at Info and final failures at
// Warn. Auth metadata is never logged. Default: no logging.
func WithLogger(l *slog.Logger) Option {
	return func(c *config) { c.logger = l }
}

// WithLogPayloads additionally logs request and response messages at Debug level (truncated to 10 kB).
// Payloads can contain secrets such as cloud credentials or webhook tokens; do not enable in production.
// Without [WithLogger] it logs to slog.Default().
func WithLogPayloads() Option {
	return func(c *config) { c.logPayloads = true }
}

// WithRequestIDHeaders sets the response header and trailer keys searched for a request id, which is then
// reported in [Error] and log records. Default [DefaultRequestIDHeaders].
func WithRequestIDHeaders(keys ...string) Option {
	return func(c *config) { c.requestIDHeaders = keys }
}

// WithDialOptions appends raw gRPC dial options, e.g. grpc.WithContextDialer for an in-memory listener or
// grpc.WithTransportCredentials for a custom CA. The SDK's interceptors still run first.
func WithDialOptions(opts ...grpc.DialOption) Option {
	return func(c *config) { c.dialOptions = append(c.dialOptions, opts...) }
}

// WithEagerConnect makes [New] connect immediately and wait until the connection is ready or ctx is done.
// By default the connection is established lazily on the first call.
func WithEagerConnect() Option {
	return func(c *config) { c.eagerConnect = true }
}
