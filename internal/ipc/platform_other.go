//go:build !windows

package ipc

import (
	"context"
	"errors"

	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

func Listen(config ServerConfig) (Listener, error) {
	if err := validateServerConfig(config); err != nil {
		return nil, err
	}
	return nil, ErrUnsupportedPlatform
}

func Connect(ctx context.Context, config ClientConfig) (protocol.Conn, error) {
	if ctx == nil {
		return nil, errors.New("ipc: Connect context must not be nil")
	}
	if err := validateClientConfig(config); err != nil {
		return nil, err
	}
	return nil, ErrUnsupportedPlatform
}
