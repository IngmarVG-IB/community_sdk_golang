# 5. Generate service accessors, registry and manifest from the protos

Date: 2026-11-10

## Status

Accepted

## Context

api-schema-public has 40+ API directories and 60+ versions, and adds versions several times a year.
Hand-writing an accessor per service version does not scale and goes stale (F10 in the community audit:
the SDKs covered 3 services on 2021-2022 versions). Retries also need per-method facts (is the RPC
read-only?) that only the protos carry.

## Decision

`internal/cmd/gen` (run by `make generate` via `go generate`) compiles every `proto/kentik/**` file of the
api-schema-public version pinned in `go.mod`, straight from the module cache, with
`github.com/bufbuild/protocompile` and the schema's own `protovendor/` imports. It emits:

- `kentikapi/zz_accessors.go`: `Client.<Api><Version>()` for every **stable** version (single-service
  versions return the generated client, multi-service versions a `...Clients` struct), plus an unversioned
  alias (`Client.Synthetics()`) for the newest stable version that covers all services of older stable
  versions;
- `kentikapi/experimental/zz_accessors.go`: the newest alpha/beta version of APIs without a stable
  version, or that add services no stable version has;
- `kentikapi/zz_registry.go`: every service and method with its HTTP mapping, streaming flags,
  idempotency (GET mapping, `idempotency_level`, or a Get/List name) and pagination style;
- `kentik-api.json`: the coverage manifest.

Naming rules: API path segments are camel-cased (`alerting/public` -> `AlertingPublic`, with a small
initialism table: AI, AS, BGP, KMI, MKP, NMS, KPTR, KTBGP); versions keep their suffix
(`V202504Beta2`). The generator fails on collisions with hand-written `Client` members.

CI runs `make check-generate` (regenerate, then `git diff --exit-code`), and a unit test compares the
committed outputs with a fresh render, so drift fails both `make test` and CI.

## Consequences

- Adding a service version is: bump `github.com/kentik/api-schema-public`, `make generate`, review the diff,
  add a CHANGELOG line.
- Unversioned aliases change type when a newer complete stable version appears. That is a breaking change
  for callers of the alias, so a schema bump that moves an alias needs a minor version bump while < v1 and
  a major one after; libraries (Terraform providers) should use versioned accessors.
- protocompile becomes a dependency of the module (used only by the generator; it is not linked into
  programs that import `kentikapi`).
