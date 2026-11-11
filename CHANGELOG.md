# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Built on the upstream `dev_grpc` line (50 commits ahead of `master`, last change 2022-07-28), which already
replaced the openapi-generator v6 clients with gRPC.

### Changed
- Minimum Go version is now 1.26 (was 1.17 on `dev_grpc`, 1.15 in v0.2.0). CI tests Go 1.26 and 1.27.
- All dependencies upgraded; `govulncheck ./...` is clean. `github.com/kentik/api-schema-public` pinned to
  `v0.0.0-20261110124345-5d1d2fa374e4`.
- Dropped the `replace` of `github.com/hashicorp/go-retryablehttp` with a personal fork; upstream v0.7.8 is used.
- Development tools (`gofumpt`, `stringer`) are Go 1.24+ `tool` directives instead of `tools/tools.go`.
- golangci-lint configuration migrated to v2.
- Replaced deprecated `io/ioutil` and `reflect.PtrTo` usage.

### Removed
- The synthetics v202202 layer (`kentikapi/synthetics` package and `Client.Synthetics` field). The
  generated code it wrapped (`gen/go/kentik/synthetics/v202202`) no longer exists in api-schema-public.
  Its tests were removed with it.

### Added
- AI-ready baseline: `AGENTS.md`, `CLAUDE.md`, `kentik-api.json` (+ validator), `Makefile`,
  `.github/workflows/ci.yml`, `.github/dependabot.yml`, `SECURITY.md`, this changelog.
