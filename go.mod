module github.com/kentik/community_sdk_golang

go 1.26.0

require (
	github.com/AlekSi/pointer v1.2.0
	github.com/antchfx/jsonquery v1.3.8
	github.com/hashicorp/go-retryablehttp v0.7.8
	github.com/kentik/api-schema-public v0.0.0-20261110124345-5d1d2fa374e4
	github.com/stretchr/testify v1.12.1
	golang.org/x/time v0.16.0
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261104224025-d1e00e9f33ab
	google.golang.org/grpc v1.85.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/antchfx/xpath v1.3.9 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.33.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20261104224025-d1e00e9f33ab // indirect
	mvdan.cc/gofumpt v0.12.0 // indirect
)

tool (
	golang.org/x/tools/cmd/stringer
	mvdan.cc/gofumpt
)
