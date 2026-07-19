package pluginruntime

import (
	"context"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

type Runtime struct {
	driver pluginapi.Driver
	conn   protocol.Conn

	initialConfig pluginapi.Config
	commands      chan pluginapi.Command

	frameSlot *LatestFrameSlot

	cancel context.CancelFunc
}
