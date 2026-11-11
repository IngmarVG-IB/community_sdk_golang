# Upgrading

This guide covers moving from the last release, **v0.2.0** (2021-05), and from the untagged commits that
downstream projects pinned (`master` at `62e10d7`, 2021-11, with openapi-generated v202101beta1 clients;
`dev_grpc` at `5f9f1d7`, 2022-04, with gRPC v202202 synthetics) to the modernised SDK on the
`modernize/2026-11` branch. The recommended tag for this line is **v0.3.0** (see the end).

The module path does not change: `github.com/kentik/community_sdk_golang`.

## At a glance

| area | before | now |
|---|---|---|
| Go | 1.15 (v0.2.0) / 1.17 (dev_grpc) | **1.26** minimum, tested on 1.26 and 1.27 |
| client | `kentikapi.NewClient(kentikapi.Config{...})` | `kentikapi.New(ctx, opts...)` (`NewClient` and `Config` still compile, deprecated) |
| v6 transport | openapi REST clients (master), gRPC with hand-written models (dev_grpc) | gRPC with the generated api-schema-public clients |
| synthetics | v202101beta1 (master), v202202 (dev_grpc) | **v202309** (`client.SyntheticsV202309()`, alias `client.Synthetics()`) |
| cloud export | v202101beta1 | **v202506** (`client.CloudExportV202506()`, alias `client.CloudExport()`); v202210 also available |
| other v6 APIs | none | every stable version in api-schema-public; alpha/beta in `kentikapi/experimental` |
| errors | `kentikapi.Is*Error(err)` | same helpers, plus `*kentikapi.Error` (code, method, request id, attempts) |
| test fake | `kentikapi/fakeapiserver` (openapi HTTP) | `kentikapitest` (in-memory gRPC, any service) |
| v5 REST | `client.Users`, `client.Devices`, ... | unchanged, **deprecated** and frozen |

## 1. Bump Go and the module

```bash
go mod edit -go=1.26
go get github.com/kentik/community_sdk_golang@<version>   # v0.3.0 once tagged
go mod tidy
```

## 2. Create the client

```go
// before (v0.2.x)
client := kentikapi.NewClient(kentikapi.Config{
	APIURL:    kentikapi.APIURLEU, // "https://api.kentik.eu/api/v5" in v0.2
	AuthEmail: email,
	AuthToken: token,
})

// after
client, err := kentikapi.New(ctx,
	kentikapi.WithCredentials(email, token),
	kentikapi.WithRegion(kentikapi.RegionEU),
)
if err != nil {
	return err
}
defer client.Close()
```

`Config` still exists and implements `Option`, so `kentikapi.NewClient(cfg)` and `kentikapi.New(ctx, cfg)`
compile. The only source change for v0.2.x callers is that `NewClient` now also returns an error:
`client, err := kentikapi.NewClient(cfg)`. `Config.APIURL` accepts the old `.../api/v5` form;
`CloudExportAPIURL` and `SyntheticsAPIURL` are ignored.

| v0.2.x / dev_grpc | now |
|---|---|
| `Config.APIURL`, `WithAPIURL(url)` | `WithRegion(RegionUS / RegionEU)` or `WithEndpoint("host:443")`; `WithAPIURL` still works (deprecated) |
| `Config.AuthEmail`, `Config.AuthToken` | `WithCredentials(email, token)` (or `WithPerRPCCredentials`) |
| `Config.RetryCfg`, `WithRetryMaxAttempts/MinDelay/MaxDelay` | `WithRetryPolicy(RetryPolicy{...})`, `WithoutRetries()`; the old options still work |
| `Config.Timeout`, `WithTimeout` | `WithTimeout` (whole call) and `WithAttemptTimeout` (per attempt) |
| `Config.LogPayloads`, `WithLogPayloads` | `WithLogger(*slog.Logger)`; `WithLogPayloads` logs messages through slog |
| `APIURLUS`, `APIURLEU` | now the base URL without `/api/v5`; prefer `RegionUS`, `RegionEU` (`EndpointUS`, `EndpointEU` for gRPC) |
| - | `WithUserAgent`, `WithRateLimit`, `WithRequestIDHeaders`, `WithDialOptions`, `WithEagerConnect` |

Behaviour changes: v6 retries now apply only to read-only RPCs by default (`IsIdempotent`); set
`RetryPolicy.Retryable` to retry mutations as well. Retries honour the server's `RetryInfo`.

## 3. Synthetics: v202101beta1 / v202202 to v202309

The v202101beta1 openapi clients (`client.SyntheticsAdminServiceAPI`, `client.SyntheticsDataServiceAPI`,
package `kentikapi/synthetics`) and the dev_grpc v202202 wrapper (`client.Synthetics` field, models in
`kentikapi/synthetics`) are gone: Kentik removed both versions from api-schema-public. Use the generated
v202309 messages directly:

```go
import synthetics "github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v202309"

// before (master, openapi)
resp, _, err := client.SyntheticsAdminServiceAPI.TestsList(ctx).Execute()

// before (dev_grpc)
tests, err := client.Synthetics.GetAllTests(ctx)

// after
resp, err := client.SyntheticsV202309().Admin.ListTests(ctx, &synthetics.ListTestsRequest{})
for _, t := range resp.GetTests() { ... }

results, err := client.SyntheticsV202309().Data.GetResultsForTests(ctx, &synthetics.GetResultsForTestsRequest{...})
```

Field and message names follow the v202309 proto (`kentik/synthetics/v202309/synthetics.proto`); the
changes between v202202 and v202309 are Kentik's, see their API docs. Protobuf messages are compared with
`proto.Equal`, serialised with `protojson`, and read through `Get...()` accessors that are nil-safe.

## 4. Cloud export: v202101beta1 to v202506

```go
import cloudexport "github.com/kentik/api-schema-public/gen/go/kentik/cloud_export/v202506"

// before (master, openapi)
resp, _, err := client.CloudExportAdminServiceAPI.ExportList(ctx).Execute()
// before (dev_grpc)
resp, err := client.Cloud.GetAllExports(ctx)

// after
resp, err := client.CloudExportV202506().ListCloudExports(ctx, &cloudexport.ListCloudExportsRequest{})
```

`client.Cloud` (v202101beta1, hand-written `kentikapi/cloud` models) still compiles and works while Kentik
serves that version, but it is deprecated. The openapi package `kentikapi/cloudexport` is gone.

## 5. Errors

`kentikapi.IsAuthError`, `IsNotFoundError`, `IsInvalidRequestError`, `IsRateLimitExhaustedError`,
`IsTemporaryError` and `IsTimeoutError` work for v6 and v5 errors. For details:

```go
var apiErr *kentikapi.Error
if errors.As(err, &apiErr) {
	log.Printf("%s/%s: %s (request id %s, %d attempts)", apiErr.Service, apiErr.Method, apiErr.Code,
		apiErr.RequestID, apiErr.Attempts)
}
status.Code(err) // works too
```

## 6. Tests: fakeapiserver to kentikapitest

`kentikapi/fakeapiserver` (openapi HTTP fake for v202101beta1) is removed. `kentikapitest` serves any
api-schema-public service in memory:

```go
srv := kentikapitest.NewServer(t,
	kentikapitest.HandleUnary(cloudexport.CloudExportAdminService_ListCloudExports_FullMethodName,
		func(ctx context.Context, req *cloudexport.ListCloudExportsRequest) (*cloudexport.ListCloudExportsResponse, error) {
			return &cloudexport.ListCloudExportsResponse{Exports: fixtures}, nil
		}),
	kentikapitest.WithAuth(), // reject calls without kentikapitest.Email / Token
)
client := srv.Client(t) // or kentikapi.New(ctx, append(srv.ClientOptions(), yourOpts...)...)
```

For full fakes, register a generated server implementation with `kentikapitest.WithService`. Use
`WithFaults` to inject errors and `srv.Calls()` to assert on requests.

## 7. v5 REST resources (deprecated)

`client.Users`, `Devices`, `DeviceLabels`, `Sites`, `Tags`, `SavedFilters`, `CustomDimensions`,
`CustomApplications`, `Query`, `MyKentikPortal`, `Plans` and `Alerting`, and package `kentikapi/models`, keep
working. They are frozen: no new fields or endpoints. Moving to v6:

| v5 | v6 |
|---|---|
| `client.Users` | `client.UserV202211()` |
| `client.Sites` | `client.SiteV202509()` |
| `client.DeviceLabels` | `client.LabelV202210()` |
| `client.MyKentikPortal` | `client.MKPV202407()` |
| `client.Alerting` | `client.AlertingPublicV202505()` (alerts, policies, mitigations; suppressions also in `AlertingPublicV202611()`) |
| `client.Devices` | `experimental.New(client).DeviceV202504Beta2()` (beta) |
| `client.CustomDimensions`, `CustomApplications`, `SavedFilters`, `Plans`, `Tags` | `experimental` alpha versions (`CustomDimensionV202411Alpha1`, `CustomApplicationV202501Alpha1`, `SavedFilterV202501Alpha1`, `PlanV202501Alpha1`, `FlowTagV202404Alpha1`) |
| `client.Query` | no v6 equivalent yet; keep using v5 |

**If you come straight from v0.2.0** (not from the dev_grpc commits): the unreleased dev_grpc line,
which this branch includes, already changed the v5 models: IDs are `string` instead of `int`
(`models.ID`), constructors take `...RequiredFields` structs, and fields and enum values the API never
returned were removed. The full list (113 items, generated with `apidiff`) is in
[upgrading/models-v0.2.0.txt](upgrading/models-v0.2.0.txt).

## 8. Paging

```go
for alert, err := range kentikapi.Paginate(ctx, client.AlertingPublicV202505().Alert.List,
	&alerting.AlertServiceListRequest{}, (*alerting.AlertServiceListResponse).GetAlerts) {
	if err != nil { return err }
	...
}
```

## 9. Unversioned aliases

`client.Synthetics()`, `client.CloudExport()` and the other aliases follow the newest complete stable
version. When Kentik publishes a new one and the SDK picks it up, the alias's return type changes. Libraries,
including the Terraform providers, should call the versioned accessors (`SyntheticsV202309()`).

## Version recommendation: v0.3.0, not v1.0.0

Recommend tagging this line **v0.3.0** (not tagged yet):

- **The public API embeds an unversioned dependency.** Accessors return types from
  `github.com/kentik/api-schema-public`, which only has `v0.0.0-...` pseudo-versions and has deleted
  versions before (synthetics v202101beta1 and v202202). A v1 promise would cover types the SDK does not
  control.
- **Aliases change type by design** when a schema bump moves them; under v1 every such bump would be a
  major release.
- **The surface is new.** `New`, the options, `Error`, `Paginate` and `kentikapitest` have had no external
  users yet. One or two minor releases of real use (the Terraform providers, ktranslate) should shake out
  naming before freezing it.
- **v0.x matches what changed.** v0.2.0 to here is breaking (Go 1.26, `NewClient` returns an error, the
  v6 packages were replaced, the dev_grpc model changes), and Go semver allows that in a v0 minor. A v1.0.0
  would suggest stability we cannot offer yet.

Criteria for v1.0.0: api-schema-public publishes tagged releases (or Kentik commits to not deleting stable
versions), the alias policy is settled (keep, or drop aliases in v1), the deprecated v5 layer is removed
or split into its own module, and v0.3.x has run in production with the Terraform providers.
