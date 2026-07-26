package plugins

import (
	"context"
	"errors"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

var (
	ErrInvalidManifest                  = errors.New("plugins: invalid manifest")
	ErrDuplicatePluginID                = errors.New("plugins: duplicate plugin ID")
	ErrInvalidEntrypoint                = errors.New("plugins: invalid entrypoint")
	ErrUnknownPlugin                    = errors.New("plugins: unknown plugin")
	ErrManagerNotStarted                = errors.New("plugins: manager not started")
	ErrManagerClosed                    = errors.New("plugins: manager closed")
	ErrInvalidState                     = errors.New("plugins: invalid state")
	ErrControlBackpressure              = errors.New("plugins: control backpressure")
	ErrConfigRevisionRegression         = errors.New("plugins: config revision regression")
	ErrConfigRevisionConflict           = errors.New("plugins: config revision conflict")
	ErrSubscriptionGenerationRegression = errors.New("plugins: subscription generation regression")
	ErrSubscriptionGenerationConflict   = errors.New("plugins: subscription generation conflict")
	ErrHandshakeTimeout                 = errors.New("plugins: handshake timeout")
	ErrAuthenticationFailed             = errors.New("plugins: authentication failed")
	ErrDescriptorMismatch               = errors.New("plugins: descriptor mismatch")
	ErrProtocolIncompatible             = errors.New("plugins: protocol incompatible")
	ErrProtocolViolation                = errors.New("plugins: protocol violation")
	ErrHeartbeatTimeout                 = errors.New("plugins: heartbeat timeout")
	ErrGracefulShutdownTimeout          = errors.New("plugins: graceful shutdown timeout")
	ErrKillTimeout                      = errors.New("plugins: kill timeout")
	ErrRestartLimitReached              = errors.New("plugins: restart limit reached")
)

type FrameSink interface {
	Submit(string, uint64, trackingmodel.TrackingFrame)
}

type Manager interface {
	Start(context.Context) error
	Close(context.Context) error
	List() []RuntimeSnapshot
	Get(string) (RuntimeSnapshot, bool)
	Enable(context.Context, string) error
	Disable(context.Context, string) error
	Restart(context.Context, string) error
	UpdateConfig(context.Context, string, pluginapi.Config) error
	SetActive(context.Context, string, bool) error
	UpdateSubscription(context.Context, string, pluginapi.Subscription) error
	Subscribe(context.Context) <-chan Event
}
