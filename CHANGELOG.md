# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Built on the upstream `dev_grpc` line (50 commits ahead of `master`, last change 2022-07-28), which already
replaced the openapi-generator v6 clients with gRPC.

### Added
- `kentikapi.New(ctx, opts...)`: one `Client` over a single gRPC connection (ADR 0004), with
  - email and API token auth as per-RPC credentials (`x-ch-auth-email`, `x-ch-auth-api-token`), refusing
    plaintext unless `WithInsecure`; `WithPerRPCCredentials` for custom auth;
  - `WithRegion(RegionUS|RegionEU)` presets (`grpc.api.kentik.com:443`, `grpc.api.kentik.eu:443`) and
    `WithEndpoint` for a custom target;
  - `WithUserAgent`, `WithTimeout` (whole call), `WithAttemptTimeout` (per attempt);
  - retries with exponential backoff and jitter on Unavailable, ResourceExhausted and DeadlineExceeded, for
    idempotent RPCs only by default, honouring `google.rpc.RetryInfo` and `grpc-retry-pushback-ms`
    (`RetryPolicy`, `WithRetryPolicy`, `WithoutRetries`);
  - optional client-side token-bucket rate limit (`WithRateLimit`);
  - structured `*kentikapi.Error` (service, method, code, message, request id, attempts) that still works with
    `status.FromError` and the `Is*Error` helpers;
  - `log/slog` logging (`WithLogger`), request id header capture (`WithRequestIDHeaders`), `WithDialOptions`,
    `WithEagerConnect`, `Client.Conn()` and an idempotent `Client.Close()`.
- Typed accessors for every stable service version in api-schema-public, generated from the protos by
  `internal/cmd/gen` (`make generate`, ADR 0005): `Client.SyntheticsV202309()`, `Client.CloudExportV202506()`,
  `Client.AlertingPublicV202505()`, ... and unversioned aliases for the newest complete stable version
  (`Client.Synthetics()`, `Client.CloudExport()`, `Client.User()`, ...). 22 APIs, 25 stable versions.
- `kentikapi/experimental`: accessors for 19 alpha/beta versions with no stable equivalent.
- `kentikapi.Services()` / `LookupMethod()` registry (HTTP mapping, streaming, idempotency, pagination);
  retries use it to decide which RPCs are read-only.
- `kentik-api.json` is now generated, and CI fails on generated-code drift.
- `kentikapi.Paginate` (Go 1.23 `iter.Seq2`) and `CollectAll` for paged list RPCs (alerting
  `PaginationConfig`, audit `offset`/`limit`), ADR 0006.
- `kentikapitest`: in-memory bufconn fake server with pluggable services (`WithService`), per-RPC typed
  handlers (`HandleUnary`), call recording, auth checks (`WithAuth`) and fault injection (`WithFaults`).
- Runnable offline `Example*` functions (pkg.go.dev), package docs (`doc.go`), and compile-only example
  programs in `examples/{synthetics,cloudexport,alerts,experimental}`.
- `kentikapi.Version()` and a `kentik-community-sdk-golang/<version>` user agent.

- `docs/UPGRADING.md` (from v0.2.0, master `62e10d7` and dev_grpc `5f9f1d7`) and
  `docs/upgrading/models-v0.2.0.txt` (apidiff of the v5 models).
- `kentikapi.Config` is back (deprecated) and implements `Option`, so v0.2.x code
  `kentikapi.NewClient(kentikapi.Config{...})` compiles again; it now also returns an error.

### Deprecated
- `NewClient` (use `New`), `ClientOption` (use `Option`), `WithAPIURL` (use `WithRegion`/`WithEndpoint`),
  `RetryConfig`, `APIURLUS`/`APIURLEU`, every v5 REST field on `Client`, the `kentikapi/models` package, and
  `Client.Cloud` with the `kentikapi/cloud` package (cloud_export v202101beta1).

### Changed
- `Option` (and its alias `ClientOption`) is now an interface instead of a function type, so `Config`
  can be passed as an option. Code that only calls the `With*` constructors is unaffected.
- Retries of v6 calls apply only to read-only RPCs by default (`IsIdempotent`), and the default codes are
  Unavailable, ResourceExhausted and DeadlineExceeded (dev_grpc retried FailedPrecondition and Aborted on
  every RPC).
- Legacy examples moved to `examples/legacy/`.
- `WithLogPayloads` now logs through `log/slog` (`slog.Default()` unless `WithLogger` is set).
- `WithTimeout` accepts a negative value to disable the overall timeout (0 still keeps the default).
- Minimum Go version is now 1.26 (was 1.17 on `dev_grpc`, 1.15 in v0.2.0). CI tests Go 1.26 and 1.27.
- All dependencies upgraded; `govulncheck ./...` is clean. `github.com/kentik/api-schema-public` pinned to
  `v0.0.0-20261110124345-5d1d2fa374e4`.
- Dropped the `replace` of `github.com/hashicorp/go-retryablehttp` with a personal fork; upstream v0.7.8 is used.
- Development tools (`gofumpt`, `stringer`) are Go 1.24+ `tool` directives instead of `tools/tools.go`.
- golangci-lint configuration migrated to v2.
- Replaced deprecated `io/ioutil` and `reflect.PtrTo` usage.

### Removed
- (since v0.2.x master) the openapi-generated `kentikapi/synthetics`, `kentikapi/cloudexport` and
  `kentikapi/fakeapiserver` packages and the `Client.*ServiceAPI` fields; removed on dev_grpc, see
  docs/UPGRADING.md.
- The synthetics v202202 layer (`kentikapi/synthetics` package and `Client.Synthetics` field). The
  generated code it wrapped (`gen/go/kentik/synthetics/v202202`) no longer exists in api-schema-public.
  Its tests were removed with it.

### Added
- AI-ready baseline: `AGENTS.md`, `CLAUDE.md`, `kentik-api.json` (+ validator), `Makefile`,
  `.github/workflows/ci.yml`, `.github/dependabot.yml`, `SECURITY.md`, this changelog.
