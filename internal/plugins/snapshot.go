package plugins

import (
	"time"

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
	StateBackoff      State = "backoff"
	StateCrashed      State = "crashed"
	StateUnresponsive State = "unresponsive"
	StateIncompatible State = "incompatible"
)

type RuntimeSnapshot struct {
	ID           string
	Name         string
	Version      string
	Capabilities trackingmodel.Capability

	Enabled bool
	Active  bool
	State   State
	PID     int

	ConfigRevision         uint64
	SubscriptionGeneration uint64

	StartedAt       time.Time
	LastHeartbeatAt time.Time
	LastFrameAt     time.Time
	NextRestartAt   time.Time

	FrameRate float64

	ConsecutiveFailures int
	RestartCount        int
	LastError           string
}

func (s RuntimeSnapshot) clone() RuntimeSnapshot { return s }
