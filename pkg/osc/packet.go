package osc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var (
	ErrMalformedPacket = errors.New("malformed OSC packet")
	ErrUnsupportedType = errors.New("unsupported OSC type")
	ErrInvalidAddress  = errors.New("invalid OSC address")
	ErrInvalidArgument = errors.New("invalid OSC argument")
	ErrServerRunning   = errors.New("OSC server is already serving")
	ErrServerClosed    = errors.New("OSC server is closed")
)

const maxBundleDepth = 32

// ValueKind identifies one of the OSC argument types supported by this package.
type ValueKind uint8

const (
	ValueInt32 ValueKind = iota + 1
	ValueFloat32
	ValueString
	ValueBool
)

// Value is an OSC argument. Only the field associated with Kind is used.
type Value struct {
	Kind ValueKind
	I32  int32
	F32  float32
	Str  string
	Bool bool
}

// Int32 returns an OSC int32 value.
func Int32(v int32) Value { return Value{Kind: ValueInt32, I32: v} }

// Float32 returns an OSC float32 value.
func Float32(v float32) Value { return Value{Kind: ValueFloat32, F32: v} }

// String returns an OSC string value.
func String(v string) Value { return Value{Kind: ValueString, Str: v} }

// Bool returns an OSC boolean value.
func Bool(v bool) Value { return Value{Kind: ValueBool, Bool: v} }

// Message is an OSC address and its ordered arguments.
type Message struct {
	Address string
	Args    []Value
}

// Bundle is an OSC bundle represented by its raw, encoded packet elements.
// Timetag zero is encoded as the OSC immediate timetag one.
type Bundle struct {
	Timetag  uint64
	Elements [][]byte
}

// MarshalMessage encodes a supported OSC message.
func MarshalMessage(m Message) ([]byte, error) {
	if !ValidAddress(m.Address) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidAddress, m.Address)
	}

	var tags strings.Builder
	tags.Grow(len(m.Args) + 1)
	tags.WriteByte(',')
	for _, arg := range m.Args {
		switch arg.Kind {
		case ValueInt32:
			tags.WriteByte('i')
		case ValueFloat32:
			tags.WriteByte('f')
		case ValueString:
			if strings.ContainsRune(arg.Str, '\x00') {
				return nil, fmt.Errorf("%w: OSC string contains NUL", ErrInvalidArgument)
			}
			tags.WriteByte('s')
		case ValueBool:
			if arg.Bool {
				tags.WriteByte('T')
			} else {
				tags.WriteByte('F')
			}
		default:
			return nil, fmt.Errorf("%w: value kind %d", ErrUnsupportedType, arg.Kind)
		}
	}

	var out bytes.Buffer
	writePaddedString(&out, m.Address)
	writePaddedString(&out, tags.String())
	for _, arg := range m.Args {
		switch arg.Kind {
		case ValueInt32:
			var data [4]byte
			binary.BigEndian.PutUint32(data[:], uint32(arg.I32))
			out.Write(data[:])
		case ValueFloat32:
			var data [4]byte
			binary.BigEndian.PutUint32(data[:], math.Float32bits(arg.F32))
			out.Write(data[:])
		case ValueString:
			writePaddedString(&out, arg.Str)
		case ValueBool:
			// OSC T/F type tags have no payload bytes.
		}
	}
	return out.Bytes(), nil
}

// MarshalBundle encodes a bundle after validating every raw packet element.
func MarshalBundle(b Bundle) ([]byte, error) {
	var out bytes.Buffer
	out.WriteString("#bundle\x00")
	timetag := b.Timetag
	if timetag == 0 {
		timetag = 1
	}
	var tag [8]byte
	binary.BigEndian.PutUint64(tag[:], timetag)
	out.Write(tag[:])

	for _, element := range b.Elements {
		if len(element) == 0 || len(element) > math.MaxInt32 {
			return nil, ErrMalformedPacket
		}
		if _, err := UnmarshalPacket(element); err != nil {
			return nil, err
		}
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], uint32(len(element)))
		out.Write(size[:])
		out.Write(element)
	}
	return out.Bytes(), nil
}

// NTPTime converts a wall-clock time to an OSC/NTP timetag.
func NTPTime(t time.Time) uint64 {
	const ntpToUnix = 2_208_988_800
	seconds := uint64(t.Unix() + ntpToUnix)
	fraction := uint64((uint64(t.Nanosecond()) << 32) / 1_000_000_000)
	return seconds<<32 | fraction
}

// UnmarshalPacket decodes an OSC message or bundle into messages in wire order.
func UnmarshalPacket(packet []byte) ([]Message, error) {
	return unmarshalPacket(packet, 0)
}

func unmarshalPacket(packet []byte, depth int) ([]Message, error) {
	if depth > maxBundleDepth || len(packet) < 4 {
		return nil, ErrMalformedPacket
	}
	if bytes.HasPrefix(packet, []byte("#bundle\x00")) {
		return unmarshalBundle(packet, depth)
	}
	message, consumed, err := unmarshalMessage(packet)
	if err != nil {
		return nil, err
	}
	if consumed != len(packet) {
		return nil, ErrMalformedPacket
	}
	return []Message{message}, nil
}

func unmarshalBundle(packet []byte, depth int) ([]Message, error) {
	if len(packet) < 16 || !bytes.Equal(packet[:8], []byte("#bundle\x00")) {
		return nil, ErrMalformedPacket
	}

	offset := 16
	var messages []Message
	for offset < len(packet) {
		if offset+4 > len(packet) {
			return nil, ErrMalformedPacket
		}
		size := int(int32(binary.BigEndian.Uint32(packet[offset : offset+4])))
		offset += 4
		if size <= 0 || size > len(packet)-offset {
			return nil, ErrMalformedPacket
		}
		decoded, err := unmarshalPacket(packet[offset:offset+size], depth+1)
		if err != nil {
			return nil, err
		}
		messages = append(messages, decoded...)
		offset += size
	}
	return messages, nil
}

func unmarshalMessage(packet []byte) (Message, int, error) {
	address, offset, err := readPaddedString(packet, 0)
	if err != nil || !ValidAddress(address) {
		return Message{}, 0, ErrMalformedPacket
	}
	tags, offset, err := readPaddedString(packet, offset)
	if err != nil || len(tags) == 0 || tags[0] != ',' {
		return Message{}, 0, ErrMalformedPacket
	}

	message := Message{Address: address, Args: make([]Value, 0, len(tags)-1)}
	for _, tag := range tags[1:] {
		switch tag {
		case 'i':
			if len(packet)-offset < 4 {
				return Message{}, 0, ErrMalformedPacket
			}
			message.Args = append(message.Args, Int32(int32(binary.BigEndian.Uint32(packet[offset:offset+4]))))
			offset += 4
		case 'f':
			if len(packet)-offset < 4 {
				return Message{}, 0, ErrMalformedPacket
			}
			message.Args = append(message.Args, Float32(math.Float32frombits(binary.BigEndian.Uint32(packet[offset:offset+4]))))
			offset += 4
		case 's':
			value, next, err := readPaddedString(packet, offset)
			if err != nil {
				return Message{}, 0, err
			}
			message.Args = append(message.Args, String(value))
			offset = next
		case 'T':
			message.Args = append(message.Args, Bool(true))
		case 'F':
			message.Args = append(message.Args, Bool(false))
		default:
			return Message{}, 0, fmt.Errorf("%w: tag %q", ErrUnsupportedType, tag)
		}
	}
	return message, offset, nil
}

// ValidAddress reports whether address is encodable by this OSC subset.
func ValidAddress(address string) bool {
	return strings.HasPrefix(address, "/") && !strings.ContainsRune(address, '\x00')
}

func writePaddedString(out *bytes.Buffer, value string) {
	out.WriteString(value)
	out.WriteByte(0)
	for out.Len()%4 != 0 {
		out.WriteByte(0)
	}
}

func readPaddedString(packet []byte, offset int) (string, int, error) {
	if offset < 0 || offset >= len(packet) {
		return "", 0, ErrMalformedPacket
	}
	terminator := bytes.IndexByte(packet[offset:], 0)
	if terminator < 0 {
		return "", 0, ErrMalformedPacket
	}
	terminator += offset
	next := terminator + 1
	for next%4 != 0 {
		next++
	}
	if next > len(packet) {
		return "", 0, ErrMalformedPacket
	}
	for index := terminator + 1; index < next; index++ {
		if packet[index] != 0 {
			return "", 0, ErrMalformedPacket
		}
	}
	return string(append([]byte(nil), packet[offset:terminator]...)), next, nil
}
