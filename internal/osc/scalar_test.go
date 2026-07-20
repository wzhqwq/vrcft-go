package osc

import (
	"testing"
	"unsafe"
)

func TestScalarValueLayoutAndTags(t *testing.T) {
	if got := unsafe.Sizeof(scalarValue{}); got != 8 {
		t.Fatalf("sizeof(scalarValue) = %d, want 8", got)
	}

	tests := []struct {
		name  string
		value scalarValue
		tag   byte
	}{
		{name: "float", value: floatScalar(0.25), tag: 'f'},
		{name: "int", value: intScalar(-4), tag: 'i'},
		{name: "false", value: boolScalar(false), tag: 'F'},
		{name: "true", value: boolScalar(true), tag: 'T'},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.value.typeTag(); got != test.tag {
				t.Fatalf("typeTag() = %q, want %q", got, test.tag)
			}
		})
	}
}

func TestScalarEqual(t *testing.T) {
	if !scalarEqual(floatScalar(1), floatScalar(1.0005), 0.001) {
		t.Fatal("floats within epsilon differ")
	}
	if scalarEqual(floatScalar(1), floatScalar(1.002), 0.001) {
		t.Fatal("floats outside epsilon compare equal")
	}
	if !scalarEqual(intScalar(-4), intScalar(-4), 0.001) {
		t.Fatal("equal ints differ")
	}
	if scalarEqual(boolScalar(false), boolScalar(true), 0.001) {
		t.Fatal("different bools compare equal")
	}
	if scalarEqual(intScalar(1), floatScalar(1), 0.001) {
		t.Fatal("different scalar kinds compare equal")
	}
}
