package osc

import (
	"encoding/binary"
	"fmt"
)

const immediateBundleHeaderSize = 16

type messageBuilder struct {
	buffer []byte
}

func (builder *messageBuilder) encodeScalar(address string, value scalarValue) ([]byte, error) {
	packet, err := appendScalarMessage(builder.buffer[:0], address, value)
	if err != nil {
		return nil, err
	}
	builder.buffer = packet
	return packet, nil
}

type bundleBuilder struct {
	buffer      []byte
	maxDatagram int
}

func newBundleBuilder(maxDatagram int) bundleBuilder {
	capacity := maxDatagram
	if capacity < immediateBundleHeaderSize {
		capacity = immediateBundleHeaderSize
	}
	builder := bundleBuilder{
		buffer:      make([]byte, 0, capacity),
		maxDatagram: maxDatagram,
	}
	builder.reset()
	return builder
}

func (builder *bundleBuilder) reset() {
	builder.buffer = builder.buffer[:0]
	builder.buffer = append(builder.buffer, "#bundle\x00"...)
	builder.buffer = binary.BigEndian.AppendUint64(builder.buffer, 1)
}

func (builder *bundleBuilder) appendScalar(address string, value scalarValue) (bool, error) {
	messageSize, err := scalarMessageSize(address, value)
	if err != nil {
		return false, err
	}
	if len(builder.buffer)+4+messageSize > builder.maxDatagram {
		return false, nil
	}

	sizeOffset := len(builder.buffer)
	builder.buffer = append(builder.buffer, 0, 0, 0, 0)
	messageStart := len(builder.buffer)
	builder.buffer, err = appendScalarMessage(builder.buffer, address, value)
	if err != nil {
		builder.buffer = builder.buffer[:sizeOffset]
		return false, err
	}
	binary.BigEndian.PutUint32(
		builder.buffer[sizeOffset:sizeOffset+4],
		uint32(len(builder.buffer)-messageStart),
	)
	return true, nil
}

func (builder *bundleBuilder) bytes() []byte {
	return builder.buffer
}

func (builder *bundleBuilder) empty() bool {
	return len(builder.buffer) == immediateBundleHeaderSize
}

func appendScalarMessage(buffer []byte, address string, value scalarValue) ([]byte, error) {
	if !validAddress(address) {
		return nil, fmt.Errorf("invalid OSC address %q", address)
	}
	if value.typeTag() == 0 {
		return nil, fmt.Errorf("%w: scalar kind %d", ErrUnsupportedType, value.kind)
	}

	buffer = appendPaddedString(buffer, address)
	buffer = append(buffer, ',', value.typeTag(), 0, 0)
	switch value.kind {
	case scalarFloat32, scalarInt32:
		buffer = binary.BigEndian.AppendUint32(buffer, value.bits)
	case scalarFalse, scalarTrue:
		// OSC boolean type tags carry no payload bytes.
	}
	return buffer, nil
}

func scalarMessageSize(address string, value scalarValue) (int, error) {
	if !validAddress(address) {
		return 0, fmt.Errorf("invalid OSC address %q", address)
	}
	if value.typeTag() == 0 {
		return 0, fmt.Errorf("%w: scalar kind %d", ErrUnsupportedType, value.kind)
	}
	size := paddedStringSize(address) + 4
	if value.kind == scalarFloat32 || value.kind == scalarInt32 {
		size += 4
	}
	return size, nil
}

func appendPaddedString(buffer []byte, value string) []byte {
	buffer = append(buffer, value...)
	buffer = append(buffer, 0)
	for len(buffer)%4 != 0 {
		buffer = append(buffer, 0)
	}
	return buffer
}

func paddedStringSize(value string) int {
	return (len(value) + 4) &^ 3
}
