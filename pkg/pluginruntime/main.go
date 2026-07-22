package pluginruntime

import (
	"context"
	"errors"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

var ErrConnectionUnavailable = errors.New("pluginruntime: connection factory unavailable")

var connect = func(context.Context) (protocol.Conn, string, error) {
	return nil, "", ErrConnectionUnavailable
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
