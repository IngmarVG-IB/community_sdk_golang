// Command synthetics lists synthetic tests and agents (synthetics v202309).
//
//	KTAPI_AUTH_EMAIL=... KTAPI_AUTH_TOKEN=... go run ./examples/synthetics -region us
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	synthetics "github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v202309"

	"github.com/kentik/community_sdk_golang/examples/internal/exampleutil"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	client, err := exampleutil.NewClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	admin := client.SyntheticsV202309().Admin

	tests, err := admin.ListTests(ctx, &synthetics.ListTestsRequest{})
	if err != nil {
		return fmt.Errorf("list tests: %w", err)
	}
	for _, t := range tests.GetTests() {
		fmt.Printf("test  %-10s %-12s %s\n", t.GetId(), t.GetType(), t.GetName())
	}

	agents, err := admin.ListAgents(ctx, &synthetics.ListAgentsRequest{})
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}
	for _, a := range agents.GetAgents() {
		fmt.Printf("agent %-10s %-12s %s\n", a.GetId(), a.GetStatus(), a.GetSiteName())
	}
	return nil
}
