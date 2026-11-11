package kentikapi_test

import (
	"context"
	"sync"
	"testing"
	"time"

	synthetics "github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v202309"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/kentik/community_sdk_golang/kentikapi"
	"github.com/kentik/community_sdk_golang/kentikapitest"
)

// fakeSynthetics is a SyntheticsAdminService whose behaviour is set per test. It records the incoming
// metadata of every call.
type fakeSynthetics struct {
	synthetics.UnimplementedSyntheticsAdminServiceServer

	getTest    func(ctx context.Context, n int, req *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error)
	createTest func(ctx context.Context, n int, req *synthetics.CreateTestRequest) (*synthetics.CreateTestResponse, error)

	mu    sync.Mutex
	calls []metadata.MD
}

func (f *fakeSynthetics) record(ctx context.Context) int {
	md, _ := metadata.FromIncomingContext(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, md)
	return len(f.calls)
}

func (f *fakeSynthetics) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeSynthetics) GetTest(ctx context.Context, req *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
	n := f.record(ctx)
	return f.getTest(ctx, n, req)
}

func (f *fakeSynthetics) CreateTest(
	ctx context.Context, req *synthetics.CreateTestRequest,
) (*synthetics.CreateTestResponse, error) {
	n := f.record(ctx)
	return f.createTest(ctx, n, req)
}

// startBufconn serves register on a kentikapitest server and returns a Client connected to it.
func startBufconn(t *testing.T, register func(grpc.ServiceRegistrar), opts ...kentikapi.Option) *kentikapi.Client {
	t.Helper()
	srv := kentikapitest.NewServer(t, kentikapitest.WithService(register))
	return srv.Client(t, append([]kentikapi.Option{
		kentikapi.WithCredentials("user@example.com", "token-123"),
		kentikapi.WithRetryPolicy(fastRetries()),
	}, opts...)...)
}

func fastRetries() kentikapi.RetryPolicy {
	p := kentikapi.DefaultRetryPolicy()
	p.InitialBackoff = time.Microsecond
	p.MaxBackoff = 10 * time.Microsecond
	return p
}

func okGetTestUnary(ctx context.Context, req *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
	return okGetTest(ctx, 0, req)
}
