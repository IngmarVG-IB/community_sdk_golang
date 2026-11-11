# 4. Build the v6 SDK on the api-schema-public gRPC clients

Date: 2026-11-10

## Status

Accepted. Supersedes the v6 parts of the v0.2.x design (openapi-generator clients) and the hand-written
domain models of the `dev_grpc` line.

## Context

Kentik publishes its v6 APIs as protobuf services in `github.com/kentik/api-schema-public`, with Go
gRPC clients (`gen/go`), grpc-gateway REST mappings and OpenAPI v2/v3 specs generated from the same protos.
At the time of writing the schema has 44 services and 61 versions; this SDK covered two of them on
2021-2022 versions (synthetics v202101beta1/v202202, cloud_export v202101beta1).

Two earlier approaches did not keep up:

- v0.2.x generated REST clients with openapi-generator from copied OpenAPI specs. Each API version needed a
  manual regeneration, the generated code was large (tens of thousands of lines), and it shipped its own
  vulnerable dependencies (`x/oauth2`, `x/net`).
- `dev_grpc` used the generated gRPC stubs but wrapped every message in hand-written Go models plus
  converters. That cost roughly 2,000 lines per service and broke when the schema moved on (its synthetics
  v202202 package has since been deleted upstream).

Options considered:

1. **openapi-generator over `gen/openapiv3`.** Plain HTTP/JSON, but loses typed enums and oneofs, needs a
   generation step per version, and duplicates code Kentik already publishes.
2. **Hand-written REST over the grpc-gateway mapping with `protojson`.** No gRPC dependency, but every call
   path, verb and query mapping has to be maintained by hand, and streaming RPCs cannot be used.
3. **Hand-written domain models over gRPC** (`dev_grpc`). Nicer Go types, but the cost per service is too high
   to cover 40+ services, and the models drift.
4. **Use the generated gRPC clients directly, behind one configured connection.** The SDK owns
   transport concerns (auth, endpoints, retries, rate limiting, errors, logging) and generates thin typed
   accessors; request and response types are the generated protobuf messages.
5. **connect-go.** Would need our own code generation from the protos and a second set of stubs; the Kentik
   endpoints speak standard gRPC, so it brings no benefit.

## Decision

Option 4. `kentikapi.New(ctx, opts...)` returns a `*kentikapi.Client` holding a single `*grpc.ClientConn`
to `grpc.api.kentik.com:443` (US) or `grpc.api.kentik.eu:443` (EU), or a custom endpoint. Cross-cutting
behaviour is implemented once, in interceptors and per-RPC credentials:

- authentication: `x-ch-auth-email` and `x-ch-auth-api-token` metadata (gRPC lowercases header names; the
  protos declare them as `X-CH-Auth-Email` and `X-CH-Auth-API-Token` in their OpenAPI security definitions),
  sent via `credentials.PerRPCCredentials` that require TLS unless the caller opts into an insecure endpoint;
- retries with exponential backoff and jitter on `Unavailable`, `ResourceExhausted` and `DeadlineExceeded`,
  for idempotent RPCs only by default;
- an optional client-side token-bucket rate limit;
- structured errors (`*kentikapi.Error`) that wrap the gRPC status with service, method and request id and
  still satisfy `status.FromError`;
- `log/slog` logging.

Typed per-service accessors (`client.SyntheticsV202309()`, `client.Synthetics()`, ...) are generated from
the protos (see ADR 5). Callers use the generated request and response messages directly.

## Consequences

- New API versions arrive by bumping the api-schema-public module and running `make generate`; no
  hand-written models.
- The SDK depends on `google.golang.org/grpc` and `google.golang.org/protobuf`. Their upgrade cadence (and
  that of api-schema-public) drives ours.
- Users work with protobuf messages (`GetX()` accessors, `proto.Equal`, `protojson`) rather than plain
  structs. This is the main ergonomic trade-off; pagination iterators and the `kentikapitest` fake server
  soften it.
- The v5 REST layer stays as-is, deprecated, reachable through the same `Client`.
