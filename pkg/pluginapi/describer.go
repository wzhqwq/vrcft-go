package pluginapi

import (
	"errors"
	"regexp"
	"strings"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

var (
	descriptorIDPattern         = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
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
	if !validSemanticVersion(d.Version) {
		return errors.New("Descriptor.Version must be a semantic version")
	}
	if d.Capabilities == 0 || d.Capabilities&^knownDescriptorCapabilities != 0 {
		return errors.New("Descriptor.Capabilities must be a nonempty subset of known capabilities")
	}
	return nil
}

func validSemanticVersion(version string) bool {
	if strings.Count(version, "+") > 1 {
		return false
	}
	coreAndPrerelease, build, hasBuild := strings.Cut(version, "+")
	if hasBuild && !validVersionIdentifiers(build, false) {
		return false
	}

	core, prerelease, hasPrerelease := strings.Cut(coreAndPrerelease, "-")
	if hasPrerelease && !validVersionIdentifiers(prerelease, true) {
		return false
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if !validCoreVersionNumber(part) {
			return false
		}
	}
	return true
}

func validCoreVersionNumber(value string) bool {
	if value == "" || len(value) > 1 && value[0] == '0' {
		return false
	}
	for index := range len(value) {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func validVersionIdentifiers(value string, rejectNumericLeadingZero bool) bool {
	if value == "" {
		return false
	}
	for identifier := range strings.SplitSeq(value, ".") {
		if identifier == "" {
			return false
		}
		numeric := true
		for index := range len(identifier) {
			character := identifier[index]
			if character < '0' || character > '9' {
				numeric = false
			}
			if !isVersionIdentifierCharacter(character) {
				return false
			}
		}
		if rejectNumericLeadingZero && numeric && len(identifier) > 1 && identifier[0] == '0' {
			return false
		}
	}
	return true
}

func isVersionIdentifierCharacter(character byte) bool {
	return character >= '0' && character <= '9' ||
		character >= 'A' && character <= 'Z' ||
		character >= 'a' && character <= 'z' ||
		character == '-'
}
