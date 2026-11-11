package kentikapitest_test

import (
	"context"
	"errors"
	"io"
	"testing"

	kptr "github.com/kentik/api-schema-public/gen/go/kentik/kptr/v202602"
	synthetics "github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v202309"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kentik/community_sdk_golang/kentikapi"
	"github.com/kentik/community_sdk_golang/kentikapitest"
)

func getTest(_ context.Context, req *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
	return &synthetics.GetTestResponse{Test: &synthetics.Test{Id: req.GetId(), Name: "t" + req.GetId()}}, nil
}

func TestHandleUnaryAndCalls(t *testing.T) {
	srv := kentikapitest.NewServer(t, kentikapitest.HandleUnary(synthetics.SyntheticsAdminService_GetTest_FullMethodName, getTest))
	c := srv.Client(t)

	resp, err := c.Synthetics().Admin.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "7"})
	require.NoError(t, err)
	assert.Equal(t, "t7", resp.GetTest().GetName())

	calls := srv.Calls()
	require.Len(t, calls, 1)
	assert.Equal(t, synthetics.SyntheticsAdminService_GetTest_FullMethodName, calls[0].Method)
	assert.Equal(t, []string{kentikapitest.Email}, calls[0].Metadata.Get(kentikapi.MetadataAuthEmail))
	req, ok := calls[0].Request.(*synthetics.GetTestRequest)
	require.True(t, ok)
	assert.Equal(t, "7", req.GetId())

	_, err = c.Synthetics().Admin.DeleteTest(t.Context(), &synthetics.DeleteTestRequest{Id: "7"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

type fakeAdmin struct {
	synthetics.UnimplementedSyntheticsAdminServiceServer
}

func (fakeAdmin) ListTests(context.Context, *synthetics.ListTestsRequest) (*synthetics.ListTestsResponse, error) {
	return &synthetics.ListTestsResponse{Tests: []*synthetics.Test{{Id: "1"}}}, nil
}

func TestWithServiceAndAuth(t *testing.T) {
	srv := kentikapitest.NewServer(t,
		kentikapitest.WithService(func(r grpc.ServiceRegistrar) {
			synthetics.RegisterSyntheticsAdminServiceServer(r, fakeAdmin{})
		}),
		kentikapitest.WithAuth(),
	)

	resp, err := srv.Client(t).Synthetics().Admin.ListTests(t.Context(), &synthetics.ListTestsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetTests(), 1)

	bad := srv.Client(t, kentikapi.WithCredentials("intruder@example.com", "nope"))
	_, err = bad.Synthetics().Admin.ListTests(t.Context(), &synthetics.ListTestsRequest{})
	assert.True(t, kentikapi.IsAuthError(err))
}

func TestWithFaultsDrivesClientRetries(t *testing.T) {
	m := synthetics.SyntheticsAdminService_GetTest_FullMethodName
	srv := kentikapitest.NewServer(t,
		kentikapitest.HandleUnary(m, getTest),
		kentikapitest.WithFaults(m, status.Error(codes.Unavailable, "down"), status.Error(codes.ResourceExhausted, "slow")),
	)

	_, err := srv.Client(t).Synthetics().Admin.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "1"})
	require.NoError(t, err)
	assert.Len(t, srv.CallsTo(m), 3, "two injected failures, then success")
}

type fakeKptr struct {
	kptr.UnimplementedKptrServiceServer
}

func (fakeKptr) ListResolvedAddresses(
	_ *kptr.ListResolvedAddressesRequest, stream grpc.ServerStreamingServer[kptr.ListResolvedAddressesResponse],
) error {
	for range 2 {
		if err := stream.Send(&kptr.ListResolvedAddressesResponse{}); err != nil {
			return err
		}
	}
	return status.Error(codes.DataLoss, "stream broke")
}

func TestStreamingErrorsAreWrapped(t *testing.T) {
	srv := kentikapitest.NewServer(t, kentikapitest.WithService(func(r grpc.ServiceRegistrar) {
		kptr.RegisterKptrServiceServer(r, fakeKptr{})
	}))
	stream, err := srv.Client(t).KPTR().ListResolvedAddresses(t.Context(), &kptr.ListResolvedAddressesRequest{})
	require.NoError(t, err)

	n := 0
	for {
		_, err = stream.Recv()
		if err != nil {
			break
		}
		n++
	}
	assert.Equal(t, 2, n)
	assert.False(t, errors.Is(err, io.EOF))
	var apiErr *kentikapi.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, codes.DataLoss, apiErr.Code)
	assert.Equal(t, "ListResolvedAddresses", apiErr.Method)
	assert.Len(t, srv.Calls(), 1)
}
