package pluginapi

import (
	"errors"
	"regexp"
	"strings"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

var (
	descriptorIDPattern         = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
	semanticVersionPattern      = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	knownDescriptorCapabilities = trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression
)

type Descriptor struct {
	APIVersion   uint16
	ID           string
	Name         string
	Version      string
	Description  string
	Capabilities trackingmodel.Capability
}

func (d Descriptor) Validate() error {
	if d.APIVersion != APIVersion {
		return errors.New("Descriptor.APIVersion must equal pluginapi.APIVersion")
	}
	if !descriptorIDPattern.MatchString(d.ID) {
		return errors.New("Descriptor.ID is invalid")
	}
	if strings.TrimSpace(d.Name) == "" {
		return errors.New("Descriptor.Name must be nonblank")
	}
	if !semanticVersionPattern.MatchString(d.Version) {
		return errors.New("Descriptor.Version must be a semantic version")
	}
	if d.Capabilities == 0 || d.Capabilities&^knownDescriptorCapabilities != 0 {
		return errors.New("Descriptor.Capabilities must be a nonempty subset of known capabilities")
	}
	return nil
}
