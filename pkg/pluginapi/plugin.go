package pluginapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const APIVersion uint16 = 1

type Driver interface {
	Descriptor() Descriptor
	Run(context.Context, Host) error
}

type Host interface {
	Startup() Startup
	Events() <-chan ControlEvent
	PublishFrame(trackingmodel.TrackingFrame) bool
	PublishStatus(DeviceStatus)
	Log(LogLevel, string)
}

type Startup struct {
	Active       bool
	Config       Config
	Subscription Subscription
}

type Config struct {
	Revision uint64
	Data     json.RawMessage
}

func (c Config) Clone() Config {
	clone := Config{Revision: c.Revision}
	if len(c.Data) != 0 {
		clone.Data = make(json.RawMessage, len(c.Data))
		copy(clone.Data, c.Data)
	}
	return clone
}

func (c Config) Validate() error {
	if len(c.Data) == 0 {
		return nil
	}
	if c.Revision == 0 {
		return errors.New("Config.Revision must be positive when Data is nonempty")
	}
	if !json.Valid(c.Data) {
		return errors.New("Config.Data must contain valid JSON")
	}
	return nil
}

type DeviceState string

const (
	DeviceInitializing DeviceState = "initializing"
	DeviceReady        DeviceState = "ready"
	DeviceDisconnected DeviceState = "disconnected"
	DeviceError        DeviceState = "error"
)

type DeviceStatus struct {
	State   DeviceState
	Message string
}

func (s DeviceStatus) Validate() error {
	switch s.State {
	case DeviceInitializing, DeviceReady, DeviceDisconnected:
		return nil
	case DeviceError:
		if strings.TrimSpace(s.Message) == "" {
			return errors.New("DeviceStatus.Message must be nonblank for error state")
		}
		return nil
	default:
		return errors.New("DeviceStatus.State is unknown")
	}
}
