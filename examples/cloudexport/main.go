// Command cloudexport lists cloud exports (cloud_export v202506) and shows structured error handling.
//
//	go run ./examples/cloudexport -region eu
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	cloudexport "github.com/kentik/api-schema-public/gen/go/kentik/cloud_export/v202506"

	"github.com/kentik/community_sdk_golang/examples/internal/exampleutil"
	"github.com/kentik/community_sdk_golang/kentikapi"
)

func main() {
	ctx := context.Background()
	client, err := exampleutil.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := client.CloudExport().ListCloudExports(ctx, &cloudexport.ListCloudExportsRequest{})
	var apiErr *kentikapi.Error
	switch {
	case errors.As(err, &apiErr):
		log.Fatalf("%s failed: code=%s request_id=%s attempts=%d", apiErr.Method, apiErr.Code, apiErr.RequestID,
			apiErr.Attempts)
	case err != nil:
		log.Fatal(err)
	}
	for _, e := range resp.GetExports() {
		fmt.Printf("%-10s %-8s %s\n", e.GetId(), e.GetCloudProvider(), e.GetName())
	}
	if n := resp.GetInvalidExportsCount(); n > 0 {
		fmt.Printf("(%d invalid exports skipped)\n", n)
	}
}
