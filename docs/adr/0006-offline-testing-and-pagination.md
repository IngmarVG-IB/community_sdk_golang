# 6. Offline fake server and reflection-based pagination

Date: 2026-11-10

## Status

Accepted

## Context

Tests must run offline (no tenant credentials exist in CI), and downstream users such as the Terraform
providers need the same ability. The v0.2 line shipped an openapi-generated HTTP fake server
(`kentikapi/fakeapiserver`) that had to be regenerated per API version. Paged list RPCs in api-schema-public
use two different styles (alerting's `PaginationConfig`/`PaginationInfo`, audit's top-level
`offset`/`limit`), and more may follow.

## Decision

- Package `kentikapitest` runs a gRPC server on `bufconn`. Behaviour is pluggable per service
  (`WithService` with any generated server implementation) or per RPC (`HandleUnary` with a typed
  function, served through the gRPC unknown-service handler so no full implementation is needed). It
  records calls, can enforce auth metadata and inject faults, and returns ready `kentikapi.Client`s.
  It works for every service in the schema without generated code of its own.
- `kentikapi.Paginate` is one generic function returning `iter.Seq2[Item, error]`. It detects the paging
  style with protoreflect on the request (and the response's `PaginationInfo`), so it needs no per-RPC
  generated wrappers. The generator records the detected style per method in the registry and manifest,
  so a new style shows up in review.

## Consequences

- The SDK's own tests use `kentikapitest`, which keeps it exercised.
- Paginate relies on field names (`pagination`, `offset`, `limit`, `has_more_results`, `total_count`); a
  schema change to those names needs a code change, caught by the generator test and Paginate tests.
- `kentikapitest` is a public API with compatibility obligations like `kentikapi` itself.
