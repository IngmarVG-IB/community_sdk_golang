# community_sdk_golang

[![Go Reference](https://pkg.go.dev/badge/github.com/kentik/community_sdk_golang.svg)](https://pkg.go.dev/github.com/kentik/community_sdk_golang)

This repository is the Kentik Go SDK for the community. It is distributed as _github.com/kentik/community_sdk_golang_ Go module.

[kentikapi](kentikapi) package contains Go client library for [Kentik APIs](https://kb.kentik.com/v0/Ab09.htm).

## Requirements

- [Go](https://go.dev/doc/install) >= 1.26

## Installation

```bash
go get github.com/kentik/community_sdk_golang/kentikapi
```

## Usage

The SDK builds on the gRPC clients generated in
[api-schema-public](https://github.com/kentik/api-schema-public). `kentikapi.New` returns one `Client`
with authentication, region selection, retries, rate limiting, structured errors and `log/slog` logging;
typed accessors hand you the generated client for each service version.

```go
import (
	synthetics "github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v202309"
	"github.com/kentik/community_sdk_golang/kentikapi"
)

client, err := kentikapi.New(ctx,
	kentikapi.WithCredentials(email, token),   // or kentikapi.GetCredentials(kentikapi.DefaultProfile)
	kentikapi.WithRegion(kentikapi.RegionEU), // default RegionUS
)
if err != nil { return err }
defer client.Close()

tests, err := client.SyntheticsV202309().Admin.ListTests(ctx, &synthetics.ListTestsRequest{})
```

- Every stable service version has an accessor (`client.CloudExportV202506()`, `client.UserV202211()`, ...)
  and an unversioned alias for the newest one (`client.Synthetics()`, `client.CloudExport()`, ...).
  [kentik-api.json](kentik-api.json) lists them all.
- Alpha and beta APIs without a stable version are in
  [`kentikapi/experimental`](kentikapi/experimental).
- `kentikapi.Paginate` iterates paged list RPCs (`for item, err := range ...`).
- [`kentikapitest`](kentikapitest) runs an in-memory fake API server for your tests.
- Package documentation: <https://pkg.go.dev/github.com/kentik/community_sdk_golang/kentikapi>.

Upgrading from v0.2.x: see [docs/UPGRADING.md](docs/UPGRADING.md). The v5 REST resources (`client.Users`,
`client.Devices`, ...) still work but are deprecated.

### Examples

[examples/](examples) has small programs (`synthetics`, `cloudexport`, `alerts`, `experimental`) that call
the **live** Kentik API with your credentials:

```bash
export KTAPI_AUTH_EMAIL=<email> KTAPI_AUTH_TOKEN=<token>
go run ./examples/synthetics -region us
```

CI only compiles them. Offline, runnable examples are the `Example*` functions in the package docs.
Legacy v5 examples are in [examples/legacy](examples/legacy) (build tag `examples`).

### Errors

Failed v6 calls return `*kentikapi.Error` (service, method, gRPC code, message, request id, attempts). It
works with `errors.As`, `status.Code` and the `kentikapi.Is*Error` helpers.

## Contributing

Anybody who wants to contribute to development is welcome to provide pull requests. To work on the SDK, install tools listed in [requirements section](#requirements).

Development commands are in the [Makefile](Makefile) and documented in [AGENTS.md](AGENTS.md):
`make build test lint vuln generate`.

The project uses ADR to record the architectural decisions: [docs/adr/0001-record-architecture-decisions.md](docs/adr/0001-record-architecture-decisions.md)

### Release

Release process for the SDK is based on Git repository tags that follow [semantic versioning](https://semver.org/).

To release the SDK:
1. Make sure that all code that you want to release is in _master_ branch.
1. Navigate to [repository releases page](https://github.com/kentik/community_sdk_golang/releases), click _Draft a new release_ button and put tag version (in _v\[0-9].\[0-9].\[0-9]_ format), name and description.
