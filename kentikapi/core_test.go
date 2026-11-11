package kentikapi_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	synthetics "github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v202309"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/kentik/community_sdk_golang/kentikapi"
)

func newSynth(t *testing.T, f *fakeSynthetics, opts ...kentikapi.Option) synthetics.SyntheticsAdminServiceClient {
	t.Helper()
	c := startBufconn(t, func(s *grpc.Server) { synthetics.RegisterSyntheticsAdminServiceServer(s, f) }, opts...)
	return synthetics.NewSyntheticsAdminServiceClient(c.Conn())
}

func okGetTest(context.Context, int, *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
	return &synthetics.GetTestResponse{Test: &synthetics.Test{Id: "42", Name: "ping"}}, nil
}

func TestAuthMetadata(t *testing.T) {
	f := &fakeSynthetics{getTest: okGetTest}
	svc := newSynth(t, f, kentikapi.WithUserAgent("terraform-provider-kentik/1.0"))

	resp, err := svc.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "42"})
	require.NoError(t, err)
	assert.Equal(t, "ping", resp.GetTest().GetName())

	md := f.calls[0]
	assert.Equal(t, []string{"user@example.com"}, md.Get(kentikapi.MetadataAuthEmail))
	assert.Equal(t, []string{"token-123"}, md.Get(kentikapi.MetadataAuthToken))
	ua := strings.Join(md.Get("user-agent"), " ")
	assert.True(t, strings.HasPrefix(ua, "terraform-provider-kentik/1.0 kentik-community-sdk-golang/"), ua)
}

func TestMissingCredentialsIsUnauthenticatedAndNotRetried(t *testing.T) {
	f := &fakeSynthetics{getTest: okGetTest}
	svc := newSynth(t, f, kentikapi.WithCredentials("", "token-only"))

	_, err := svc.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "42"})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.True(t, kentikapi.IsAuthError(err))
	assert.Equal(t, 0, f.count(), "request must not reach the server")
}

func TestPerRPCCredentialsOverride(t *testing.T) {
	f := &fakeSynthetics{getTest: okGetTest}
	svc := newSynth(t, f, kentikapi.WithPerRPCCredentials(staticCreds{"x-custom": "yes"}))

	_, err := svc.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "42"})
	require.NoError(t, err)
	assert.Equal(t, []string{"yes"}, f.calls[0].Get("x-custom"))
	assert.Empty(t, f.calls[0].Get(kentikapi.MetadataAuthEmail))
}

type staticCreds map[string]string

func (s staticCreds) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return s, nil
}

func (staticCreds) RequireTransportSecurity() bool { return false }

func TestRetries(t *testing.T) {
	unavailable := status.Error(codes.Unavailable, "try later")
	tests := []struct {
		name      string
		opts      []kentikapi.Option
		failures  int
		failWith  error
		wantCalls int
		wantCode  codes.Code
	}{
		{name: "retries Unavailable until success", failures: 2, failWith: unavailable, wantCalls: 3, wantCode: codes.OK},
		{name: "gives up after MaxRetries", failures: 99, failWith: unavailable, wantCalls: 5, wantCode: codes.Unavailable},
		{
			name: "retries ResourceExhausted", failures: 1, wantCalls: 2, wantCode: codes.OK,
			failWith: status.Error(codes.ResourceExhausted, "slow down"),
		},
		{
			name: "does not retry NotFound", failures: 99, wantCalls: 1, wantCode: codes.NotFound,
			failWith: status.Error(codes.NotFound, "no such test"),
		},
		{
			name: "WithoutRetries", opts: []kentikapi.Option{kentikapi.WithoutRetries()},
			failures: 99, failWith: unavailable, wantCalls: 1, wantCode: codes.Unavailable,
		},
		{
			name: "legacy WithRetryMaxAttempts", opts: []kentikapi.Option{kentikapi.WithRetryMaxAttempts(1)},
			failures: 99, failWith: unavailable, wantCalls: 2, wantCode: codes.Unavailable,
		},
		{
			name: "custom codes", wantCalls: 2, wantCode: codes.OK, failures: 1,
			failWith: status.Error(codes.Aborted, "conflict"),
			opts: []kentikapi.Option{kentikapi.WithRetryPolicy(kentikapi.RetryPolicy{
				MaxRetries: 1, InitialBackoff: time.Microsecond, Codes: []codes.Code{codes.Aborted},
			})},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeSynthetics{getTest: func(ctx context.Context, n int, req *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
				if n <= tt.failures {
					return nil, tt.failWith
				}
				return okGetTest(ctx, n, req)
			}}
			svc := newSynth(t, f, tt.opts...)

			_, err := svc.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "42"})
			assert.Equal(t, tt.wantCode, status.Code(err))
			assert.Equal(t, tt.wantCalls, f.count())
			if err != nil {
				var apiErr *kentikapi.Error
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, tt.wantCalls, apiErr.Attempts)
			}
		})
	}
}

func TestNonIdempotentNotRetriedByDefault(t *testing.T) {
	create := func(context.Context, int, *synthetics.CreateTestRequest) (*synthetics.CreateTestResponse, error) {
		return nil, status.Error(codes.Unavailable, "down")
	}
	t.Run("default", func(t *testing.T) {
		f := &fakeSynthetics{createTest: create}
		_, err := newSynth(t, f).CreateTest(t.Context(), &synthetics.CreateTestRequest{})
		assert.Equal(t, codes.Unavailable, status.Code(err))
		assert.Equal(t, 1, f.count())
	})
	t.Run("opt in", func(t *testing.T) {
		f := &fakeSynthetics{createTest: create}
		p := fastRetries()
		p.MaxRetries = 2
		p.Retryable = func(string) bool { return true }
		_, err := newSynth(t, f, kentikapi.WithRetryPolicy(p)).CreateTest(t.Context(), &synthetics.CreateTestRequest{})
		assert.Equal(t, codes.Unavailable, status.Code(err))
		assert.Equal(t, 3, f.count())
	})
}

func TestAttemptTimeoutIsRetried(t *testing.T) {
	f := &fakeSynthetics{getTest: func(ctx context.Context, n int, req *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
		if n == 1 {
			<-ctx.Done() // hang until the client gives up on this attempt
			return nil, ctx.Err()
		}
		return okGetTest(ctx, n, req)
	}}
	svc := newSynth(t, f, kentikapi.WithAttemptTimeout(50*time.Millisecond))

	_, err := svc.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "42"})
	require.NoError(t, err)
	assert.Equal(t, 2, f.count())
}

func TestOverallTimeoutStopsRetries(t *testing.T) {
	f := &fakeSynthetics{getTest: func(context.Context, int, *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
		return nil, status.Error(codes.Unavailable, "down")
	}}
	p := kentikapi.DefaultRetryPolicy()
	p.InitialBackoff = time.Hour
	svc := newSynth(t, f, kentikapi.WithRetryPolicy(p), kentikapi.WithTimeout(200*time.Millisecond))

	start := time.Now()
	_, err := svc.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "42"})
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Less(t, time.Since(start), 5*time.Second, "must not sleep past the deadline")
	assert.Equal(t, 1, f.count())
}

func TestServerRetryInfoIsHonoured(t *testing.T) {
	f := &fakeSynthetics{getTest: func(ctx context.Context, n int, req *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
		if n == 1 {
			st, _ := status.New(codes.ResourceExhausted, "quota").WithDetails(
				&errdetails.RetryInfo{RetryDelay: durationpb.New(100 * time.Millisecond)})
			return nil, st.Err()
		}
		return okGetTest(ctx, n, req)
	}}
	svc := newSynth(t, f)

	start := time.Now()
	_, err := svc.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "42"})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond)
}

func TestStructuredError(t *testing.T) {
	f := &fakeSynthetics{getTest: func(ctx context.Context, _ int, _ *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
		_ = grpc.SetHeader(ctx, metadata.Pairs("x-request-id", "req-abc"))
		return nil, status.Error(codes.NotFound, "test 42 not found")
	}}
	svc := newSynth(t, f)

	_, err := svc.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "42"})
	var apiErr *kentikapi.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "kentik.synthetics.v202309.SyntheticsAdminService", apiErr.Service)
	assert.Equal(t, "GetTest", apiErr.Method)
	assert.Equal(t, codes.NotFound, apiErr.Code)
	assert.Equal(t, "test 42 not found", apiErr.Message)
	assert.Equal(t, "req-abc", apiErr.RequestID)
	assert.Equal(t, 1, apiErr.Attempts)
	assert.False(t, apiErr.Temporary())
	assert.Equal(t,
		"kentikapi: synthetics.v202309.SyntheticsAdminService/GetTest: NotFound: test 42 not found (request id req-abc)",
		apiErr.Error())

	// gRPC helpers and the legacy predicates still work.
	assert.Equal(t, codes.NotFound, status.Code(err))
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, "test 42 not found", st.Message())
	assert.True(t, kentikapi.IsNotFoundError(err))
	assert.False(t, kentikapi.IsTemporaryError(err))
}

func TestRequestIDHeadersOption(t *testing.T) {
	f := &fakeSynthetics{getTest: func(ctx context.Context, _ int, _ *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
		_ = grpc.SetTrailer(ctx, metadata.Pairs("x-kentik-trace", "trace-1"))
		return nil, status.Error(codes.Internal, "boom")
	}}
	svc := newSynth(t, f, kentikapi.WithRequestIDHeaders("x-kentik-trace"))

	_, err := svc.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "42"})
	var apiErr *kentikapi.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "trace-1", apiErr.RequestID)
}

func TestContextCancellation(t *testing.T) {
	f := &fakeSynthetics{getTest: func(ctx context.Context, _ int, _ *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	svc := newSynth(t, f)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, err := svc.GetTest(ctx, &synthetics.GetTestRequest{Id: "42"})
	assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
	assert.True(t, kentikapi.IsTimeoutError(err))
	assert.Equal(t, 1, f.count(), "caller deadline must not be retried")
}

func TestRateLimit(t *testing.T) {
	f := &fakeSynthetics{getTest: okGetTest}
	svc := newSynth(t, f, kentikapi.WithRateLimit(20, 1)) // one token per 50ms

	start := time.Now()
	for range 3 {
		_, err := svc.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "42"})
		require.NoError(t, err)
	}
	assert.GreaterOrEqual(t, time.Since(start), 90*time.Millisecond)

	// A deadline shorter than the wait fails fast without reaching the server.
	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	_, err := svc.GetTest(ctx, &synthetics.GetTestRequest{Id: "42"})
	assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
	assert.Equal(t, 3, f.count())
}

func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f := &fakeSynthetics{getTest: func(ctx context.Context, n int, req *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
		if n == 1 {
			return nil, status.Error(codes.Unavailable, "down")
		}
		return okGetTest(ctx, n, req)
	}}
	svc := newSynth(t, f, kentikapi.WithLogger(logger))

	_, err := svc.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "42"})
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "kentikapi retrying")
	assert.Contains(t, out, "rpc.method=/kentik.synthetics.v202309.SyntheticsAdminService/GetTest")
	assert.Contains(t, out, "kentikapi call")
	assert.NotContains(t, out, "token-123", "credentials must never be logged")
}

func TestClose(t *testing.T) {
	f := &fakeSynthetics{getTest: okGetTest}
	c := startBufconn(t, func(s *grpc.Server) { synthetics.RegisterSyntheticsAdminServiceServer(s, f) })
	require.NoError(t, c.Close())
	require.NoError(t, c.Close(), "Close is idempotent")

	_, err := synthetics.NewSyntheticsAdminServiceClient(c.Conn()).GetTest(t.Context(), &synthetics.GetTestRequest{})
	assert.Equal(t, codes.Canceled, status.Code(err))
}

func TestNewValidation(t *testing.T) {
	_, err := kentikapi.New(t.Context(), kentikapi.WithRegion("mars"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown region "mars"`)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = kentikapi.New(ctx)
	assert.True(t, errors.Is(err, context.Canceled))

	c, err := kentikapi.NewClient(kentikapi.WithCredentials("a", "b")) // deprecated path still works
	require.NoError(t, err)
	assert.NotNil(t, c.Users)
	assert.NotNil(t, c.Cloud)
	require.NoError(t, c.Close())
}

func TestEagerConnect(t *testing.T) {
	f := &fakeSynthetics{getTest: okGetTest}
	c := startBufconn(t, func(s *grpc.Server) { synthetics.RegisterSyntheticsAdminServiceServer(s, f) },
		kentikapi.WithEagerConnect())
	require.NotNil(t, c)

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err := kentikapi.New(ctx, kentikapi.WithEndpoint("127.0.0.1:1"), kentikapi.WithInsecure(),
		kentikapi.WithEagerConnect())
	require.Error(t, err)
}
