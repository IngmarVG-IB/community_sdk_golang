package kentikapi

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"runtime/debug"
	"sync"

	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/kentik/community_sdk_golang/kentikapi/internal/api_connection"
	"github.com/kentik/community_sdk_golang/kentikapi/internal/cloud"
	"github.com/kentik/community_sdk_golang/kentikapi/internal/resources"
)

const modulePath = "github.com/kentik/community_sdk_golang"

// Client is the entry point to the Kentik APIs. Create it with [New] and release it with [Client.Close].
// A Client is safe for concurrent use; create one per process and share it.
//
// v6 (gRPC) services are reached through typed accessors such as Client.SyntheticsV202309 or the
// Client.Synthetics alias for the newest stable version. The exported fields are the legacy v5 REST
// resources and are deprecated.
type Client struct {
	// Deprecated: v5 REST resource. Use the v6 alerting accessors.
	Alerting *resources.AlertingAPI
	// Deprecated: wraps cloud_export v202101beta1 (beta, superseded). Use Client.CloudExport.
	Cloud *cloud.API
	// Deprecated: v5 REST resource. The v6 schema has custom_application v202501alpha1 (experimental).
	CustomApplications *resources.CustomApplicationsAPI
	// Deprecated: v5 REST resource. The v6 schema has custom_dimension v202411alpha1 (experimental).
	CustomDimensions *resources.CustomDimensionsAPI
	// Deprecated: v5 REST resource. Use Client.Label (label v202210).
	DeviceLabels *resources.DeviceLabelsAPI
	// Deprecated: v5 REST resource. The v6 schema has device v202504beta2 (experimental).
	Devices *resources.DevicesAPI
	// Deprecated: v5 REST resource. Use Client.MKP (mkp v202407).
	MyKentikPortal *resources.MyKentikPortalAPI
	// Deprecated: v5 REST resource. The v6 schema has plan v202501alpha1 (experimental).
	Plans *resources.PlansAPI
	// Deprecated: v5 REST resource (no v6 equivalent yet).
	Query *resources.QueryAPI
	// Deprecated: v5 REST resource. The v6 schema has saved_filter v202501alpha1 (experimental).
	SavedFilters *resources.SavedFiltersAPI
	// Deprecated: v5 REST resource. Use Client.Site (site v202509).
	Sites *resources.SitesAPI
	// Deprecated: v5 REST resource. The v6 schema has flow_tag v202404alpha1 (experimental).
	Tags *resources.TagsAPI
	// Deprecated: v5 REST resource. Use Client.User (user v202211).
	Users *resources.UsersAPI

	conn      *grpc.ClientConn
	cfg       config
	limiter   *rate.Limiter
	logger    *slog.Logger
	closeOnce sync.Once
	closeErr  error
}

// New creates a Client. Without options it targets the US region with no credentials; pass at least
// [WithCredentials]. New does not perform network I/O unless [WithEagerConnect] is given, in which case it
// waits for the connection until ctx is done.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg := defaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	cfg.retry = cfg.retry.normalized()

	target, apiURL, plaintext, err := resolveEndpoints(cfg)
	if err != nil {
		return nil, err
	}

	c := &Client{cfg: cfg, logger: cfg.logger}
	if c.logger == nil {
		if cfg.logPayloads {
			c.logger = slog.Default()
		} else {
			c.logger = slog.New(slog.DiscardHandler)
		}
	}
	if cfg.rateLimit > 0 {
		c.limiter = rate.NewLimiter(cfg.rateLimit, cfg.rateBurst)
	}

	dialOpts := []grpc.DialOption{
		grpc.WithUserAgent(userAgent(cfg.userAgent)),
		grpc.WithChainUnaryInterceptor(c.unaryInterceptor),
		grpc.WithChainStreamInterceptor(c.streamInterceptor),
	}
	if plaintext {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			MinVersion: tls.VersionTLS13,
		})))
	}
	switch {
	case cfg.perRPC != nil:
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(cfg.perRPC))
	case cfg.authEmail != "" || cfg.authToken != "":
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(
			tokenCredentials{email: cfg.authEmail, token: cfg.authToken, insecure: plaintext}))
	}
	dialOpts = append(dialOpts, cfg.dialOptions...)

	conn, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("kentikapi: create gRPC client for %q: %w", target, err)
	}
	c.conn = conn

	if cfg.eagerConnect {
		if err := waitReady(ctx, conn); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("kentikapi: connect to %q: %w", target, err)
		}
	}

	c.initLegacy(apiURL)
	return c, nil
}

// NewClient creates a Client without a context.
//
// Deprecated: use [New].
func NewClient(opts ...ClientOption) (*Client, error) {
	return New(context.Background(), opts...)
}

// Conn returns the underlying gRPC connection, already configured with authentication, retries, rate
// limiting and logging. Use it to build any generated api-schema-public client directly:
//
//	svc := synthetics.NewSyntheticsAdminServiceClient(client.Conn())
func (c *Client) Conn() grpc.ClientConnInterface { return c.conn }

// Close releases the connection. Calls in flight fail with codes.Canceled. Close is idempotent.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		if c.conn != nil {
			c.closeErr = c.conn.Close()
		}
	})
	return c.closeErr
}

func (c *Client) initLegacy(apiURL string) {
	rc := api_connection.NewRestClient(api_connection.RestClientConfig{
		APIURL:    apiURL + "/api/v5",
		AuthEmail: c.cfg.authEmail,
		AuthToken: c.cfg.authToken,
		RetryCfg:  c.cfg.legacyRetry,
		Timeout:   &c.cfg.timeout,
	})
	lp := c.cfg.logPayloads
	c.Alerting = resources.NewAlertingAPI(rc, lp)
	c.Cloud = cloud.NewAPI(c.conn)
	c.CustomApplications = resources.NewCustomApplicationsAPI(rc, lp)
	c.CustomDimensions = resources.NewCustomDimensionsAPI(rc, lp)
	c.DeviceLabels = resources.NewDeviceLabelsAPI(rc, lp)
	c.Devices = resources.NewDevicesAPI(rc, lp)
	c.MyKentikPortal = resources.NewMyKentikPortalAPI(rc, lp)
	c.Plans = resources.NewPlansAPI(rc, lp)
	c.Query = resources.NewQueryAPI(rc, lp)
	c.SavedFilters = resources.NewSavedFiltersAPI(rc, lp)
	c.Sites = resources.NewSitesAPI(rc, lp)
	c.Tags = resources.NewTagsAPI(rc, lp)
	c.Users = resources.NewUsersAPI(rc, lp)
}

// resolveEndpoints returns the gRPC target, the v5 REST base URL and whether TLS is disabled.
func resolveEndpoints(cfg config) (target, apiURL string, plaintext bool, err error) {
	ri, err := cfg.region.info()
	if err != nil {
		return "", "", false, err
	}
	target, apiURL, plaintext = ri.grpcEndpoint, ri.apiURL, cfg.insecure
	if cfg.apiURL != "" {
		apiURL = cfg.apiURL
		if cfg.endpoint == "" {
			var tlsOn bool
			target, tlsOn, err = grpcTargetFromAPIURL(cfg.apiURL)
			if err != nil {
				return "", "", false, fmt.Errorf("kentikapi: invalid API URL %q: %w", cfg.apiURL, err)
			}
			plaintext = plaintext || !tlsOn
		}
	}
	if cfg.endpoint != "" {
		target = cfg.endpoint
	}
	return target, apiURL, plaintext, nil
}

// grpcTargetFromAPIURL keeps the pre-v0.3 mapping: https://api.kentik.com -> grpc.api.kentik.com:443.
// IP addresses (local test servers) are used as-is.
func grpcTargetFromAPIURL(apiURL string) (target string, tlsEnabled bool, err error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", false, err
	}
	if u.Host == "" {
		return "", false, errors.New("missing host")
	}
	tlsEnabled = u.Scheme == "https"
	port := u.Port()
	if port == "" {
		port = "80"
		if tlsEnabled {
			port = "443"
		}
	}
	host := u.Hostname()
	if net.ParseIP(host) == nil {
		host = "grpc." + host
	}
	return net.JoinHostPort(host, port), tlsEnabled, nil
}

func waitReady(ctx context.Context, conn *grpc.ClientConn) error {
	conn.Connect()
	for {
		s := conn.GetState()
		if s == connectivity.Ready {
			return nil
		}
		if s == connectivity.Shutdown {
			return errors.New("connection shut down")
		}
		if !conn.WaitForStateChange(ctx, s) {
			return ctx.Err()
		}
	}
}

// Version returns the SDK module version as recorded in the binary's build info, or "devel".
func Version() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if bi.Main.Path == modulePath && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
		for _, d := range bi.Deps {
			if d.Path == modulePath {
				return d.Version
			}
		}
	}
	return "devel"
}

func userAgent(prefix string) string {
	ua := "kentik-community-sdk-golang/" + Version()
	if prefix != "" {
		ua = prefix + " " + ua
	}
	return ua
}
