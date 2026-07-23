package ipc

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

const frameHeaderSize = 4

type fatalStreamError struct {
	err error
}

func (e *fatalStreamError) Error() string { return e.err.Error() }
func (e *fatalStreamError) Unwrap() error { return e.err }

func writeFrame(writer io.Writer, message protocol.Message) error {
	if err := message.Validate(); err != nil {
		return fmt.Errorf("ipc: validate outbound message: %w", err)
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("ipc: encode outbound message: %w", err)
	}
	if len(payload) > protocol.MaxMessageSize {
		return fmt.Errorf("%w: encoded size %d", ErrFrameTooLarge, len(payload))
	}

	var header [frameHeaderSize]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if err := writeFull(writer, header[:]); err != nil {
		return &fatalStreamError{err: fmt.Errorf("ipc: write frame header: %w", err)}
	}
	if err := writeFull(writer, payload); err != nil {
		return &fatalStreamError{err: fmt.Errorf("ipc: write frame body: %w", err)}
	}
	return nil
}

func writeFull(writer io.Writer, data []byte) error {
	for len(data) != 0 {
		written, err := writer.Write(data)
		if written < 0 || written > len(data) {
			return io.ErrShortWrite
		}
		data = data[written:]
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrNoProgress
		}
	}
	return nil
}

func readFrame(reader io.Reader) (protocol.Message, error) {
	var header [frameHeaderSize]byte
	read, err := io.ReadFull(reader, header[:])
	if err != nil {
		if read == 0 && errors.Is(err, io.EOF) {
			return protocol.Message{}, io.EOF
		}
		return protocol.Message{}, &fatalStreamError{
			err: fmt.Errorf("%w: truncated header: %v", ErrMalformedFrame, err),
		}
	}

	length := binary.BigEndian.Uint32(header[:])
	if length == 0 {
		return protocol.Message{}, &fatalStreamError{
			err: fmt.Errorf("%w: zero-length payload", ErrMalformedFrame),
		}
	}
	if uint64(length) > uint64(protocol.MaxMessageSize) {
		return protocol.Message{}, &fatalStreamError{
			err: fmt.Errorf("%w: declared size %d", ErrFrameTooLarge, length),
		}
	}

	payload := make([]byte, int(length))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return protocol.Message{}, &fatalStreamError{
			err: fmt.Errorf("%w: truncated body: %v", ErrMalformedFrame, err),
		}
	}
	var message protocol.Message
	if err := json.Unmarshal(payload, &message); err != nil {
		return protocol.Message{}, &fatalStreamError{
			err: fmt.Errorf("%w: decode protocol message: %v", ErrMalformedFrame, err),
		}
	}
	return message, nil
}
