package osc_test

import (
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/wzhqwq/vrcft-go/pkg/osc"
)

func TestMessageRoundTrip(t *testing.T) {
	original := osc.Message{Address: "/plugin/status", Args: []osc.Value{
		osc.Int32(-4), osc.Float32(0.25), osc.String("ready"),
		osc.Bool(true), osc.Bool(false),
	}}
	packet, err := osc.MarshalMessage(original)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := osc.UnmarshalPacket(packet)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(messages, []osc.Message{original}) {
		t.Fatalf("messages = %#v, want %#v", messages, []osc.Message{original})
	}
}

func TestMarshalMessageExactWire(t *testing.T) {
	tests := []struct {
		name string
		arg  osc.Value
		want []byte
	}{
		{"int32", osc.Int32(-4), []byte{'/', 'x', 0, 0, ',', 'i', 0, 0, 0xff, 0xff, 0xff, 0xfc}},
		{"float32", osc.Float32(0.25), []byte{'/', 'x', 0, 0, ',', 'f', 0, 0, 0x3e, 0x80, 0, 0}},
		{"string", osc.String("hi"), []byte{'/', 'x', 0, 0, ',', 's', 0, 0, 'h', 'i', 0, 0}},
		{"true", osc.Bool(true), []byte{'/', 'x', 0, 0, ',', 'T', 0, 0}},
		{"false", osc.Bool(false), []byte{'/', 'x', 0, 0, ',', 'F', 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := osc.MarshalMessage(osc.Message{Address: "/x", Args: []osc.Value{tt.arg}})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("packet = %x, want %x", got, tt.want)
			}
		})
	}
}

func TestBundleRoundTripAndNestedOrder(t *testing.T) {
	one := mustMarshalMessage(t, "/one")
	two := mustMarshalMessage(t, "/two")
	three := mustMarshalMessage(t, "/three")
	nested := rawBundle(1, two, three)
	packet := rawBundle(1, one, nested)

	messages, err := osc.UnmarshalPacket(packet)
	if err != nil {
		t.Fatal(err)
	}
	var addresses []string
	for _, message := range messages {
		addresses = append(addresses, message.Address)
	}
	if want := []string{"/one", "/two", "/three"}; !reflect.DeepEqual(addresses, want) {
		t.Fatalf("addresses = %v, want %v", addresses, want)
	}
}

func TestMarshalBundleNormalizesZeroTimetag(t *testing.T) {
	packet, err := osc.MarshalBundle(osc.Bundle{Elements: [][]byte{mustMarshalMessage(t, "/one")}})
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.BigEndian.Uint64(packet[8:16]); got != 1 {
		t.Fatalf("timetag = %d, want 1", got)
	}
}

func TestNTPTimeEpoch(t *testing.T) {
	if got := osc.NTPTime(time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)); got != 0 {
		t.Fatalf("NTPTime(epoch) = %d, want 0", got)
	}
	if got := osc.NTPTime(time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)); got != uint64(2208988800)<<32 {
		t.Fatalf("NTPTime(unix epoch) = %d, want %d", got, uint64(2208988800)<<32)
	}
}

func TestValidAddress(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{"/valid", true},
		{"/avatar/parameters/Test", true},
		{"", false},
		{"relative", false},
		{"/contains\x00nul", false},
	}
	for _, tt := range tests {
		if got := osc.ValidAddress(tt.address); got != tt.want {
			t.Errorf("ValidAddress(%q) = %v, want %v", tt.address, got, tt.want)
		}
	}
}

func TestUnmarshalPacketRejectsMalformedInput(t *testing.T) {
	message := mustMarshalMessage(t, "/one")
	unsupported := append([]byte(nil), message...)
	unsupported[9] = 'b'
	nonzeroPadding := append([]byte(nil), message...)
	nonzeroPadding[5] = 1
	trailing := append(append([]byte(nil), message...), 0)
	zeroElement := append(rawBundle(1), 0, 0, 0, 0)
	truncatedElement := append(rawBundle(1), 0, 0, 0, 4, 0, 0)
	tooDeep := message
	for range 33 {
		tooDeep = rawBundle(1, tooDeep)
	}

	tests := []struct {
		name string
		in   []byte
		err  error
	}{
		{"short input", []byte{0, 1, 2}, osc.ErrMalformedPacket},
		{"relative address", []byte{'x', 0, 0, 0, ',', 0, 0, 0}, osc.ErrMalformedPacket},
		{"nonzero padding", nonzeroPadding, osc.ErrMalformedPacket},
		{"unsupported tag", unsupported, osc.ErrUnsupportedType},
		{"trailing bytes", trailing, osc.ErrMalformedPacket},
		{"zero bundle element size", zeroElement, osc.ErrMalformedPacket},
		{"truncated bundle element", truncatedElement, osc.ErrMalformedPacket},
		{"too deeply nested", tooDeep, osc.ErrMalformedPacket},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := osc.UnmarshalPacket(tt.in)
			if !errors.Is(err, tt.err) {
				t.Fatalf("UnmarshalPacket(%x) error = %v, want errors.Is(_, %v)", tt.in, err, tt.err)
			}
		})
	}
}

func TestMarshalRejectsInvalidInputs(t *testing.T) {
	message := mustMarshalMessage(t, "/one")
	tests := []struct {
		name string
		call func() error
		err  error
	}{
		{"invalid address", func() error { _, err := osc.MarshalMessage(osc.Message{Address: "relative"}); return err }, osc.ErrInvalidAddress},
		{"address NUL", func() error { _, err := osc.MarshalMessage(osc.Message{Address: "/bad\x00"}); return err }, osc.ErrInvalidAddress},
		{"string NUL", func() error {
			_, err := osc.MarshalMessage(osc.Message{Address: "/x", Args: []osc.Value{osc.String("bad\x00")}})
			return err
		}, osc.ErrInvalidArgument},
		{"unsupported value kind", func() error {
			_, err := osc.MarshalMessage(osc.Message{Address: "/x", Args: []osc.Value{{Kind: osc.ValueKind(255)}}})
			return err
		}, osc.ErrUnsupportedType},
		{"empty bundle element", func() error { _, err := osc.MarshalBundle(osc.Bundle{Elements: [][]byte{{}}}); return err }, osc.ErrMalformedPacket},
		{"invalid bundle element", func() error { _, err := osc.MarshalBundle(osc.Bundle{Elements: [][]byte{{0, 1, 2, 3}}}); return err }, osc.ErrMalformedPacket},
		{"oversized bundle element", func() error {
			if unsafe.Sizeof(uintptr(0)) < 8 {
				t.Skip("a slice larger than an OSC int32 length is not representable")
			}
			oneByte := byte(0)
			overflow := []byte{oneByte}
			header := (*[3]uintptr)(unsafe.Pointer(&overflow))
			header[1] = uintptr(math.MaxInt32) + 1
			header[2] = uintptr(math.MaxInt32) + 1
			_, err := osc.MarshalBundle(osc.Bundle{Elements: [][]byte{overflow}})
			runtime.KeepAlive(&oneByte)
			return err
		}, osc.ErrMalformedPacket},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if !errors.Is(err, tt.err) {
				t.Fatalf("error = %v, want errors.Is(_, %v)", err, tt.err)
			}
		})
	}

	element := append([]byte(nil), message...)
	before := append([]byte(nil), element...)
	if _, err := osc.MarshalBundle(osc.Bundle{Elements: [][]byte{element}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(element, before) {
		t.Fatalf("MarshalBundle mutated element: got %x, want %x", element, before)
	}
}

func TestUnmarshalPacketOwnsReturnedValues(t *testing.T) {
	packet := mustMarshalMessageWithString(t, "/address", "value")
	messages, err := osc.UnmarshalPacket(packet)
	if err != nil {
		t.Fatal(err)
	}
	packet[1] = 'X'
	packet[len(packet)-4] = 'X'
	if messages[0].Address != "/address" {
		t.Fatalf("Address = %q, want %q", messages[0].Address, "/address")
	}
	if messages[0].Args[0].Str != "value" {
		t.Fatalf("string = %q, want %q", messages[0].Args[0].Str, "value")
	}
	if len(messages[0].Args) != 1 {
		t.Fatalf("Args length = %d, want 1", len(messages[0].Args))
	}
}

func mustMarshalMessage(t *testing.T, address string) []byte {
	t.Helper()
	packet, err := osc.MarshalMessage(osc.Message{Address: address})
	if err != nil {
		t.Fatal(err)
	}
	return packet
}

func mustMarshalMessageWithString(t *testing.T, address, value string) []byte {
	t.Helper()
	packet, err := osc.MarshalMessage(osc.Message{Address: address, Args: []osc.Value{osc.String(value)}})
	if err != nil {
		t.Fatal(err)
	}
	return packet
}

func rawBundle(timetag uint64, elements ...[]byte) []byte {
	packet := make([]byte, 16)
	copy(packet, "#bundle\x00")
	binary.BigEndian.PutUint64(packet[8:], timetag)
	for _, element := range elements {
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], uint32(len(element)))
		packet = append(packet, size[:]...)
		packet = append(packet, element...)
	}
	return packet
}
