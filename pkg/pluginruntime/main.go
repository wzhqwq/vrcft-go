package pluginruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/wzhqwq/vrcft-go/internal/ipc"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

var ErrConnectionUnavailable = errors.New("pluginruntime: connection factory unavailable")

const (
	PipeNameEnv     = "VRCFT_PIPE_NAME"
	SessionTokenEnv = "VRCFT_SESSION_TOKEN"
)

var connect = connectFromEnvironment

func connectFromEnvironment(ctx context.Context) (protocol.Conn, string, error) {
	pipeName := os.Getenv(PipeNameEnv)
	if strings.TrimSpace(pipeName) == "" {
		return nil, "", fmt.Errorf("%w: %s must be nonblank", ErrConnectionUnavailable, PipeNameEnv)
	}
	token := os.Getenv(SessionTokenEnv)
	if strings.TrimSpace(token) == "" {
		return nil, "", fmt.Errorf("%w: %s must be nonblank", ErrConnectionUnavailable, SessionTokenEnv)
	}
	conn, err := ipc.Connect(ctx, ipc.ClientConfig{PipeName: pipeName})
	if err != nil {
		return nil, "", fmt.Errorf("pluginruntime: connect named pipe: %w", err)
	}
	return conn, token, nil
}

func Main(driver pluginapi.Driver) error {
	ctx := context.Background()
	conn, token, err := connect(ctx)
	if err != nil {
		return err
	}
	cfg := DefaultRuntimeConfig()
	cfg.Token = token
	runtime, err := New(driver, conn, cfg)
	if err != nil {
		if conn != nil {
			if closeErr := conn.Close(); closeErr != nil {
				return errors.Join(err, closeErr)
			}
		}
		return err
	}
	return runtime.Run(ctx)
}
