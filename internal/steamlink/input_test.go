package steamlink

import (
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/osc"
)

func TestInputAcceptsKnownMessagesInWireOrder(t *testing.T) {
	gaze := mustMarshalInput(t, osc.Message{
		Address: "/sl/eyeTrackedGazePoint",
		Args:    []osc.Value{osc.Float32(0.25), osc.Float32(-0.5), osc.Float32(-1)},
	})
	first := mustMarshalInput(t, osc.Message{
		Address: "/sl/xrfb/facew/JawDrop",
		Args:    []osc.Value{osc.Float32(0.2)},
	})
	second := mustMarshalInput(t, osc.Message{
		Address: "/sl/xrfb/facew/JawDrop",
		Args:    []osc.Value{osc.Float32(0.8)},
	})
	packet := mustMarshalBundleInput(t, gaze, first, second)

	observations, report, err := decodeDatagram(packet)
	if err != nil {
		t.Fatal(err)
	}
	if report.InvalidMessages != 0 || report.UnknownMessages != 0 || len(report.UnknownAddresses) != 0 {
		t.Fatalf("report = %#v, want no diagnostics", report)
	}
	if len(observations) != 3 {
		t.Fatalf("observations = %#v, want 3", observations)
	}
	if observations[0] != (observation{ID: rawGazePoint, Values: [3]float32{0.25, -0.5, -1}}) {
		t.Fatalf("gaze observation = %#v", observations[0])
	}
	if observations[1] != (observation{ID: rawJawDrop, Values: [3]float32{0.2}}) {
		t.Fatalf("first observation = %#v", observations[1])
	}
	if observations[2] != (observation{ID: rawJawDrop, Values: [3]float32{0.8}}) {
		t.Fatalf("second observation = %#v", observations[2])
	}
}

func TestInputRejectsUnsupportedSibling(t *testing.T) {
	good, err := osc.MarshalMessage(osc.Message{
		Address: "/sl/xrfb/facew/JawDrop", Args: []osc.Value{osc.Float32(0.5)},
	})
	if err != nil {
		t.Fatal(err)
	}
	blob := []byte{'/', 'x', 0, 0, ',', 'b', 0, 0, 0, 0, 0, 0}
	packet := make([]byte, 16)
	copy(packet, "#bundle\x00")
	binary.BigEndian.PutUint64(packet[8:], 1)
	for _, element := range [][]byte{good, blob} {
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], uint32(len(element)))
		packet = append(packet, size[:]...)
		packet = append(packet, element...)
	}
	observations, _, err := decodeDatagram(packet)
	if !errors.Is(err, osc.ErrUnsupportedType) || len(observations) != 0 {
		t.Fatalf("got %v, %v; want whole-packet rejection", observations, err)
	}
}

func TestInputRejectsCodecFailuresWithoutObservations(t *testing.T) {
	valid := mustMarshalInput(t, osc.Message{
		Address: "/sl/xrfb/facew/JawDrop", Args: []osc.Value{osc.Float32(0.5)},
	})
	for _, packet := range [][]byte{
		append(append([]byte(nil), valid...), 0),
		func() []byte {
			packet := append([]byte(nil), valid...)
			packet[len("/sl/xrfb/facew/JawDrop")+1] = 1
			return packet
		}(),
	} {
		observations, _, err := decodeDatagram(packet)
		if !errors.Is(err, osc.ErrMalformedPacket) || len(observations) != 0 {
			t.Fatalf("got %v, %v; want malformed whole-packet rejection", observations, err)
		}
	}
}

func TestInputAcceptsEmptyBundle(t *testing.T) {
	packet := mustMarshalBundleInput(t)
	observations, report, err := decodeDatagram(packet)
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 0 || report.InvalidMessages != 0 || report.UnknownMessages != 0 || len(report.UnknownAddresses) != 0 {
		t.Fatalf("got %#v, %#v; want empty result", observations, report)
	}
}

func TestInputLimitsPacketMessagesAndAddresses(t *testing.T) {
	t.Run("payload", func(t *testing.T) {
		observations, _, err := decodeDatagram(make([]byte, 65508))
		if !errors.Is(err, errPacketLimit) || len(observations) != 0 {
			t.Fatalf("got %v, %v; want packet limit", observations, err)
		}
	})
	t.Run("messages", func(t *testing.T) {
		message := mustMarshalInput(t, osc.Message{Address: "/unknown", Args: []osc.Value{osc.Float32(0)}})
		elements := make([][]byte, 513)
		for index := range elements {
			elements[index] = message
		}
		observations, _, err := decodeDatagram(mustMarshalBundleInput(t, elements...))
		if !errors.Is(err, errPacketLimit) || len(observations) != 0 {
			t.Fatalf("got %v, %v; want message limit", observations, err)
		}
	})
	t.Run("address", func(t *testing.T) {
		address := "/" + strings.Repeat("x", 256)
		packet := mustMarshalInput(t, osc.Message{Address: address, Args: []osc.Value{osc.Float32(0)}})
		observations, _, err := decodeDatagram(packet)
		if !errors.Is(err, errPacketLimit) || len(observations) != 0 {
			t.Fatalf("got %v, %v; want address limit", observations, err)
		}
	})
}

func TestInputReportsUnknownSupportedAddressesWithinBound(t *testing.T) {
	elements := make([][]byte, 33)
	for index := range elements {
		elements[index] = mustMarshalInput(t, osc.Message{
			Address: "/unknown/" + string(rune('a'+index)),
			Args:    []osc.Value{osc.String("not a tracking field")},
		})
	}

	observations, report, err := decodeDatagram(mustMarshalBundleInput(t, elements...))
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 0 || report.InvalidMessages != 0 || report.UnknownMessages != 33 {
		t.Fatalf("got %#v, %#v", observations, report)
	}
	if len(report.UnknownAddresses) != 32 || report.UnknownAddresses[0] != "/unknown/a" {
		t.Fatalf("unknown addresses = %#v", report.UnknownAddresses)
	}
}

func TestInputKeepsValidSiblingAndReportsInvalidKnownMessage(t *testing.T) {
	good := mustMarshalInput(t, osc.Message{
		Address: "/sl/xrfb/facew/JawDrop", Args: []osc.Value{osc.Float32(0.5)},
	})
	wrongType := mustMarshalInput(t, osc.Message{
		Address: "/sl/xrfb/facew/JawDrop", Args: []osc.Value{osc.Int32(1)},
	})
	wrongArity := mustMarshalInput(t, osc.Message{
		Address: "/sl/xrfb/facew/JawDrop", Args: []osc.Value{osc.Float32(0.2), osc.Float32(0.3)},
	})

	observations, report, err := decodeDatagram(mustMarshalBundleInput(t, good, wrongType, wrongArity))
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 || observations[0].ID != rawJawDrop || report.InvalidMessages != 2 {
		t.Fatalf("got %#v, %#v", observations, report)
	}
}

func TestInputRejectsInvalidGaze(t *testing.T) {
	tests := []struct {
		name string
		args []osc.Value
	}{
		{name: "arity", args: []osc.Value{osc.Float32(0), osc.Float32(0)}},
		{name: "positive z", args: []osc.Value{osc.Float32(0), osc.Float32(0), osc.Float32(0)}},
		{name: "nan", args: []osc.Value{osc.Float32(float32(math.NaN())), osc.Float32(0), osc.Float32(-1)}},
		{name: "infinity", args: []osc.Value{osc.Float32(0), osc.Float32(float32(math.Inf(1))), osc.Float32(-1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet := mustMarshalInput(t, osc.Message{Address: "/sl/eyeTrackedGazePoint", Args: tt.args})
			observations, report, err := decodeDatagram(packet)
			if err != nil {
				t.Fatal(err)
			}
			if len(observations) != 0 || report.InvalidMessages != 1 {
				t.Fatalf("got %#v, %#v", observations, report)
			}
		})
	}
}

func TestInputRejectsInvalidWeights(t *testing.T) {
	tests := []float32{float32(math.NaN()), float32(math.Inf(1)), -0.01, 1.01}
	for _, value := range tests {
		packet := mustMarshalInput(t, osc.Message{
			Address: "/sl/xrfb/facew/JawDrop", Args: []osc.Value{osc.Float32(value)},
		})
		observations, report, err := decodeDatagram(packet)
		if err != nil {
			t.Fatal(err)
		}
		if len(observations) != 0 || report.InvalidMessages != 1 {
			t.Fatalf("value %v got %#v, %#v", value, observations, report)
		}
	}
}

func mustMarshalInput(t *testing.T, message osc.Message) []byte {
	t.Helper()
	packet, err := osc.MarshalMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	return packet
}

func mustMarshalBundleInput(t *testing.T, elements ...[]byte) []byte {
	t.Helper()
	packet, err := osc.MarshalBundle(osc.Bundle{Elements: elements})
	if err != nil {
		t.Fatal(err)
	}
	return packet
}
