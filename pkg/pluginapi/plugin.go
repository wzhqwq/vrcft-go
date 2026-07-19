package pluginapi

import (
	"context"
	"encoding/json"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type Driver interface {
	Descriptor() Descriptor

	Run(ctx context.Context, env Environment) error
}

type Environment interface {
	InitialConfig() Config
	Commands() <-chan Command

	PublishFrame(frame trackingmodel.TrackingFrame) bool
	PublishStatus(status DeviceStatus)
	Log(level LogLevel, message string)
}

type Config struct {
	Revision uint64
	Data     json.RawMessage
}

type CommandType uint8

const (
	CommandSetActive CommandType = iota + 1
	CommandReconfigure
	CommandShutdown
)

type Command struct {
	Type CommandType

	Active bool
	Config Config
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
