package processing

type Validator interface {
	Validate(frame *CanonicalFrame)
}
