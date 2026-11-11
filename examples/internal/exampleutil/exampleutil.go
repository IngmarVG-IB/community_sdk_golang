// Package exampleutil holds the shared setup of the example programs.
package exampleutil

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/kentik/community_sdk_golang/kentikapi"
)

// NewClient builds a client from KTAPI_AUTH_EMAIL / KTAPI_AUTH_TOKEN (or ~/.kentik/<profile>) and the
// -region flag. The examples call the live Kentik API: run them only against a tenant you control.
func NewClient(ctx context.Context) (*kentikapi.Client, error) {
	region := flag.String("region", "us", "Kentik region: us or eu")
	profile := flag.String("profile", kentikapi.DefaultProfile, "profile file name under ~/.kentik")
	debug := flag.Bool("debug", false, "log every RPC")
	flag.Parse()

	email, token, err := kentikapi.GetCredentials(*profile)
	if err != nil {
		return nil, err
	}
	level := slog.LevelWarn
	if *debug {
		level = slog.LevelDebug
	}
	return kentikapi.New(ctx,
		kentikapi.WithCredentials(email, token),
		kentikapi.WithRegion(kentikapi.Region(*region)),
		kentikapi.WithUserAgent("community-sdk-golang-examples"),
		kentikapi.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))),
	)
}
