package picoconnect

import (
	"encoding/binary"
	"errors"
	"math"
)

const shapeCount = 52

var errInvalidLegacyPacket = errors.New("invalid PICO Connect legacy face packet")

type shapePacket struct {
	values          [shapeCount]float32
	present         [shapeCount]bool
	sourceTimestamp uint64
}

// parseLegacyPacket decodes the 16-byte-header PICO Connect format used by
// faceTrackingTransferProtocol=2. The first 892 body bytes are significant.
func parseLegacyPacket(data []byte) (shapePacket, error) {
	var packet shapePacket
	if len(data) < 908 || data[2] != 2 {
		return packet, errInvalidLegacyPacket
	}
	packet.sourceTimestamp = binary.LittleEndian.Uint64(data[16:24])
	for index := range packet.values {
		value := math.Float32frombits(binary.LittleEndian.Uint32(data[24+index*4:]))
		if !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) {
			packet.values[index] = value
			packet.present[index] = true
		}
	}
	return packet, nil
}
