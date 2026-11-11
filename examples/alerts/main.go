// Command alerts iterates over all alerts with kentikapi.Paginate (alerting/public v202505).
//
//	go run ./examples/alerts
package main

import (
	"context"
	"fmt"
	"log"

	alerting "github.com/kentik/api-schema-public/gen/go/kentik/alerting/public/v202505"

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

	alerts := client.AlertingPublicV202505().Alert
	n := 0
	for a, err := range kentikapi.Paginate(ctx, alerts.List, &alerting.AlertServiceListRequest{},
		(*alerting.AlertServiceListResponse).GetAlerts, kentikapi.WithPageSize(200)) {
		if err != nil {
			log.Fatal(err)
		}
		n++
		fmt.Printf("%s %s\n", a.GetId(), a.GetStartTimeAt().AsTime())
	}
	fmt.Println(n, "alerts")
}
