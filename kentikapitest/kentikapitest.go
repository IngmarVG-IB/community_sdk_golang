// Package kentikapitest runs an in-memory Kentik API server for tests, so code built on kentikapi (for
// example the Terraform providers) can be tested offline, without credentials and without the network.
//
// A Server listens on a gRPC bufconn listener. Plug in behaviour in either of two ways:
//
//   - register a full service implementation (usually a struct embedding the generated
//     Unimplemented...Server) with [WithService];
//   - handle single RPCs with a typed function using [HandleUnary].
//
// Calls to anything else fail with codes.Unimplemented. The server records every call ([Server.Calls]),
// checks the auth metadata sent by the client ([WithAuth]), and can inject failures ([WithFaults]) to
// exercise retry and error handling.
//
//	srv := kentikapitest.NewServer(t,
//		kentikapitest.HandleUnary(synthetics.SyntheticsAdminService_GetTest_FullMethodName,
//			func(ctx context.Context, req *synthetics.GetTestRequest) (*synthetics.GetTestResponse, error) {
//				return &synthetics.GetTestResponse{Test: &synthetics.Test{Id: req.GetId()}}, nil
//			}),
//	)
//	client := srv.Client(t)
//	resp, err := client.Synthetics().Admin.GetTest(ctx, &synthetics.GetTestRequest{Id: "1"})
package kentikapitest

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	"github.com/kentik/community_sdk_golang/kentikapi"
)

// Credentials used by [Server.Client] and expected by [WithAuth] when called with no arguments.
const (
	Email = "test@example.com"
	Token = "test-api-token"
)

const bufSize = 1 << 20

// Call is one RPC received by the server.
type Call struct {
	// Method is the full gRPC method, e.g. "/kentik.synthetics.v202309.SyntheticsAdminService/GetTest".
	Method string
	// Metadata is the incoming request metadata (auth headers, user agent, ...).
	Metadata metadata.MD
	// Request is the decoded request message for unary calls, nil for streams.
	Request proto.Message
}

// Server is an in-memory gRPC server. Create it with [NewServer]; it stops when the test ends.
type Server struct {
	lis      *bufconn.Listener
	srv      *grpc.Server
	handlers map[string]unaryHandler
	auth     *[2]string

	done chan struct{}

	mu     sync.Mutex
	calls  []Call
	faults map[string][]error
}

// Option configures a [Server].
type Option func(*Server, *[]func(grpc.ServiceRegistrar), *[]grpc.ServerOption)

type unaryHandler func(ctx context.Context, stream grpc.ServerStream) (proto.Message, error)

// WithService registers a service implementation, e.g.
//
//	kentikapitest.WithService(func(r grpc.ServiceRegistrar) { synthetics.RegisterSyntheticsAdminServiceServer(r, fake) })
func WithService(register func(grpc.ServiceRegistrar)) Option {
	return func(_ *Server, regs *[]func(grpc.ServiceRegistrar), _ *[]grpc.ServerOption) {
		*regs = append(*regs, register)
	}
}

// HandleUnary handles one unary RPC with fn. fullMethod is the generated ..._FullMethodName constant.
// Methods registered with HandleUnary must not belong to a service also registered with [WithService].
func HandleUnary[Req, Resp proto.Message](fullMethod string, fn func(context.Context, Req) (Resp, error)) Option {
	return func(s *Server, _ *[]func(grpc.ServiceRegistrar), _ *[]grpc.ServerOption) {
		s.handlers[fullMethod] = func(ctx context.Context, stream grpc.ServerStream) (proto.Message, error) {
			var zero Req
			req, ok := zero.ProtoReflect().Type().New().Interface().(Req)
			if !ok {
				return nil, status.Errorf(codes.Internal, "kentikapitest: cannot create request for %s", fullMethod)
			}
			if err := stream.RecvMsg(req); err != nil {
				return nil, err
			}
			s.record(ctx, fullMethod, req)
			if err := s.fault(fullMethod); err != nil {
				return nil, err
			}
			resp, err := fn(ctx, req)
			if err != nil {
				return nil, err
			}
			if err := stream.SendMsg(resp); err != nil {
				return nil, err
			}
			return resp, nil
		}
	}
}

// WithAuth makes the server reject calls whose x-ch-auth-email / x-ch-auth-api-token metadata do not
// match with codes.Unauthenticated. Without arguments it expects [Email] and [Token].
func WithAuth(emailAndToken ...string) Option {
	return func(s *Server, _ *[]func(grpc.ServiceRegistrar), _ *[]grpc.ServerOption) {
		creds := [2]string{Email, Token}
		if len(emailAndToken) == 2 {
			creds = [2]string{emailAndToken[0], emailAndToken[1]}
		}
		s.auth = &creds
	}
}

// WithFaults makes the next len(errs) calls to fullMethod fail with errs, in order, before the real handler
// runs. Use status.Error to build them, e.g. status.Error(codes.Unavailable, "down").
func WithFaults(fullMethod string, errs ...error) Option {
	return func(s *Server, _ *[]func(grpc.ServiceRegistrar), _ *[]grpc.ServerOption) {
		s.faults[fullMethod] = append(s.faults[fullMethod], errs...)
	}
}

// WithServerOptions passes options to grpc.NewServer.
func WithServerOptions(opts ...grpc.ServerOption) Option {
	return func(_ *Server, _ *[]func(grpc.ServiceRegistrar), so *[]grpc.ServerOption) {
		*so = append(*so, opts...)
	}
}

// TB is the part of testing.TB used by this package; *testing.T and *testing.B satisfy it.
type TB interface {
	Helper()
	Cleanup(func())
	Fatalf(format string, args ...any)
}

// NewServer starts a server and stops it when the test ends.
func NewServer(t TB, opts ...Option) *Server {
	t.Helper()
	s, err := Start(opts...)
	if err != nil {
		t.Fatalf("kentikapitest: start server: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

// Start starts a server outside a test (for Example functions or local tools). Call [Server.Close] when
// done.
func Start(opts ...Option) (*Server, error) {
	s := &Server{
		lis:      bufconn.Listen(bufSize),
		handlers: map[string]unaryHandler{},
		faults:   map[string][]error{},
		done:     make(chan struct{}),
	}
	var regs []func(grpc.ServiceRegistrar)
	serverOpts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(s.unaryInterceptor),
		grpc.ChainStreamInterceptor(s.streamInterceptor),
		grpc.UnknownServiceHandler(s.unknown),
	}
	for _, o := range opts {
		o(s, &regs, &serverOpts)
	}
	s.srv = grpc.NewServer(serverOpts...)
	for _, r := range regs {
		r(s.srv)
	}
	go func() {
		defer close(s.done)
		_ = s.srv.Serve(s.lis) // returns when Close stops the server
	}()
	return s, nil
}

// Close stops the server immediately; in-flight calls fail. It is safe to call more than once.
func (s *Server) Close() {
	s.srv.Stop()
	<-s.done
}

// Dial returns a dialer for grpc.WithContextDialer that connects to the in-memory listener.
func (s *Server) Dial(ctx context.Context, _ string) (net.Conn, error) { return s.lis.DialContext(ctx) }

// ClientOptions returns the kentikapi options that connect a client to this server: endpoint, dialer,
// plaintext, test credentials ([Email], [Token]) and fast retries. Append your own options after them.
func (s *Server) ClientOptions() []kentikapi.Option {
	retry := kentikapi.DefaultRetryPolicy()
	retry.InitialBackoff = time.Millisecond
	retry.MaxBackoff = 10 * time.Millisecond
	return []kentikapi.Option{
		kentikapi.WithEndpoint("passthrough:///kentikapitest"),
		kentikapi.WithInsecure(),
		kentikapi.WithCredentials(Email, Token),
		kentikapi.WithRetryPolicy(retry),
		kentikapi.WithDialOptions(grpc.WithContextDialer(s.Dial)),
	}
}

// NewClient returns a kentikapi.Client connected to the server. opts are applied after
// [Server.ClientOptions]. The caller closes it.
func (s *Server) NewClient(ctx context.Context, opts ...kentikapi.Option) (*kentikapi.Client, error) {
	return kentikapi.New(ctx, append(s.ClientOptions(), opts...)...)
}

// Client is [Server.NewClient] for tests: it fails the test on error and closes the client when the test
// ends.
func (s *Server) Client(t TB, opts ...kentikapi.Option) *kentikapi.Client {
	t.Helper()
	c, err := s.NewClient(context.Background(), opts...)
	if err != nil {
		t.Fatalf("kentikapitest: new client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// Calls returns a copy of the calls received so far, in arrival order.
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Call(nil), s.calls...)
}

// CallsTo returns the calls received for fullMethod.
func (s *Server) CallsTo(fullMethod string) []Call {
	var out []Call
	for _, c := range s.Calls() {
		if c.Method == fullMethod {
			out = append(out, c)
		}
	}
	return out
}

func (s *Server) record(ctx context.Context, method string, req proto.Message) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, Call{Method: method, Metadata: md.Copy(), Request: req})
}

func (s *Server) fault(method string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	errs := s.faults[method]
	if len(errs) == 0 {
		return nil
	}
	s.faults[method] = errs[1:]
	return errs[0]
}

func (s *Server) checkAuth(ctx context.Context) error {
	if s.auth == nil {
		return nil
	}
	md, _ := metadata.FromIncomingContext(ctx)
	if first(md, kentikapi.MetadataAuthEmail) != s.auth[0] || first(md, kentikapi.MetadataAuthToken) != s.auth[1] {
		return status.Error(codes.Unauthenticated, "kentikapitest: invalid or missing auth email/token")
	}
	return nil
}

func first(md metadata.MD, key string) string {
	if v := md.Get(key); len(v) > 0 {
		return v[0]
	}
	return ""
}

func (s *Server) unaryInterceptor(
	ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (any, error) {
	if err := s.checkAuth(ctx); err != nil {
		return nil, err
	}
	m, _ := req.(proto.Message)
	s.record(ctx, info.FullMethod, m)
	if err := s.fault(info.FullMethod); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

func (s *Server) streamInterceptor(
	srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler,
) error {
	if err := s.checkAuth(ss.Context()); err != nil {
		return err
	}
	if _, ok := s.handlers[info.FullMethod]; !ok {
		// HandleUnary records decoded requests itself; plain streams are recorded here.
		s.record(ss.Context(), info.FullMethod, nil)
		if err := s.fault(info.FullMethod); err != nil {
			return err
		}
	}
	return handler(srv, ss)
}

// unknown serves HandleUnary methods and rejects everything else.
func (s *Server) unknown(_ any, stream grpc.ServerStream) error {
	method, ok := grpc.MethodFromServerStream(stream)
	if !ok {
		return status.Error(codes.Internal, "kentikapitest: no method in stream")
	}
	h, ok := s.handlers[method]
	if !ok {
		return status.Error(codes.Unimplemented, fmt.Sprintf("kentikapitest: %s is not implemented by this fake", method))
	}
	_, err := h(stream.Context(), stream)
	return err
}
