package trackingmodel

type Capability uint32

const (
	CapabilityEye Capability = 1 << iota
	CapabilityExpression
	CapabilityLip
)

func (c Capability) Has(target Capability) bool {
	return c&target != 0
}
