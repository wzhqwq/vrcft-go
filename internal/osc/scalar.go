package osc

import "math"

type scalarKind uint8

const (
	scalarFloat32 scalarKind = iota + 1
	scalarInt32
	scalarFalse
	scalarTrue
)

type scalarValue struct {
	bits uint32
	kind scalarKind
}

func floatScalar(value float32) scalarValue {
	return scalarValue{bits: math.Float32bits(value), kind: scalarFloat32}
}

func intScalar(value int32) scalarValue {
	return scalarValue{bits: uint32(value), kind: scalarInt32}
}

func boolScalar(value bool) scalarValue {
	if value {
		return scalarValue{kind: scalarTrue}
	}
	return scalarValue{kind: scalarFalse}
}

func (value scalarValue) typeTag() byte {
	switch value.kind {
	case scalarFloat32:
		return 'f'
	case scalarInt32:
		return 'i'
	case scalarFalse:
		return 'F'
	case scalarTrue:
		return 'T'
	default:
		return 0
	}
}

func scalarEqual(left, right scalarValue, epsilon float32) bool {
	if left.kind != right.kind {
		return false
	}
	if left.kind == scalarFloat32 {
		delta := math.Float32frombits(left.bits) - math.Float32frombits(right.bits)
		return float32(math.Abs(float64(delta))) <= epsilon
	}
	return left.bits == right.bits
}
