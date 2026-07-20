package osc

import (
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

var benchmarkPacketLength int

type discardPacketSender struct{}

func (discardPacketSender) Send(packet []byte) error {
	benchmarkPacketLength = len(packet)
	return nil
}

func BenchmarkMarshalScalarMessage(b *testing.B) {
	message := Message{
		Address: "/avatar/parameters/v2/JawX",
		Args:    []Value{Float32(0.25)},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		packet, err := MarshalMessage(message)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkPacketLength = len(packet)
	}
}

func BenchmarkMessageBuilderScalar(b *testing.B) {
	var builder messageBuilder
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		packet, err := builder.encodeScalar("/avatar/parameters/v2/JawX", floatScalar(0.25))
		if err != nil {
			b.Fatal(err)
		}
		benchmarkPacketLength = len(packet)
	}
}

func BenchmarkMarshalScalarBundle(b *testing.B) {
	floatMessage := Message{
		Address: "/avatar/parameters/v2/JawX",
		Args:    []Value{Float32(0.25)},
	}
	boolMessage := Message{
		Address: "/avatar/parameters/EyeTrackingActive",
		Args:    []Value{Bool(true)},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		first, err := MarshalMessage(floatMessage)
		if err != nil {
			b.Fatal(err)
		}
		second, err := MarshalMessage(boolMessage)
		if err != nil {
			b.Fatal(err)
		}
		packet, err := MarshalBundle(Bundle{Timetag: 1, Elements: [][]byte{first, second}})
		if err != nil {
			b.Fatal(err)
		}
		benchmarkPacketLength = len(packet)
	}
}

func BenchmarkBundleBuilderScalars(b *testing.B) {
	builder := newBundleBuilder(1200)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		builder.reset()
		if ok, err := builder.appendScalar("/avatar/parameters/v2/JawX", floatScalar(0.25)); err != nil || !ok {
			b.Fatalf("append float = %v, %v", ok, err)
		}
		if ok, err := builder.appendScalar("/avatar/parameters/EyeTrackingActive", boolScalar(true)); err != nil || !ok {
			b.Fatalf("append bool = %v, %v", ok, err)
		}
		benchmarkPacketLength = len(builder.bytes())
	}
}

func BenchmarkParameterSenderUnchangedFrame(b *testing.B) {
	catalog := buildSenderTestCatalog(b, true)
	sender := newParameterSender(discardPacketSender{}, SenderConfig{UseBundles: true})
	sender.SetCatalog(catalog)
	source := &testValueSource{
		floats: map[parameters.ParameterID]float32{0: 0.25},
		bools:  map[parameters.ParameterID]bool{1: true},
	}
	if err := sender.Send(source); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := sender.Send(source); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParameterSenderChangedFrame(b *testing.B) {
	catalog := buildSenderTestCatalog(b, true)
	sender := newParameterSender(discardPacketSender{}, SenderConfig{UseBundles: true})
	sender.SetCatalog(catalog)
	source := &testValueSource{
		floats: map[parameters.ParameterID]float32{0: 0.25},
		bools:  map[parameters.ParameterID]bool{1: true},
	}
	if err := sender.Send(source); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := range b.N {
		if index&1 == 0 {
			source.floats[0] = 0.5
		} else {
			source.floats[0] = 0.25
		}
		if err := sender.Send(source); err != nil {
			b.Fatal(err)
		}
	}
}
