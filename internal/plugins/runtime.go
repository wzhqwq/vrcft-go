package plugins

import (
	"context"
	"os/exec"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/protocol"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type State string

const (
	StateDisabled     State = "disabled"
	StateStopped      State = "stopped"
	StateStarting     State = "starting"
	StateHandshaking  State = "handshaking"
	StateRunning      State = "running"
	StateStopping     State = "stopping"
	StateCrashed      State = "crashed"
	StateUnresponsive State = "unresponsive"
	StateIncompatible State = "incompatible"
)

type RuntimeSnapshot struct {
	ID      string
	Name    string
	Version string

	Enabled bool
	State   State

	PID int

	Capabilities trackingmodel.Capability

	StartedAt       time.Time
	LastHeartbeatAt time.Time
	LastFrameAt     time.Time

	FrameRate float64

	RestartCount int
	LastError    string
}

type runtimeInstance struct {
	manifest Manifest

	state State

	cmd     *exec.Cmd
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
