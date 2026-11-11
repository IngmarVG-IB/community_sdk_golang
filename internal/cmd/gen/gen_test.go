package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNaming(t *testing.T) {
	assert.Equal(t, "CloudExport", camel("cloud_export"))
	assert.Equal(t, "AlertingPublic", camel("alerting/public"))
	assert.Equal(t, "AIAdvisor", camel("ai_advisor"))
	assert.Equal(t, "BGPMonitoring", camel("bgp_monitoring"))
	assert.Equal(t, "V202309", versionIdent("v202309"))
	assert.Equal(t, "V202504Beta2", versionIdent("v202504beta2"))
	assert.Equal(t, "V20261005Alpha1", versionIdent("v20261005alpha1"))

	assert.Equal(t, "Admin", fieldName("synthetics", "SyntheticsAdminService"))
	assert.Equal(t, "Admin", fieldName("bgp_monitoring", "BgpMonitoringAdminService"))
	assert.Equal(t, "Alert", fieldName("alerting/public", "AlertService"))
	assert.Equal(t, "Agent", fieldName("kagent", "AgentService"))
	assert.Equal(t, "Synthetics", fieldName("synthetics", "SyntheticsService"), "never strips to empty")

	v := &Version{API: &API{Path: "alerting/public"}, Name: "v202505"}
	assert.Equal(t, "alertingpublicv202505", importAlias(v))
	assert.Equal(t, "AlertingPublicV202505", accessorName(v))
	assert.Equal(t, "AlertingPublicV202505Clients", groupTypeName(v))
}

func TestVersionOrder(t *testing.T) {
	vs := []string{"v202504beta2", "v20261005alpha1", "v202309", "v202308beta1", "v202101beta1", "v202308"}
	slices.SortFunc(vs, func(a, b string) int {
		if versionLess(a, b) {
			return -1
		}
		return 1
	})
	assert.Equal(t, []string{"v202101beta1", "v202308beta1", "v202308", "v202309", "v202504beta2", "v20261005alpha1"}, vs)
	assert.Equal(t, "stable", stabilityOf("v1"))
	assert.Equal(t, "beta", stabilityOf("v202410beta1"))
}

func svc(names ...string) []*Service {
	out := make([]*Service, 0, len(names))
	for _, n := range names {
		out = append(out, &Service{Name: n})
	}
	return out
}

func TestClassify(t *testing.T) {
	alerting := &API{Path: "alerting/public"}
	alerting.Versions = []*Version{
		{API: alerting, Name: "v202505", Stability: "stable", Services: svc("AlertService", "SuppressionService")},
		{API: alerting, Name: "v202611", Stability: "stable", Services: svc("SuppressionService")},
	}
	synth := &API{Path: "synthetics"}
	synth.Versions = []*Version{
		{API: synth, Name: "v202202", Stability: "stable", Services: svc("SyntheticsAdminService")},
		{API: synth, Name: "v202309", Stability: "stable", Services: svc("SyntheticsAdminService")},
		{API: synth, Name: "v20261005alpha1", Stability: "alpha", Services: svc("SyntheticsBulkActionService")},
	}
	bgp := &API{Path: "bgp_monitoring"}
	bgp.Versions = []*Version{
		{API: bgp, Name: "v202205beta1", Stability: "beta", Services: svc("BgpMonitoringAdminService")},
		{API: bgp, Name: "v202210", Stability: "stable", Services: svc("BgpMonitoringAdminService")},
	}
	device := &API{Path: "device"}
	device.Versions = []*Version{
		{API: device, Name: "v202308beta1", Stability: "beta", Services: svc("DeviceService")},
		{API: device, Name: "v202504beta2", Stability: "beta", Services: svc("DeviceService")},
	}
	classify(&Schema{APIs: []*API{alerting, synth, bgp, device}})

	assert.True(t, alerting.Versions[0].IsAlias, "partial v202611 must not take the alias")
	assert.False(t, alerting.Versions[1].IsAlias)
	assert.Equal(t, exposureStable, alerting.Versions[1].Exposure)

	assert.True(t, synth.Versions[1].IsAlias)
	assert.Equal(t, exposureStable, synth.Versions[0].Exposure, "older stable versions keep accessors")
	assert.Equal(t, exposureExperimental, synth.Versions[2].Exposure, "alpha adds a new service")

	assert.Equal(t, exposureNone, bgp.Versions[0].Exposure, "beta superseded by stable")
	assert.True(t, bgp.Versions[0].Superseded)

	assert.Equal(t, exposureNone, device.Versions[0].Exposure)
	assert.Equal(t, exposureExperimental, device.Versions[1].Exposure, "newest pre-release when no stable exists")
}

func TestFirstSentence(t *testing.T) {
	assert.Equal(t, "Manages tests.", firstSentence(" Manages tests. And more.\n\nDetails"))
	assert.Empty(t, firstSentence("{{.Name}}"))
}

// TestRealSchema runs the generator against the pinned api-schema-public module (from the module cache,
// offline) and checks facts the SDK relies on, plus that the committed outputs are up to date.
func TestRealSchema(t *testing.T) {
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	dir, ref, err := moduleDir(t.Context(), root)
	require.NoError(t, err)
	s, err := LoadSchema(t.Context(), dir, ref)
	require.NoError(t, err)
	require.NoError(t, checkNames(s))

	byDir := map[string]*Version{}
	methods := map[string]*Method{}
	for _, v := range allVersions(s) {
		byDir[v.Dir] = v
		for _, sv := range v.Services {
			for _, m := range sv.Methods {
				methods[m.FullMethod] = m
			}
		}
	}

	syn := byDir["synthetics/v202309"]
	require.NotNil(t, syn)
	assert.True(t, syn.IsAlias)
	assert.Len(t, syn.Services, 2)
	lt := methods["/kentik.synthetics.v202309.SyntheticsAdminService/ListTests"]
	require.NotNil(t, lt)
	assert.Equal(t, "GET", lt.HTTPMethod)
	assert.True(t, lt.Idempotent)
	assert.False(t, methods["/kentik.synthetics.v202309.SyntheticsAdminService/CreateTest"].Idempotent)
	assert.True(t, methods["/kentik.synthetics.v202309.SyntheticsDataService/GetResultsForTests"].Idempotent,
		"read-only POST by name")

	assert.True(t, byDir["alerting/public/v202505"].IsAlias)
	assert.Equal(t, pagingConfig, methods["/kentik.alerting.public.v202505.AlertService/List"].Pagination)
	assert.Equal(t, pagingOffset, methods["/kentik.audit.public.v202601.AuditService/ListAuditEvents"].Pagination)
	assert.Equal(t, "github.com/kentik/api-schema-public/gen/go/kentik/audit/public/v202601",
		byDir["audit/public/v202601"].GoImport, "Go package follows the proto directory, not go_package")
	assert.True(t, methods["/kentik.kptr.v202602.KptrService/ListResolvedAddresses"].ServerStreaming)
	assert.False(t, methods["/kentik.diagnostic.v202501.DiagnosticService/Chat"].Idempotent, "client streaming")

	assert.Equal(t, exposureExperimental, byDir["device/v202504beta2"].Exposure)
	assert.Equal(t, exposureNone, byDir["cloud_export/v202101beta1"].Exposure)
	assert.Equal(t, exposureStable, byDir["cloud_export/v202210"].Exposure)
	assert.True(t, byDir["cloud_export/v202506"].IsAlias)

	// Committed generated files must match.
	for path, render := range map[string]func() ([]byte, error){
		"kentikapi/zz_registry.go":               func() ([]byte, error) { return RenderRegistry(s) },
		"kentikapi/zz_accessors.go":              func() ([]byte, error) { return RenderAccessors(s, "kentikapi") },
		"kentikapi/experimental/zz_accessors.go": func() ([]byte, error) { return RenderAccessors(s, "experimental") },
		"kentik-api.json":                        func() ([]byte, error) { return RenderManifest(s) },
	} {
		want, err := render()
		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(root, path))
		require.NoError(t, err)
		assert.True(t, bytes.Equal(want, got), "%s is stale; run make generate", path)
	}
}
