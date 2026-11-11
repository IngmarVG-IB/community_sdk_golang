// Command experimental lists devices with the beta device API (device v202504beta2), which has no stable
// version yet and therefore lives in package kentikapi/experimental.
//
//	go run ./examples/experimental
package main

import (
	"context"
	"fmt"
	"log"

	device "github.com/kentik/api-schema-public/gen/go/kentik/device/v202504beta2"

	"github.com/kentik/community_sdk_golang/examples/internal/exampleutil"
	"github.com/kentik/community_sdk_golang/kentikapi/experimental"
)

func main() {
	ctx := context.Background()
	client, err := exampleutil.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	resp, err := experimental.New(client).DeviceV202504Beta2().ListDevices(ctx, &device.ListDevicesRequest{})
	if err != nil {
		log.Fatal(err)
	}
	for _, d := range resp.GetDevices() {
		fmt.Println(d.GetId(), d.GetDeviceName())
	}
}
