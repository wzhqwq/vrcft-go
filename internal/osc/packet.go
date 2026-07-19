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
)

type ValueKind uint8

const (
	ValueInt32 ValueKind = iota + 1
	ValueFloat32
	ValueString
	ValueBool
)

type Value struct {
	Kind ValueKind
	I32  int32
	F32  float32
	Str  string
	Bool bool
}

func Int32(v int32) Value     { return Value{Kind: ValueInt32, I32: v} }
func Float32(v float32) Value { return Value{Kind: ValueFloat32, F32: v} }
func String(v string) Value   { return Value{Kind: ValueString, Str: v} }
func Bool(v bool) Value       { return Value{Kind: ValueBool, Bool: v} }

type Message struct {
	Address string
	Args    []Value
}

type Bundle struct {
	// Timetag 1 means "immediately" in OSC. Zero is normalized to 1.
	Timetag  uint64
	Elements [][]byte
}

func MarshalMessage(m Message) ([]byte, error) {
	if !validAddress(m.Address) {
		return nil, fmt.Errorf("invalid OSC address %q", m.Address)
	}

	var out bytes.Buffer
	writePaddedString(&out, m.Address)

	var tags strings.Builder
	tags.WriteByte(',')
	for _, arg := range m.Args {
		switch arg.Kind {
		case ValueInt32:
			tags.WriteByte('i')
		case ValueFloat32:
			tags.WriteByte('f')
		case ValueString:
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
	writePaddedString(&out, tags.String())

	for _, arg := range m.Args {
		switch arg.Kind {
		case ValueInt32:
			_ = binary.Write(&out, binary.BigEndian, arg.I32)
		case ValueFloat32:
			_ = binary.Write(&out, binary.BigEndian, math.Float32bits(arg.F32))
		case ValueString:
			writePaddedString(&out, arg.Str)
		case ValueBool:
			// OSC T/F type tags carry no payload bytes.
		}
	}
	return out.Bytes(), nil
}

func MarshalBundle(b Bundle) ([]byte, error) {
	var out bytes.Buffer
	out.WriteString("#bundle\x00")
	if b.Timetag == 0 {
		b.Timetag = 1
	}
	_ = binary.Write(&out, binary.BigEndian, b.Timetag)
	for _, element := range b.Elements {
		if len(element) == 0 || len(element) > math.MaxInt32 {
			return nil, ErrMalformedPacket
		}
		_ = binary.Write(&out, binary.BigEndian, int32(len(element)))
		out.Write(element)
	}
	return out.Bytes(), nil
}

// NTPTime converts a wall-clock time to an OSC/NTP timetag.
func NTPTime(t time.Time) uint64 {
	const ntpToUnix = 2_208_988_800
	sec := uint64(t.Unix() + ntpToUnix)
	frac := uint64((uint64(t.Nanosecond()) << 32) / 1_000_000_000)
	return sec<<32 | frac
}

func UnmarshalPacket(packet []byte) ([]Message, error) {
	if len(packet) < 4 {
		return nil, ErrMalformedPacket
	}
	if bytes.HasPrefix(packet, []byte("#bundle\x00")) {
		return unmarshalBundle(packet)
	}
	m, _, err := unmarshalMessage(packet)
	if err != nil {
		return nil, err
	}
	return []Message{m}, nil
}

func unmarshalBundle(packet []byte) ([]Message, error) {
	if len(packet) < 16 || !bytes.Equal(packet[:8], []byte("#bundle\x00")) {
		return nil, ErrMalformedPacket
	}
	offset := 16 // skip marker and timetag
	var result []Message
	for offset < len(packet) {
		if offset+4 > len(packet) {
			return nil, ErrMalformedPacket
		}
		size := int(int32(binary.BigEndian.Uint32(packet[offset : offset+4])))
		offset += 4
		if size <= 0 || offset+size > len(packet) {
			return nil, ErrMalformedPacket
		}
		messages, err := UnmarshalPacket(packet[offset : offset+size])
		if err != nil {
			return nil, err
		}
		result = append(result, messages...)
		offset += size
	}
	return result, nil
}

func unmarshalMessage(packet []byte) (Message, int, error) {
	address, offset, err := readPaddedString(packet, 0)
	if err != nil || !validAddress(address) {
		return Message{}, 0, ErrMalformedPacket
	}
	tags, offset, err := readPaddedString(packet, offset)
	if err != nil || len(tags) == 0 || tags[0] != ',' {
		return Message{}, 0, ErrMalformedPacket
	}

	m := Message{Address: address, Args: make([]Value, 0, len(tags)-1)}
	for _, tag := range tags[1:] {
		switch tag {
		case 'i':
			if offset+4 > len(packet) {
				return Message{}, 0, ErrMalformedPacket
			}
			m.Args = append(m.Args, Int32(int32(binary.BigEndian.Uint32(packet[offset:offset+4]))))
			offset += 4
		case 'f':
			if offset+4 > len(packet) {
				return Message{}, 0, ErrMalformedPacket
			}
			m.Args = append(m.Args, Float32(math.Float32frombits(binary.BigEndian.Uint32(packet[offset:offset+4]))))
			offset += 4
		case 's':
			v, next, err := readPaddedString(packet, offset)
			if err != nil {
				return Message{}, 0, err
			}
			m.Args = append(m.Args, String(v))
			offset = next
		case 'T':
			m.Args = append(m.Args, Bool(true))
		case 'F':
			m.Args = append(m.Args, Bool(false))
		default:
			return Message{}, 0, fmt.Errorf("%w: tag %q", ErrUnsupportedType, tag)
		}
	}
	return m, offset, nil
}

func validAddress(address string) bool {
	return strings.HasPrefix(address, "/") && !strings.ContainsRune(address, '\x00')
}

func writePaddedString(out *bytes.Buffer, s string) {
	out.WriteString(s)
	out.WriteByte(0)
	for out.Len()%4 != 0 {
		out.WriteByte(0)
	}
}

func readPaddedString(packet []byte, offset int) (string, int, error) {
	if offset < 0 || offset >= len(packet) {
		return "", 0, ErrMalformedPacket
	}
	end := bytes.IndexByte(packet[offset:], 0)
	if end < 0 {
		return "", 0, ErrMalformedPacket
	}
	end += offset
	next := end + 1
	for next%4 != 0 {
		next++
	}
	if next > len(packet) {
		return "", 0, ErrMalformedPacket
	}
	return string(packet[offset:end]), next, nil
}
