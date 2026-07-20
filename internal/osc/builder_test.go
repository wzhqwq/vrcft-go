package osc

import (
	"bytes"
	"strings"
	"testing"
)

func TestMessageBuilderMatchesMarshalMessage(t *testing.T) {
	tests := []struct {
		address string
		scalar  scalarValue
		value   Value
	}{
		{address: "/float", scalar: floatScalar(0.25), value: Float32(0.25)},
		{address: "/int", scalar: intScalar(-4), value: Int32(-4)},
		{address: "/false", scalar: boolScalar(false), value: Bool(false)},
		{address: "/true", scalar: boolScalar(true), value: Bool(true)},
	}

	var builder messageBuilder
	for _, test := range tests {
		got, err := builder.encodeScalar(test.address, test.scalar)
		if err != nil {
			t.Fatal(err)
		}
		want, err := MarshalMessage(Message{Address: test.address, Args: []Value{test.value}})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("packet for %s = %x, want %x", test.address, got, want)
		}
	}
}

func TestBundleBuilderFramesAndLimitsElements(t *testing.T) {
	first, err := MarshalMessage(Message{Address: "/one", Args: []Value{Float32(1)}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalMessage(Message{Address: "/two", Args: []Value{Bool(false)}})
	if err != nil {
		t.Fatal(err)
	}
	maxDatagram := 16 + 4 + len(first) + 4 + len(second)
	builder := newBundleBuilder(maxDatagram)
	if !builder.empty() {
		t.Fatal("new bundle is not empty")
	}
	if ok, err := builder.appendScalar("/one", floatScalar(1)); err != nil || !ok {
		t.Fatalf("append first = %v, %v", ok, err)
	}
	if ok, err := builder.appendScalar("/two", boolScalar(false)); err != nil || !ok {
		t.Fatalf("append second = %v, %v", ok, err)
	}
	want, err := MarshalBundle(Bundle{Timetag: 1, Elements: [][]byte{first, second}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(builder.bytes(), want) {
		t.Fatalf("bundle = %x, want %x", builder.bytes(), want)
	}
	before := append([]byte(nil), builder.bytes()...)
	if ok, err := builder.appendScalar("/three", intScalar(3)); err != nil || ok {
		t.Fatalf("over-limit append = %v, %v", ok, err)
	}
	if !bytes.Equal(builder.bytes(), before) {
		t.Fatal("failed append mutated bundle")
	}
	builder.reset()
	if !builder.empty() || len(builder.bytes()) != 16 {
		t.Fatalf("reset bundle has length %d", len(builder.bytes()))
	}
}

func TestBuildersRejectInvalidInput(t *testing.T) {
	var message messageBuilder
	if _, err := message.encodeScalar("invalid", floatScalar(1)); err == nil {
		t.Fatal("invalid address accepted")
	}
	invalid := scalarValue{kind: 255}
	if _, err := message.encodeScalar("/valid", invalid); err == nil {
		t.Fatal("invalid scalar accepted")
	}
	bundle := newBundleBuilder(1200)
	if _, err := bundle.appendScalar("/"+strings.Repeat("x", 1201), invalid); err == nil {
		t.Fatal("invalid scalar accepted by bundle")
	}
}
