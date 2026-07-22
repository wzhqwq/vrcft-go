package pluginapi_test

import (
	"context"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type exampleDriver struct{}

func (exampleDriver) Descriptor() pluginapi.Descriptor {
	return pluginapi.Descriptor{
		APIVersion:   pluginapi.APIVersion,
		ID:           "example.driver",
		Name:         "Example Driver",
		Version:      "1.0.0",
		Capabilities: trackingmodel.CapabilityEye,
	}
}

func (exampleDriver) Run(ctx context.Context, host pluginapi.Host) error {
	startup := host.Startup()
	if startup.Active {
		host.PublishFrame(trackingmodel.TrackingFrame{Capabilities: startup.Subscription.Capabilities})
	}
	host.PublishStatus(pluginapi.DeviceStatus{State: pluginapi.DeviceReady})
	host.Log(pluginapi.LogInfo, "driver started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-host.Events():
			if !ok {
				return nil
			}
			switch event.(type) {
			case pluginapi.ShutdownRequested:
				return nil
			}
		}
	}
}

func ExampleDriver() {
	var driver pluginapi.Driver = exampleDriver{}
	_ = driver
	// Output:
}
