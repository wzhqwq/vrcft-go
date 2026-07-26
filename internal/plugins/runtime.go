package plugins

import (
	"context"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

type runtimeInstance struct {
	manifest Manifest

	state State

	process Process

	conn protocol.Conn

	cancel context.CancelFunc
	done   chan struct{}

	lastHeartbeat time.Time
	lastFrame     time.Time

	restartCount int
	lastError    error

	commands chan runtimeCommandType
}

type runtimeCommandType uint8

const (
	runtimeStart runtimeCommandType = iota + 1
	runtimeStop
	runtimeRestart
	runtimeProcessExited
	runtimeHeartbeatTimeout
)
