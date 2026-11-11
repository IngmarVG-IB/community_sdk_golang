# AGENTS.md

Guide for maintainers and coding agents working on `github.com/kentik/community_sdk_golang`.

## Purpose

Go SDK for the Kentik APIs. The v6 APIs are gRPC services defined in
[`github.com/kentik/api-schema-public`](https://github.com/kentik/api-schema-public); this module builds on the
Go clients generated there. The legacy v5 REST API is still wrapped (deprecated, frozen).

## Layout

| path | what |
|---|---|
| `kentikapi/` | Public package: `Client` (`New`, `Close`, `Conn`), options, regions, auth, retries, rate limit, `Error`, logging, credentials helpers. |
| `kentikapi/models/`, `kentikapi/internal/{api_*,resources,...}` | Legacy v5 REST layer. Deprecated, do not extend. |
| `kentikapi/cloud/`, `kentikapi/internal/cloud/` | Legacy cloud_export v202101beta1 wrapper. Deprecated. |
| `examples/legacy/` | v5 and v202101beta1 usage examples (build tag `examples`). They call the live API, so CI only compiles them. |
| `internal/cmd/manifestcheck/` | Validator for `kentik-api.json`. |
| `docs/adr/` | Architecture decision records (0004: v6 on api-schema-public gRPC). |
| `kentik-api.json` | Machine-readable coverage manifest. |

## Commands

All commands work offline once the module cache is warm.

| task | command |
|---|---|
| build (incl. examples) | `make build` |
| unit tests | `make test` |
| compile examples only | `make examples` |
| lint (golangci-lint v2, built with your toolchain) | `make lint` |
| vulnerability scan | `make vuln` |
| regenerate code | `make generate` |
| generated code drift check | `make check-generate` |
| format | `make fmt` |
| validate manifest | `make manifest` |
| everything CI runs | `make ci` |

## Adding a new API service or version

See the "Service coverage" section once the accessor generator lands; until then: bump
`github.com/kentik/api-schema-public` in `go.mod` (`go get github.com/kentik/api-schema-public@master`),
run `make generate test`, and update `kentik-api.json`.

## Release process

1. Make sure `make ci` passes on `master` and `CHANGELOG.md` has the release notes under a version heading.
2. Tag `vX.Y.Z` (semver) on `master` and create a GitHub release from it. Never tag from a feature branch.

## Conventions

- Go 1.26 minimum; CI runs 1.26 and 1.27.
- `context.Context` is the first parameter of every call that does I/O.
- Errors wrap with `%w`; check them with `errors.Is`/`errors.As` or the `kentikapi.Is*Error` helpers.
- Formatting: `gofumpt` + `goimports` with local prefix `github.com/kentik/community_sdk_golang`.
- Commits: small and logically separate; each one builds and passes tests.

## Do and don't

- Do keep tests offline: `httptest`, gRPC `bufconn`, recorded fixtures. Tests that need a live tenant must be
  gated behind an env var and skipped by default.
- Do not commit or look for credentials, `.env` files or tenant data.
- Do not extend the v5 REST layer. Keep its exported identifiers compiling and marked `// Deprecated:`.
- Do not hand-edit generated files; change the generator and run `make generate`.
- Do not break exported identifiers without a migration note in `docs/UPGRADING.md` and `CHANGELOG.md`.
