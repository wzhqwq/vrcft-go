package osc

import (
	"bytes"
	"math"
	"testing"
)

func TestMessageRoundTrip(t *testing.T) {
	original := Message{
		Address: "/avatar/parameters/Test",
		Args: []Value{
			Float32(0.25),
			Int32(-4),
			Bool(true),
			String("hello"),
		},
	}
	packet, err := MarshalMessage(original)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := UnmarshalPacket(packet)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("got %d messages", len(messages))
	}
	got := messages[0]
	if got.Address != original.Address || len(got.Args) != len(original.Args) {
		t.Fatalf("unexpected message: %#v", got)
	}
	if math.Abs(float64(got.Args[0].F32-0.25)) > 1e-6 || got.Args[1].I32 != -4 || !got.Args[2].Bool || got.Args[3].Str != "hello" {
		t.Fatalf("unexpected values: %#v", got.Args)
	}
}

func TestBundleRoundTrip(t *testing.T) {
	one, _ := MarshalMessage(Message{Address: "/one", Args: []Value{Float32(1)}})
	two, _ := MarshalMessage(Message{Address: "/two", Args: []Value{Bool(false)}})
	packet, err := MarshalBundle(Bundle{Timetag: 1, Elements: [][]byte{one, two}})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := UnmarshalPacket(packet)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Address != "/one" || messages[1].Address != "/two" {
		t.Fatalf("unexpected bundle: %#v", messages)
	}
}

func TestScalarWireEncoding(t *testing.T) {
	tests := []struct {
		name   string
		value  Value
		scalar scalarValue
		want   []byte
	}{
		{
			name: "float", value: Float32(1), scalar: floatScalar(1),
			want: []byte{'/', 'x', 0, 0, ',', 'f', 0, 0, 0x3f, 0x80, 0, 0},
		},
		{
			name: "int", value: Int32(-4), scalar: intScalar(-4),
			want: []byte{'/', 'x', 0, 0, ',', 'i', 0, 0, 0xff, 0xff, 0xff, 0xfc},
		},
		{
			name: "true", value: Bool(true), scalar: boolScalar(true),
			want: []byte{'/', 'x', 0, 0, ',', 'T', 0, 0},
		},
		{
			name: "false", value: Bool(false), scalar: boolScalar(false),
			want: []byte{'/', 'x', 0, 0, ',', 'F', 0, 0},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			generic, err := MarshalMessage(Message{Address: "/x", Args: []Value{test.value}})
			if err != nil {
				t.Fatal(err)
			}
			var builder messageBuilder
			optimized, err := builder.encodeScalar("/x", test.scalar)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(generic, test.want) {
				t.Fatalf("generic = %x, want %x", generic, test.want)
			}
			if !bytes.Equal(optimized, test.want) {
				t.Fatalf("optimized = %x, want %x", optimized, test.want)
			}
		})
	}
}

func TestImmediateBundleWireEncoding(t *testing.T) {
	message := []byte{'/', 'x', 0, 0, ',', 'T', 0, 0}
	want := []byte{
		'#', 'b', 'u', 'n', 'd', 'l', 'e', 0,
		0, 0, 0, 0, 0, 0, 0, 1,
		0, 0, 0, 8,
		'/', 'x', 0, 0, ',', 'T', 0, 0,
	}
	generic, err := MarshalBundle(Bundle{Timetag: 1, Elements: [][]byte{message}})
	if err != nil {
		t.Fatal(err)
	}
	builder := newBundleBuilder(len(want))
	if ok, err := builder.appendScalar("/x", boolScalar(true)); err != nil || !ok {
		t.Fatalf("appendScalar = %v, %v", ok, err)
	}
	if !bytes.Equal(generic, want) {
		t.Fatalf("generic = %x, want %x", generic, want)
	}
	if !bytes.Equal(builder.bytes(), want) {
		t.Fatalf("optimized = %x, want %x", builder.bytes(), want)
	}
}
