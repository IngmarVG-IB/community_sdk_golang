// Package kentikapi is a Go client for the Kentik APIs.
//
// The v6 APIs are gRPC services published in [api-schema-public]. This package does not wrap their
// messages: you call the clients generated there, with their request and response types, through one
// configured connection that adds authentication, retries, rate limiting, structured errors and logging.
//
// # Getting started
//
//	email, token, err := kentikapi.GetCredentials(kentikapi.DefaultProfile) // KTAPI_AUTH_EMAIL / KTAPI_AUTH_TOKEN
//	if err != nil { ... }
//	client, err := kentikapi.New(ctx,
//		kentikapi.WithCredentials(email, token),
//		kentikapi.WithRegion(kentikapi.RegionEU), // default RegionUS
//	)
//	if err != nil { ... }
//	defer client.Close()
//
//	resp, err := client.SyntheticsV202309().Admin.ListTests(ctx, &synthetics.ListTestsRequest{})
//
// where synthetics is github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v202309.
//
// # Services and versions
//
// Every stable service version has a typed accessor named after its schema directory and version:
// [Client.SyntheticsV202309], [Client.CloudExportV202506], [Client.AlertingPublicV202505],
// [Client.UserV202211], and so on. Versions with one gRPC service return its client; versions with several
// return a struct with one field per service (for example .Admin and .Data for synthetics).
//
// Each API also has an unversioned alias for its newest complete stable version, such as
// [Client.Synthetics] or [Client.CloudExport]. The alias changes type when Kentik publishes a newer stable
// version, so libraries should prefer the versioned accessors.
//
// Alpha and beta versions without a stable equivalent live in package
// [github.com/kentik/community_sdk_golang/kentikapi/experimental]. Anything else in the schema can be
// reached with [Client.Conn] and the generated constructor. [Services] lists every service and method the
// SDK knows about. Accessors and the registry are generated from the protos (make generate).
//
// # Errors
//
// Every failed v6 call returns a [*Error] with the service, method, gRPC code, message, request id (when
// the server sends one) and number of attempts. It implements GRPCStatus, so status.Code(err) and
// status.FromError keep working, and the IsNotFoundError, IsAuthError, IsTemporaryError... helpers
// classify it.
//
// # Retries, timeouts and rate limiting
//
// Read-only RPCs (see [IsIdempotent]) are retried on Unavailable, ResourceExhausted and DeadlineExceeded
// with exponential backoff and jitter, up to 4 retries; see [RetryPolicy] and [WithRetryPolicy]. Each call
// is limited to 100 seconds in total ([WithTimeout]); [WithAttemptTimeout] bounds each attempt.
// [WithRateLimit] adds a client-side token bucket. All of these honour the caller's context.
//
// # Pagination
//
// [Paginate] turns a paged list RPC into an iterator (iter.Seq2):
//
//	for ev, err := range kentikapi.Paginate(ctx, client.AuditPublic().ListAuditEvents,
//		&audit.ListAuditEventsRequest{}, (*audit.ListAuditEventsResponse).GetEvents) {
//		...
//	}
//
// # Testing
//
// Package [github.com/kentik/community_sdk_golang/kentikapitest] runs an in-memory fake server with
// pluggable service implementations, so code that uses this package can be tested offline.
//
// # Legacy v5 REST API
//
// The exported fields of [Client] (Users, Devices, Sites, ...) and package models wrap the v5 REST API.
// They still work but are deprecated and frozen; see docs/UPGRADING.md in the repository.
//
// [api-schema-public]: https://github.com/kentik/api-schema-public
package kentikapi
