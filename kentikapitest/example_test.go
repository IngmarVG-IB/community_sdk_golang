package kentikapitest_test

import (
	"context"
	"fmt"
	"log"

	synthetics "github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v202309"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kentik/community_sdk_golang/kentikapitest"
)

// In a test, use kentikapitest.NewServer(t, ...) and srv.Client(t) instead; they clean up automatically.
func Example() {
	srv, err := kentikapitest.Start(
		kentikapitest.WithAuth(),
		kentikapitest.HandleUnary(synthetics.SyntheticsAdminService_GetTest_FullMethodName,
			func(_ context.Context, req *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
				return &synthetics.GetTestResponse{Test: &synthetics.Test{Id: req.GetId(), Name: "dns check"}}, nil
			}),
		// Fail the first call to exercise the client's retries.
		kentikapitest.WithFaults(synthetics.SyntheticsAdminService_GetTest_FullMethodName,
			status.Error(codes.Unavailable, "maintenance")),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer srv.Close()

	client, err := srv.NewClient(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := client.Synthetics().Admin.GetTest(context.Background(), &synthetics.GetTestRequest{Id: "3"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.GetTest().GetName())
	fmt.Println("calls:", len(srv.Calls()))
	// Output:
	// dns check
	// calls: 2
}
