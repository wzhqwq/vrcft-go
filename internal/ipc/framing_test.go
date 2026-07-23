package ipc

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

type oneByteReader struct{ io.Reader }

func (r oneByteReader) Read(buffer []byte) (int, error) {
	if len(buffer) > 1 {
		buffer = buffer[:1]
	}
	return r.Reader.Read(buffer)
}

type oneByteWriter struct{ bytes.Buffer }

func (w *oneByteWriter) Write(buffer []byte) (int, error) {
	if len(buffer) > 1 {
		buffer = buffer[:1]
	}
	return w.Buffer.Write(buffer)
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }

func testHeartbeatMessage(t *testing.T) protocol.Message {
	t.Helper()
	message, err := protocol.NewMessage(protocol.Heartbeat{UptimeMS: 7})
	if err != nil {
		t.Fatal(err)
	}
	return message
}

func rawFrame(payload []byte) []byte {
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)
	return frame
}

func TestWriteFrameUsesBigEndianLengthAndSupportsShortWrites(t *testing.T) {
	message := testHeartbeatMessage(t)
	var writer oneByteWriter
	if err := writeFrame(&writer, message); err != nil {
		t.Fatalf("writeFrame() error = %v", err)
	}
	data := writer.Bytes()
	if got := int(binary.BigEndian.Uint32(data[:4])); got != len(data)-4 {
		t.Fatalf("declared length = %d, want %d", got, len(data)-4)
	}
	var decoded protocol.Message
	if err := json.Unmarshal(data[4:], &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, message) {
		t.Fatalf("decoded message = %#v, want %#v", decoded, message)
	}
}

func TestWriteFrameRejectsInvalidMessageBeforeWriting(t *testing.T) {
	var destination bytes.Buffer
	err := writeFrame(&destination, protocol.Message{})
	if err == nil {
		t.Fatal("writeFrame(invalid message) error = nil")
	}
	if destination.Len() != 0 {
		t.Fatalf("writeFrame(invalid message) wrote %d bytes", destination.Len())
	}
}

func TestWriteFrameRejectsZeroProgress(t *testing.T) {
	if err := writeFrame(zeroWriter{}, testHeartbeatMessage(t)); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("writeFrame(zero progress) error = %v, want io.ErrNoProgress", err)
	}
}

func TestReadFrameSupportsSplitReads(t *testing.T) {
	message := testHeartbeatMessage(t)
	var encoded bytes.Buffer
	if err := writeFrame(&encoded, message); err != nil {
		t.Fatal(err)
	}
	got, err := readFrame(oneByteReader{Reader: &encoded})
	if err != nil {
		t.Fatalf("readFrame() error = %v", err)
	}
	if !reflect.DeepEqual(got, message) {
		t.Fatalf("readFrame() = %#v, want %#v", got, message)
	}
}

func TestReadFrameClassifiesStreamFailures(t *testing.T) {
	oversized := make([]byte, 4)
	binary.BigEndian.PutUint32(oversized, uint32(protocol.MaxMessageSize+1))
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{name: "clean EOF", data: nil, want: io.EOF},
		{name: "partial header", data: []byte{0, 0}, want: ErrMalformedFrame},
		{name: "zero length", data: make([]byte, 4), want: ErrMalformedFrame},
		{name: "oversized", data: oversized, want: ErrFrameTooLarge},
		{name: "partial body", data: rawFrame([]byte(`{"version":`)), want: ErrMalformedFrame},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := test.data
			if test.name == "partial body" {
				data = data[:len(data)-2]
			}
			_, err := readFrame(bytes.NewReader(data))
			if !errors.Is(err, test.want) {
				t.Fatalf("readFrame() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestReadFrameRejectsInvalidProtocolJSON(t *testing.T) {
	tests := [][]byte{
		[]byte(`not-json`),
		[]byte(`{"version":1,"type":3,"payload":{},"unknown":true}`),
		[]byte(`{"version":1,"type":7,"payload":{"level":0,"message":"x"}}`),
	}
	for _, payload := range tests {
		if _, err := readFrame(bytes.NewReader(rawFrame(payload))); !errors.Is(err, ErrMalformedFrame) {
			t.Fatalf("readFrame(%q) error = %v, want ErrMalformedFrame", payload, err)
		}
	}
}
