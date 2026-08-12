package evaluator

import "github.com/wzhqwq/vrcft-go/internal/parameters"

const validityWordCount = (parameters.ParameterCount + 63) / 64

type Snapshot struct {
	floats     [parameters.ParameterCount]float32
	bools      [parameters.ParameterCount]bool
	floatValid [validityWordCount]uint64
	boolValid  [validityWordCount]uint64
}

func (s Snapshot) Float(id parameters.ParameterID) (float32, bool) {
	definition, ok := parameters.Definition(id)
	if !ok || definition.ValueType != parameters.ValueFloat || !valid(&s.floatValid, id) {
		return 0, false
	}
	return s.floats[id], true
}

func (s Snapshot) Bool(id parameters.ParameterID) (bool, bool) {
	definition, ok := parameters.Definition(id)
	if !ok || definition.ValueType != parameters.ValueBool || !valid(&s.boolValid, id) {
		return false, false
	}
	return s.bools[id], true
}

func valid(bits *[validityWordCount]uint64, id parameters.ParameterID) bool {
	return id < parameters.ParameterCount && bits[id/64]&(uint64(1)<<(id%64)) != 0
}

func setValid(bits *[validityWordCount]uint64, id parameters.ParameterID) {
	bits[id/64] |= uint64(1) << (id % 64)
}
