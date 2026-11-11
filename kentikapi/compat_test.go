package kentikapi_test

import (
	"testing"
	"time"

	synthetics "github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v202309"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kentik/community_sdk_golang/kentikapi"
	"github.com/kentik/community_sdk_golang/kentikapitest"
)

// TestLegacyConfigCompiles keeps the v0.2.x call shapes used by the Terraform providers compiling.
func TestLegacyConfigCompiles(t *testing.T) {
	attempts, delay, timeout := uint(2), time.Millisecond, 5*time.Second
	cfg := kentikapi.Config{
		APIURL:            kentikapi.APIURLEU + "/api/v5",
		CloudExportAPIURL: "https://cloudexports.api.kentik.com", // ignored
		SyntheticsAPIURL:  "https://synthetics.api.kentik.com",   // ignored
		AuthEmail:         "a@example.com",
		AuthToken:         "token",
		RetryCfg:          kentikapi.RetryConfig{MaxAttempts: &attempts, MinDelay: &delay},
		LogPayloads:       false,
		Timeout:           &timeout,
	}
	cfg.FillDefaults()
	c, err := kentikapi.NewClient(cfg)
	require.NoError(t, err)
	require.NoError(t, c.Close())
}

func TestLegacyConfigAsOption(t *testing.T) {
	srv := kentikapitest.NewServer(t, kentikapitest.WithAuth("legacy@example.com", "legacy-token"),
		kentikapitest.HandleUnary(synthetics.SyntheticsAdminService_GetTest_FullMethodName, okGetTestUnary))
	c := srv.Client(t, kentikapi.Config{AuthEmail: "legacy@example.com", AuthToken: "legacy-token"})

	_, err := c.Synthetics().Admin.GetTest(t.Context(), &synthetics.GetTestRequest{Id: "1"})
	assert.NoError(t, err)
}
