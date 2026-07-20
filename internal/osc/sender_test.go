package osc

import (
	"errors"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

type testValueSource struct {
	floats map[parameters.ParameterID]float32
	bools  map[parameters.ParameterID]bool
}

func (source *testValueSource) Float(id parameters.ParameterID) (float32, bool) {
	value, ok := source.floats[id]
	return value, ok
}

func (source *testValueSource) Bool(id parameters.ParameterID) (bool, bool) {
	value, ok := source.bools[id]
	return value, ok
}

type recordingPacketSender struct {
	packets [][]byte
	calls   int
	failAt  int
}

func (sender *recordingPacketSender) Send(packet []byte) error {
	sender.calls++
	if sender.failAt > 0 && sender.calls == sender.failAt {
		return errors.New("injected send failure")
	}
	sender.packets = append(sender.packets, append([]byte(nil), packet...))
	return nil
}

func TestParameterSenderExecutesCompiledOutputsAndDetectsChanges(t *testing.T) {
	catalog := buildSenderTestCatalog(t, true)
	transport := &recordingPacketSender{}
	sender := newParameterSender(transport, SenderConfig{FloatEpsilon: 0.001, UseBundles: false})
	sender.SetCatalog(catalog)
	source := &testValueSource{
		floats: map[parameters.ParameterID]float32{0: 0.6},
		bools:  map[parameters.ParameterID]bool{1: true},
	}

	if err := sender.Send(source); err != nil {
		t.Fatal(err)
	}
	if got, want := len(transport.packets), len(catalog.Outputs); got != want {
		t.Fatalf("packets = %d, want %d", got, want)
	}
	values := decodedValuesByAddress(t, transport.packets)
	assertFloatValue(t, values["/a/Float"], 0.6)
	assertIntValue(t, values["/b/Float"], 1)
	assertBoolValue(t, values["/c/FloatNegative"], false)
	assertBoolValue(t, values["/c/Float1"], false)
	assertBoolValue(t, values["/c/Float2"], true)
	assertBoolValue(t, values["/f/Active"], true)

	transport.packets = nil
	if err := sender.Send(source); err != nil {
		t.Fatal(err)
	}
	if len(transport.packets) != 0 {
		t.Fatalf("unchanged send produced %d packets", len(transport.packets))
	}

	source.floats[0] = 0.6005
	if err := sender.Send(source); err != nil {
		t.Fatal(err)
	}
	if len(transport.packets) != 0 {
		t.Fatalf("epsilon-equivalent send produced %d packets", len(transport.packets))
	}

	source.floats[0] = 0.61
	if err := sender.Send(source); err != nil {
		t.Fatal(err)
	}
	if len(transport.packets) != 1 {
		t.Fatalf("changed send produced %d packets, want 1", len(transport.packets))
	}
}

func TestParameterSenderBundlesAndResetsChangeDetection(t *testing.T) {
	catalog := buildSenderTestCatalog(t, false)
	transport := &recordingPacketSender{}
	sender := newParameterSender(transport, SenderConfig{MaxDatagram: 1200, UseBundles: true})
	sender.SetCatalog(catalog)
	source := &testValueSource{
		floats: map[parameters.ParameterID]float32{0: -0.25},
		bools:  map[parameters.ParameterID]bool{1: false},
	}

	if err := sender.Send(source); err != nil {
		t.Fatal(err)
	}
	if len(transport.packets) != 1 {
		t.Fatalf("bundled send produced %d packets, want 1", len(transport.packets))
	}
	if got := len(decodedValuesByAddress(t, transport.packets)); got != len(catalog.Outputs) {
		t.Fatalf("decoded values = %d, want %d", got, len(catalog.Outputs))
	}

	transport.packets = nil
	sender.ResetChangeDetection()
	if err := sender.Send(source); err != nil {
		t.Fatal(err)
	}
	if len(transport.packets) != 1 {
		t.Fatalf("reset send produced %d packets, want 1", len(transport.packets))
	}
}

func TestParameterSenderRetriesAfterTransportFailure(t *testing.T) {
	catalog := buildSenderTestCatalog(t, false)
	transport := &recordingPacketSender{failAt: 1}
	sender := newParameterSender(transport, SenderConfig{UseBundles: true})
	sender.SetCatalog(catalog)
	source := &testValueSource{
		floats: map[parameters.ParameterID]float32{0: 0.25},
		bools:  map[parameters.ParameterID]bool{1: true},
	}

	if err := sender.Send(source); err == nil {
		t.Fatal("send failure was not returned")
	}
	transport.failAt = 0
	if err := sender.Send(source); err != nil {
		t.Fatal(err)
	}
	if len(transport.packets) != 1 {
		t.Fatalf("retry produced %d packets, want 1", len(transport.packets))
	}
	if got := len(decodedValuesByAddress(t, transport.packets)); got != len(catalog.Outputs) {
		t.Fatalf("retry decoded %d outputs, want %d", got, len(catalog.Outputs))
	}
}

func TestParameterSenderCatalogChangeInvalidatesCache(t *testing.T) {
	transport := &recordingPacketSender{}
	sender := newParameterSender(transport, SenderConfig{UseBundles: true})
	source := &testValueSource{
		floats: map[parameters.ParameterID]float32{0: 0.25},
		bools:  map[parameters.ParameterID]bool{1: true},
	}
	first := buildSenderTestCatalog(t, false)
	sender.SetCatalog(first)
	if err := sender.Send(source); err != nil {
		t.Fatal(err)
	}

	transport.packets = nil
	second := buildSenderTestCatalog(t, true)
	sender.SetCatalog(second)
	if err := sender.Send(source); err != nil {
		t.Fatal(err)
	}
	if got := len(decodedValuesByAddress(t, transport.packets)); got != len(second.Outputs) {
		t.Fatalf("catalog change sent %d outputs, want %d", got, len(second.Outputs))
	}
}

func buildSenderTestCatalog(t testing.TB, includeBinary bool) *Catalog {
	t.Helper()
	definitions := []parameters.ParameterDefinition{
		{
			ID: 0, OSCName: "Float", ValueType: parameters.ValueFloat,
			Encodings: parameters.EncodingFloat | parameters.EncodingBinary,
			Range:     parameters.ValueRange{Min: -1, Max: 1}, HasRange: true,
		},
		{
			ID: 1, OSCName: "Active", ValueType: parameters.ValueBool,
			Encodings: parameters.EncodingBool,
		},
	}
	specs, err := NewParameterCatalog(definitions)
	if err != nil {
		t.Fatal(err)
	}
	root := NewQueryRoot()
	paths := []struct{ address, typ string }{
		{address: "/a/Float", typ: "f"},
		{address: "/b/Float", typ: "i"},
		{address: "/f/Active", typ: "T"},
	}
	if includeBinary {
		paths = append(paths,
			struct{ address, typ string }{address: "/c/FloatNegative", typ: "T"},
			struct{ address, typ string }{address: "/c/Float1", typ: "T"},
			struct{ address, typ string }{address: "/c/Float2", typ: "T"},
		)
	}
	for _, path := range paths {
		if err := root.Add(NewMethod(path.address, path.typ, AccessWriteOnly)); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := BuildCatalog(root, specs, 1)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func decodedValuesByAddress(t *testing.T, packets [][]byte) map[string]Value {
	t.Helper()
	result := make(map[string]Value)
	for _, packet := range packets {
		messages, err := UnmarshalPacket(packet)
		if err != nil {
			t.Fatal(err)
		}
		for _, message := range messages {
			if len(message.Args) != 1 {
				t.Fatalf("message %s has %d args", message.Address, len(message.Args))
			}
			result[message.Address] = message.Args[0]
		}
	}
	return result
}

func assertFloatValue(t *testing.T, value Value, want float32) {
	t.Helper()
	if value.Kind != ValueFloat32 || value.F32 != want {
		t.Fatalf("value = %#v, want float %v", value, want)
	}
}

func assertIntValue(t *testing.T, value Value, want int32) {
	t.Helper()
	if value.Kind != ValueInt32 || value.I32 != want {
		t.Fatalf("value = %#v, want int %v", value, want)
	}
}

func assertBoolValue(t *testing.T, value Value, want bool) {
	t.Helper()
	if value.Kind != ValueBool || value.Bool != want {
		t.Fatalf("value = %#v, want bool %v", value, want)
	}
}
