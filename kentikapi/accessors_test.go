package kentikapi_test

import (
	"testing"

	synthetics "github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v202309"
	bulk "github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v20261005alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kentik/community_sdk_golang/kentikapi"
	"github.com/kentik/community_sdk_golang/kentikapi/experimental"
)

func TestGeneratedAccessors(t *testing.T) {
	f := &fakeSynthetics{getTest: okGetTest}
	c := startBufconn(t, func(s *grpc.Server) {
		synthetics.RegisterSyntheticsAdminServiceServer(s, f)
		bulk.RegisterSyntheticsBulkActionServiceServer(s, bulk.UnimplementedSyntheticsBulkActionServiceServer{})
	})

	resp, err := c.Synthetics().Admin.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "42"})
	require.NoError(t, err)
	assert.Equal(t, "42", resp.GetTest().GetId())

	_, err = c.SyntheticsV202309().Admin.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "42"})
	require.NoError(t, err)
	assert.Equal(t, 2, f.count())

	// Experimental accessors share the connection (and its auth and error handling).
	_, err = experimental.New(c).SyntheticsV20261005Alpha1().BulkUpdateTests(t.Context(), &bulk.BulkUpdateTestsRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	var apiErr *kentikapi.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "kentik.synthetics.v20261005alpha1.SyntheticsBulkActionService", apiErr.Service)
}

func TestRegistry(t *testing.T) {
	m, ok := kentikapi.LookupMethod("/kentik.synthetics.v202309.SyntheticsAdminService/ListTests")
	require.True(t, ok)
	assert.Equal(t, "GET", m.HTTPMethod)
	assert.Equal(t, "/synthetics/v202309/tests", m.HTTPPath)
	assert.True(t, m.Idempotent)
	assert.True(t, kentikapi.IsIdempotent(m.FullMethod))
	assert.False(t, kentikapi.IsIdempotent("/kentik.synthetics.v202309.SyntheticsAdminService/DeleteTest"))

	var stable, experimentalCount int
	for _, s := range kentikapi.Services() {
		assert.NotEmpty(t, s.Methods, s.FullName)
		switch {
		case s.Stability == kentikapi.Stable:
			stable++
			assert.NotEmpty(t, s.Accessor, "every stable service has an accessor: %s", s.FullName)
		case s.Accessor != "":
			experimentalCount++
			assert.Contains(t, s.Accessor, "experimental.")
		}
	}
	assert.Positive(t, stable)
	assert.Positive(t, experimentalCount)
}
