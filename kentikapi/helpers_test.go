package kentikapi_test

import (
	"context"
	"net"
	"sync"
	"testing"

	synthetics "github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v202309"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"github.com/kentik/community_sdk_golang/kentikapi"
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

// startBufconn serves register on an in-memory listener and returns a Client connected to it.
func startBufconn(t *testing.T, register func(*grpc.Server), opts ...kentikapi.Option) *kentikapi.Client {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	register(srv)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	base := []kentikapi.Option{
		kentikapi.WithEndpoint("passthrough:///bufnet"),
		kentikapi.WithInsecure(),
		kentikapi.WithCredentials("user@example.com", "token-123"),
		kentikapi.WithRetryPolicy(fastRetries()),
		kentikapi.WithDialOptions(grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		})),
	}
	c, err := kentikapi.New(t.Context(), append(base, opts...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func fastRetries() kentikapi.RetryPolicy {
	p := kentikapi.DefaultRetryPolicy()
	p.InitialBackoff = 1e3 // 1µs
	p.MaxBackoff = 1e4
	return p
}
