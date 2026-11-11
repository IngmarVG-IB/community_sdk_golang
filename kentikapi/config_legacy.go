package kentikapi

import (
	"strings"
	"time"
)

// Config is the client configuration of v0.2.x. It implements [Option], so code written as
// NewClient(kentikapi.Config{...}) keeps compiling, and it can be mixed with other options:
//
//	client, err := kentikapi.New(ctx, kentikapi.Config{AuthEmail: e, AuthToken: t}, kentikapi.WithRegion(kentikapi.RegionEU))
//
// Deprecated: use [New] with [WithCredentials], [WithRegion] or [WithEndpoint], [WithRetryPolicy],
// [WithTimeout] and [WithLogPayloads].
type Config struct {
	// APIURL is the v5 REST base URL, with or without the "/api/v5" suffix (both forms were used). The
	// gRPC endpoint is derived from it as described for [WithAPIURL]. Empty means the US region.
	APIURL string
	// Deprecated: ignored. The openapi-generated v6 clients of v0.2.x were removed; v6 calls use gRPC.
	CloudExportAPIURL string
	// Deprecated: ignored, see CloudExportAPIURL.
	SyntheticsAPIURL string
	AuthEmail        string
	AuthToken        string
	// RetryCfg applies to both v5 and v6 calls. Nil fields keep the defaults.
	RetryCfg    RetryConfig
	LogPayloads bool
	// Timeout of a single call including retries. Nil keeps the default (100 seconds).
	Timeout *time.Duration
}

func (c Config) apply(cfg *config) {
	if c.APIURL != "" {
		WithAPIURL(strings.TrimSuffix(strings.TrimSuffix(c.APIURL, "/"), "/api/v5")).apply(cfg)
	}
	if c.AuthEmail != "" || c.AuthToken != "" {
		WithCredentials(c.AuthEmail, c.AuthToken).apply(cfg)
	}
	if c.RetryCfg.MaxAttempts != nil {
		WithRetryMaxAttempts(*c.RetryCfg.MaxAttempts).apply(cfg)
	}
	if c.RetryCfg.MinDelay != nil {
		WithRetryMinDelay(*c.RetryCfg.MinDelay).apply(cfg)
	}
	if c.RetryCfg.MaxDelay != nil {
		WithRetryMaxDelay(*c.RetryCfg.MaxDelay).apply(cfg)
	}
	if c.LogPayloads {
		WithLogPayloads().apply(cfg)
	}
	if c.Timeout != nil {
		WithTimeout(*c.Timeout).apply(cfg)
	}
}

// FillDefaults sets unset fields to their defaults, as in v0.2.x.
//
// Deprecated: New applies defaults itself.
func (c *Config) FillDefaults() {
	if c.APIURL == "" {
		c.APIURL = APIURLUS
	}
	c.RetryCfg.FillDefaults()
	if c.Timeout == nil {
		t := defaultTimeout
		c.Timeout = &t
	}
}
