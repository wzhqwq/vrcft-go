package parameters

type RangeKind uint8

const (
	RangeUnit RangeKind = iota
	RangeSignedUnit
	RangeEyeLid
	RangePupilCentimeter
)

type ParameterKind uint8

const (
	ParameterDetailed ParameterKind = iota
	ParameterSimplified
)

type ParameterDefinition struct {
	ID       ParameterID
	Name     string
	Category string
	Kind     ParameterKind
	Range    RangeKind

	SupportsBinary bool
}

type EvaluationPlan struct {
	Required []ParameterID
}
