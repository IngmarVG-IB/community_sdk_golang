// Package experimental exposes alpha and beta Kentik API versions that have no stable equivalent yet.
//
// Everything here can change or disappear without notice, following the API itself: Kentik may change
// pre-release versions incompatibly, and this package follows api-schema-public. The accessors are
// generated (see internal/cmd/gen); the connection, auth, retries and error handling are those of the
// kentikapi.Client passed to [New].
//
//	x := experimental.New(client)
//	devices, err := x.DeviceV202504Beta2().ListDevices(ctx, &device.ListDevicesRequest{})
package experimental

import "google.golang.org/grpc"

// ConnProvider is implemented by *kentikapi.Client.
type ConnProvider interface {
	Conn() grpc.ClientConnInterface
}

// Client gives access to experimental API versions over an existing kentikapi.Client connection.
type Client struct {
	conn grpc.ClientConnInterface
}

// New returns experimental accessors that share c's connection. Closing c closes them too.
func New(c ConnProvider) *Client {
	return &Client{conn: c.Conn()}
}
