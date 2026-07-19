package plugins

import (
	"context"
	"time"
)

type Process interface {
	PID() int
	Wait() error
	Kill() error
}

type ProcessLauncher interface {
	Start(ctx context.Context, spec ProcessSpec) (Process, error)
}

type ProcessSpec struct {
	Executable string
	Args       []string
	WorkingDir string
	Env        []string
}

type ShutdownPolicy struct {
	HandshakeTimeout time.Duration
	GracefulTimeout  time.Duration
	KillTimeout      time.Duration
}

var defaultPolicy = ShutdownPolicy{
	HandshakeTimeout: 5 * time.Second,
	GracefulTimeout:  2 * time.Second,
	KillTimeout:      2 * time.Second,
}
