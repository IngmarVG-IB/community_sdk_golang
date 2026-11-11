package kentikapi

import "fmt"

// Region selects a Kentik deployment. The zero value is RegionUS.
type Region string

// Kentik regions.
const (
	// RegionUS is the US cluster: gRPC at grpc.api.kentik.com:443, v5 REST at https://api.kentik.com.
	RegionUS Region = "us"
	// RegionEU is the EU cluster: gRPC at grpc.api.kentik.eu:443, v5 REST at https://api.kentik.eu.
	RegionEU Region = "eu"
)

// Kentik v6 gRPC endpoints.
const (
	EndpointUS = "grpc.api.kentik.com:443"
	EndpointEU = "grpc.api.kentik.eu:443"
)

// Kentik v5 REST API base URLs.
//
// Deprecated: the v5 REST API is legacy. Use [RegionUS] or [RegionEU] with [WithRegion].
const (
	APIURLUS = "https://api.kentik.com"
	APIURLEU = "https://api.kentik.eu"
)

type regionInfo struct {
	grpcEndpoint string
	apiURL       string
}

func (r Region) info() (regionInfo, error) {
	switch r {
	case RegionUS, "":
		return regionInfo{grpcEndpoint: EndpointUS, apiURL: APIURLUS}, nil
	case RegionEU:
		return regionInfo{grpcEndpoint: EndpointEU, apiURL: APIURLEU}, nil
	default:
		return regionInfo{}, fmt.Errorf("kentikapi: unknown region %q (want %q or %q)", string(r), RegionUS, RegionEU)
	}
}
