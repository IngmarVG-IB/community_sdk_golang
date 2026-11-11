package kentikapi

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// gRPC metadata keys used for authentication. The protos document them as X-CH-Auth-Email and
// X-CH-Auth-API-Token (OpenAPI security definitions); gRPC transmits header names in lower case.
const (
	MetadataAuthEmail = "x-ch-auth-email"
	MetadataAuthToken = "x-ch-auth-api-token" //nolint:gosec // header name, not a credential
)

// tokenCredentials implements credentials.PerRPCCredentials with Kentik email and API token auth.
type tokenCredentials struct {
	email, token string
	insecure     bool
}

var _ credentials.PerRPCCredentials = tokenCredentials{}

// NewTokenCredentials returns per-RPC credentials that send the Kentik email and API token with every call.
// They refuse to be sent over a connection without transport security; use [WithInsecure] only for
// local fakes.
func NewTokenCredentials(email, token string) credentials.PerRPCCredentials {
	return tokenCredentials{email: email, token: token}
}

func (c tokenCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	if c.email == "" || c.token == "" {
		// A status error keeps gRPC from reporting (and us from retrying) this as Unavailable.
		return nil, status.Error(codes.Unauthenticated, "kentikapi: missing auth email or API token; use WithCredentials")
	}
	return map[string]string{
		MetadataAuthEmail: c.email,
		MetadataAuthToken: c.token,
	}, nil
}

func (c tokenCredentials) RequireTransportSecurity() bool { return !c.insecure }
