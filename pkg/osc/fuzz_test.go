package osc_test

import (
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/osc"
)

func FuzzUnmarshalPacket(f *testing.F) {
	valid, err := osc.MarshalMessage(osc.Message{Address: "/seed", Args: []osc.Value{
		osc.Int32(1), osc.Float32(0.5), osc.String("ok"), osc.Bool(true),
	}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte("#bundle\x00\x00\x00\x00\x00\x00\x00\x00\x01"))
	f.Add([]byte{0, 1, 2, 3})
	f.Fuzz(func(t *testing.T, packet []byte) {
		_, _ = osc.UnmarshalPacket(packet)
	})
}
