package osc

import "github.com/wzhqwq/vrcft-go/internal/parameters"

// NewVRCFTParameterCatalog compiles the generated parameter definitions into
// the immutable OSC-specific catalog used for avatar address matching and output.
func NewVRCFTParameterCatalog() (*ParameterCatalog, error) {
	return NewParameterCatalog(parameters.Definitions[:])
}
