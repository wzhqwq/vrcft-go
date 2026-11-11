package picoconnect

import (
	"encoding/binary"
	"math"
	"testing"
)

func legacyFixture(size int) []byte {
	p := make([]byte, size)
	p[2] = 2
	binary.LittleEndian.PutUint64(p[16:], 123)
	binary.LittleEndian.PutUint32(p[24+7*4:], math.Float32bits(0.75))
	return p
}

func TestParseLegacyPacket(t *testing.T) {
	for _, size := range []int{908, 912} {
		got, err := parseLegacyPacket(legacyFixture(size))
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if got.values[7] != 0.75 || !got.present[7] || got.sourceTimestamp != 123 {
			t.Fatalf("size %d: got %+v", size, got)
		}
	}
	if _, err := parseLegacyPacket(legacyFixture(907)); err == nil {
		t.Fatal("short packet accepted")
	}
	wrong := legacyFixture(908)
	wrong[2] = 1
	if _, err := parseLegacyPacket(wrong); err == nil {
		t.Fatal("wrong tracking type accepted")
	}
	nonfinite := legacyFixture(908)
	binary.LittleEndian.PutUint32(nonfinite[24+7*4:], math.Float32bits(float32(math.NaN())))
	got, err := parseLegacyPacket(nonfinite)
	if err != nil || got.present[7] {
		t.Fatalf("nonfinite slot: present=%t err=%v", got.present[7], err)
	}
}
