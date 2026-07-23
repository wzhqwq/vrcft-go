package ipc

import (
	"context"
	"errors"
	"fmt"

	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

var (
	ErrUnsupportedPlatform = errors.New("ipc: named pipes are unsupported on this platform")
	ErrInvalidPipeName     = errors.New("ipc: invalid pipe name")
	ErrListenerConsumed    = errors.New("ipc: listener already accepted a connection")
	ErrFrameTooLarge       = errors.New("ipc: frame exceeds maximum size")
	ErrMalformedFrame      = errors.New("ipc: malformed frame")
)

type ServerConfig struct {
	PipeName string
}

type ClientConfig struct {
	PipeName string
}

type Listener interface {
	Accept(context.Context) (protocol.Conn, error)
	Close() error
}

func validatePipeName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("%w: name must not be empty", ErrInvalidPipeName)
	}
	if len(name) > 128 {
		return fmt.Errorf("%w: name exceeds 128 bytes", ErrInvalidPipeName)
	}
	for index := range len(name) {
		if !validPipeNameByte(name[index]) {
			return fmt.Errorf("%w: name contains a disallowed byte", ErrInvalidPipeName)
		}
	}
	return nil
}

func validPipeNameByte(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' ||
		value == '-' || value == '_'
}

func pipePath(name string) string {
	return `\\.\pipe\vrcft-` + name
}
