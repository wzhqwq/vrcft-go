//go:build windows

package ipc

import (
	"context"
	"errors"
	"fmt"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

const windowsPipeBufferSize int32 = 64 * 1024

func Listen(config ServerConfig) (Listener, error) {
	if err := validateServerConfig(config); err != nil {
		return nil, err
	}
	sid, err := currentUserSID()
	if err != nil {
		return nil, fmt.Errorf("ipc: resolve current user SID: %w", err)
	}
	listener, err := winio.ListenPipe(pipePath(config.PipeName), windowsPipeConfig(sid))
	if err != nil {
		return nil, fmt.Errorf("ipc: listen on named pipe: %w", err)
	}
	return newOneShotListener(listener), nil
}

func Connect(ctx context.Context, config ClientConfig) (protocol.Conn, error) {
	if ctx == nil {
		return nil, errors.New("ipc: Connect context must not be nil")
	}
	if err := validateClientConfig(config); err != nil {
		return nil, err
	}
	conn, err := winio.DialPipeContext(ctx, pipePath(config.PipeName))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("ipc: connect to named pipe: %w", err)
	}
	return newConn(conn), nil
}

func currentUserSID() (sid string, result error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	defer func() {
		result = errors.Join(result, token.Close())
	}()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func pipeSecurityDescriptor(sid string) string {
	return "D:P(A;;GA;;;SY)(A;;GA;;;" + sid + ")"
}

func windowsPipeConfig(sid string) *winio.PipeConfig {
	return &winio.PipeConfig{
		SecurityDescriptor: pipeSecurityDescriptor(sid),
		MessageMode:        false,
		InputBufferSize:    windowsPipeBufferSize,
		OutputBufferSize:   windowsPipeBufferSize,
	}
}
