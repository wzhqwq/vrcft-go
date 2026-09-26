package steamlink

import (
	"errors"
	"math"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/osc"
)

func FuzzDecodeDatagram(f *testing.F) {
	valid, err := osc.MarshalMessage(osc.Message{
		Address: "/sl/eyeTrackedGazePoint",
		Args:    []osc.Value{osc.Float32(0), osc.Float32(0), osc.Float32(-1)},
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte{0})
	f.Add([]byte("#bundle\x00"))
	f.Add(make([]byte, 65508))

	f.Fuzz(func(t *testing.T, packet []byte) {
		observations, _, err := decodeDatagram(packet)
		if err != nil {
			if len(observations) != 0 {
				t.Fatalf("error %v returned observations %#v", err, observations)
			}
			if len(packet) > 65507 && !errors.Is(err, errPacketLimit) {
				t.Fatalf("oversized packet error = %v, want packet limit", err)
			}
			return
		}
		for _, observation := range observations {
			if observation.ID >= rawCount {
				t.Fatalf("observation ID %d is outside registry", observation.ID)
			}
			for _, value := range observation.Values {
				if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
					t.Fatalf("non-finite observation %#v", observation)
				}
			}
		}
	})
}
