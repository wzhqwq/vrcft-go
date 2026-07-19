package pluginapi

import "github.com/wzhqwq/vrcft-go/pkg/trackingmodel"

type Descriptor struct {
	ID          string
	Name        string
	Version     string
	Description string

	Capabilities trackingmodel.Capability
}
