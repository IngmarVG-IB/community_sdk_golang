package kentikapi

import "sync"

// Stability of an API version, derived from its version suffix.
type Stability string

// Stability levels.
const (
	Stable Stability = "stable"
	Beta   Stability = "beta"
	Alpha  Stability = "alpha"
)

// ServiceInfo describes one gRPC service of one API version in api-schema-public.
type ServiceInfo struct {
	// API is the schema directory below proto/kentik, e.g. "synthetics" or "alerting/public".
	API string
	// Version is the API version, e.g. "v202309".
	Version string
	// Stability is stable, beta or alpha.
	Stability Stability
	// FullName is the fully qualified service name, e.g. "kentik.synthetics.v202309.SyntheticsAdminService".
	FullName string
	// GoPackage is the import path of the generated Go code.
	GoPackage string
	// DefaultHost is the google.api.default_host option, if set.
	DefaultHost string
	// Methods lists the RPCs.
	Methods []MethodInfo
}

// MethodInfo describes one RPC.
type MethodInfo struct {
	// Name is the RPC name, e.g. "ListTests".
	Name string
	// FullMethod is the gRPC method path, e.g. "/kentik.synthetics.v202309.SyntheticsAdminService/ListTests".
	FullMethod string
	// HTTPMethod and HTTPPath are the grpc-gateway mapping (google.api.http), if any.
	HTTPMethod, HTTPPath string
	// ClientStreaming and ServerStreaming mark streaming RPCs.
	ClientStreaming, ServerStreaming bool
	// Idempotent is true when the RPC is safe to retry (GET mapping or idempotency_level set).
	Idempotent bool
}

// Services returns metadata for every service in the pinned api-schema-public version, stable and
// experimental, in a stable order. The slice must not be modified.
func Services() []ServiceInfo { return registry }

var methodIndex = sync.OnceValue(func() map[string]*MethodInfo {
	idx := make(map[string]*MethodInfo)
	for i := range registry {
		for j := range registry[i].Methods {
			m := &registry[i].Methods[j]
			idx[m.FullMethod] = m
		}
	}
	return idx
})

// LookupMethod returns metadata for a gRPC method path such as
// "/kentik.synthetics.v202309.SyntheticsAdminService/ListTests".
func LookupMethod(fullMethod string) (MethodInfo, bool) {
	m, ok := methodIndex()[fullMethod]
	if !ok {
		return MethodInfo{}, false
	}
	return *m, true
}
