package kentikapi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestResolveEndpoints(t *testing.T) {
	tests := []struct {
		name          string
		opts          []Option
		wantTarget    string
		wantAPIURL    string
		wantPlaintext bool
	}{
		{name: "default is US", wantTarget: EndpointUS, wantAPIURL: APIURLUS},
		{name: "EU", opts: []Option{WithRegion(RegionEU)}, wantTarget: EndpointEU, wantAPIURL: APIURLEU},
		{
			name: "custom endpoint keeps region REST URL", opts: []Option{WithRegion(RegionEU), WithEndpoint("proxy:8443")},
			wantTarget: "proxy:8443", wantAPIURL: APIURLEU,
		},
		{
			name: "legacy API URL https", opts: []Option{WithAPIURL("https://api.kentik.eu")},
			wantTarget: "grpc.api.kentik.eu:443", wantAPIURL: "https://api.kentik.eu",
		},
		{
			name: "legacy API URL http IP", opts: []Option{WithAPIURL("http://127.0.0.1:8080")},
			wantTarget: "127.0.0.1:8080", wantAPIURL: "http://127.0.0.1:8080", wantPlaintext: true,
		},
		{
			name: "legacy API URL with explicit endpoint", opts: []Option{WithAPIURL("http://localhost"), WithEndpoint("x:1")},
			wantTarget: "x:1", wantAPIURL: "http://localhost",
		},
		{name: "insecure", opts: []Option{WithInsecure()}, wantTarget: EndpointUS, wantAPIURL: APIURLUS, wantPlaintext: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultConfig()
			for _, o := range tt.opts {
				o(&cfg)
			}
			target, apiURL, plaintext, err := resolveEndpoints(cfg)
			require.NoError(t, err)
			assert.Equal(t, tt.wantTarget, target)
			assert.Equal(t, tt.wantAPIURL, apiURL)
			assert.Equal(t, tt.wantPlaintext, plaintext)
		})
	}

	cfg := defaultConfig()
	WithAPIURL("://bad")(&cfg)
	_, _, _, err := resolveEndpoints(cfg)
	assert.Error(t, err)
}

func TestBackoff(t *testing.T) {
	p := RetryPolicy{InitialBackoff: 100 * time.Millisecond, MaxBackoff: time.Second, Multiplier: 2, Jitter: 0.5}.normalized()
	noJitter := func() float64 { return 0 }
	fullJitter := func() float64 { return 1 }

	assert.Equal(t, 100*time.Millisecond, p.backoff(1, noJitter))
	assert.Equal(t, 200*time.Millisecond, p.backoff(2, noJitter))
	assert.Equal(t, 400*time.Millisecond, p.backoff(3, noJitter))
	assert.Equal(t, time.Second, p.backoff(10, noJitter), "capped at MaxBackoff")
	assert.Equal(t, 50*time.Millisecond, p.backoff(1, fullJitter), "jitter takes off up to half")
}

func TestRetryPolicyNormalized(t *testing.T) {
	p := RetryPolicy{MaxRetries: -3, Jitter: 7}.normalized()
	d := DefaultRetryPolicy()
	assert.Equal(t, 0, p.MaxRetries)
	assert.Equal(t, d.InitialBackoff, p.InitialBackoff)
	assert.Equal(t, d.MaxBackoff, p.MaxBackoff)
	assert.InDelta(t, d.Multiplier, p.Multiplier, 0)
	assert.InDelta(t, 1.0, p.Jitter, 0)
	assert.Equal(t, d.Codes, p.Codes)
	assert.NotNil(t, p.Retryable)
	assert.True(t, p.retryOn(codes.DeadlineExceeded))
	assert.False(t, p.retryOn(codes.Internal))
}

func TestServerRetryDelayPushback(t *testing.T) {
	d, ok := serverRetryDelay(status.Error(codes.Unavailable, ""), metadata.Pairs("grpc-retry-pushback-ms", "250"))
	require.True(t, ok)
	assert.Equal(t, 250*time.Millisecond, d)

	_, ok = serverRetryDelay(status.Error(codes.Unavailable, ""), metadata.MD{})
	assert.False(t, ok)
}

func TestIsIdempotentFallback(t *testing.T) {
	assert.True(t, IsIdempotent("/unknown.v1.Svc/GetThing"))
	assert.True(t, IsIdempotent("/unknown.v1.Svc/ListThings"))
	assert.False(t, IsIdempotent("/unknown.v1.Svc/CreateThing"))
	assert.False(t, IsIdempotent("/unknown.v1.Svc/DeleteThing"))
}

func TestSplitMethod(t *testing.T) {
	svc, m := splitMethod("/kentik.synthetics.v202309.SyntheticsAdminService/ListTests")
	assert.Equal(t, "kentik.synthetics.v202309.SyntheticsAdminService", svc)
	assert.Equal(t, "ListTests", m)
}
