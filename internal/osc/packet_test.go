package osc

import (
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
