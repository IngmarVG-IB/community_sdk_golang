package kentikapi_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	audit "github.com/kentik/api-schema-public/gen/go/kentik/audit/public/v202601"
	synthetics "github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v202309"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kentik/community_sdk_golang/kentikapi"
	"github.com/kentik/community_sdk_golang/kentikapitest"
)

// The examples below run against an in-memory kentikapitest server so they execute offline; in real code
// only the kentikapi.New call differs.

func ExampleNew() {
	ctx := context.Background()
	client, err := kentikapi.New(ctx,
		kentikapi.WithCredentials(os.Getenv("KTAPI_AUTH_EMAIL"), os.Getenv("KTAPI_AUTH_TOKEN")),
		kentikapi.WithRegion(kentikapi.RegionEU),
		kentikapi.WithUserAgent("my-tool/1.2"),
		kentikapi.WithTimeout(30*time.Second),
		kentikapi.WithRateLimit(5, 10),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// No network I/O has happened yet: the connection is established on the first call.
	fmt.Println(client.Conn() != nil)
	// Output: true
}

func ExampleClient_SyntheticsV202309() {
	srv, err := kentikapitest.Start(kentikapitest.HandleUnary(synthetics.SyntheticsAdminService_ListTests_FullMethodName,
		func(context.Context, *synthetics.ListTestsRequest) (*synthetics.ListTestsResponse, error) {
			return &synthetics.ListTestsResponse{Tests: []*synthetics.Test{{Id: "1", Name: "ping example.com"}}}, nil
		}))
	if err != nil {
		log.Fatal(err)
	}
	defer srv.Close()
	client, err := srv.NewClient(context.Background()) // in real code: kentikapi.New(ctx, kentikapi.WithCredentials(email, token))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := client.SyntheticsV202309().Admin.ListTests(context.Background(), &synthetics.ListTestsRequest{})
	if err != nil {
		log.Fatal(err)
	}
	for _, test := range resp.GetTests() {
		fmt.Println(test.GetId(), test.GetName())
	}
	// Output: 1 ping example.com
}

func ExampleError() {
	srv, err := kentikapitest.Start(kentikapitest.HandleUnary(synthetics.SyntheticsAdminService_GetTest_FullMethodName,
		func(context.Context, *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
			return nil, status.Error(codes.NotFound, "test 99 not found")
		}))
	if err != nil {
		log.Fatal(err)
	}
	defer srv.Close()
	client, err := srv.NewClient(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	_, err = client.Synthetics().Admin.GetTest(context.Background(), &synthetics.GetTestRequest{Id: "99"})

	var apiErr *kentikapi.Error
	if errors.As(err, &apiErr) {
		fmt.Println(apiErr.Method, apiErr.Code)
	}
	fmt.Println(kentikapi.IsNotFoundError(err), status.Code(err))
	// Output:
	// GetTest NotFound
	// true NotFound
}

func ExamplePaginate() {
	srv, err := kentikapitest.Start(kentikapitest.HandleUnary(audit.AuditService_ListAuditEvents_FullMethodName,
		func(_ context.Context, req *audit.ListAuditEventsRequest) (*audit.ListAuditEventsResponse, error) {
			var events []*audit.AuditEvent
			for i := req.GetOffset(); i < min(req.GetOffset()+req.GetLimit(), 5); i++ {
				events = append(events, &audit.AuditEvent{Id: i, ApiAction: "update"})
			}
			return &audit.ListAuditEventsResponse{Events: events}, nil
		}))
	if err != nil {
		log.Fatal(err)
	}
	defer srv.Close()
	client, err := srv.NewClient(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	var ids []uint64
	for ev, err := range kentikapi.Paginate(ctx, client.AuditPublic().ListAuditEvents,
		&audit.ListAuditEventsRequest{}, (*audit.ListAuditEventsResponse).GetEvents, kentikapi.WithPageSize(2)) {
		if err != nil {
			log.Fatal(err)
		}
		ids = append(ids, ev.GetId())
	}
	fmt.Println(ids)
	fmt.Println("pages fetched:", len(srv.Calls()))
	// Output:
	// [0 1 2 3 4]
	// pages fetched: 3
}

func ExampleWithRetryPolicy() {
	p := kentikapi.DefaultRetryPolicy()
	p.MaxRetries = 2
	p.InitialBackoff = 200 * time.Millisecond
	// Also retry mutating RPCs (only safe if your requests are idempotent server-side).
	p.Retryable = func(string) bool { return true }

	client, err := kentikapi.New(context.Background(), kentikapi.WithRetryPolicy(p))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	fmt.Println(p.MaxRetries)
	// Output: 2
}
