package protocol

import (
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type MessageType uint16

const (
	MessageHello MessageType = iota + 1
	MessageInitialize
	MessageReady
	MessageTrackingFrame
	MessageHeartbeat
	MessageStatus
	MessageLog
	MessageReconfigure
	MessageSetActive
	MessageShutdown
	MessageShutdownAck
	MessageError
)

type Header struct {
	Magic      [4]byte
	Version    uint16
	Type       MessageType
	PayloadLen uint32
	Sequence   uint64
}

var Magic = [4]byte{'V', 'F', 'T', '1'}

const MaxPayloadSize = 1024 * 1024

type Hello struct {
	Token string `json:"token"`

	PluginID      string `json:"pluginId"`
	PluginVersion string `json:"pluginVersion"`

	ProtocolMin uint16 `json:"protocolMin"`
	ProtocolMax uint16 `json:"protocolMax"`

	Capabilities trackingmodel.Capability `json:"capabilities"`
}

type Initialize struct {
	Config pluginapi.Config `json:"config"`
}

type Ready struct {
	Capabilities trackingmodel.Capability `json:"capabilities"`
}

type Heartbeat struct {
	UptimeMS uint64 `json:"uptimeMs"`
}
