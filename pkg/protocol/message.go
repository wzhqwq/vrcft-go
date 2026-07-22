package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const Version uint16 = 1

const MaxPayloadSize = 1024 * 1024

// messageEnvelopeAllowance bounds the JSON fields surrounding a maximum-size
// payload without making the payload limit depend on envelope formatting.
const messageEnvelopeAllowance = 256

type MessageType uint16

const (
	MessageHello MessageType = iota + 1
	MessageInitialize
	MessageReady
	MessageHeartbeat
	MessageTrackingFrame
	MessageStatus
	MessageLog
	MessageConfigChanged
	MessageSubscriptionChanged
	MessageActiveChanged
	MessageShutdown
	MessageShutdownAck
	MessageError
)

type Message struct {
	Version uint16
	Type    MessageType
	Payload any
}

type Hello struct {
	Token       string               `json:"token"`
	Descriptor  pluginapi.Descriptor `json:"descriptor"`
	ProtocolMin uint16               `json:"protocolMin"`
	ProtocolMax uint16               `json:"protocolMax"`
}

type Initialize struct {
	Startup pluginapi.Startup `json:"startup"`
}

type Ready struct{}

type Heartbeat struct {
	UptimeMS uint64 `json:"uptimeMs"`
}

type TrackingFrame struct {
	Generation uint64                      `json:"generation"`
	Frame      trackingmodel.TrackingFrame `json:"frame"`
}

type Status struct {
	Status pluginapi.DeviceStatus `json:"status"`
}

type Log struct {
	Level   pluginapi.LogLevel `json:"level"`
	Message string             `json:"message"`
	Dropped uint64             `json:"dropped,omitempty"`
}

type ConfigChanged struct {
	Config pluginapi.Config `json:"config"`
}

type SubscriptionChanged struct {
	Subscription pluginapi.Subscription `json:"subscription"`
}

type ActiveChanged struct {
	Active bool `json:"active"`
}

type Shutdown struct{}

type ShutdownAck struct{}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type wireMessage struct {
	Version uint16          `json:"version"`
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type wireConfig struct {
	Revision uint64
	Data     json.RawMessage `json:"Data,omitempty"`
}

type wireStartup struct {
	Active       bool
	Config       wireConfig
	Subscription pluginapi.Subscription
}

type wireInitialize struct {
	Startup wireStartup `json:"startup"`
}

type wireConfigChanged struct {
	Config wireConfig `json:"config"`
}

func NewMessage(payload any) (Message, error) {
	messageType, err := messageTypeForPayload(payload)
	if err != nil {
		return Message{}, err
	}
	message := Message{Version: Version, Type: messageType, Payload: payload}
	if err := message.Validate(); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (m Message) Validate() error {
	if m.Version != Version {
		return fmt.Errorf("protocol: message version %d must equal %d", m.Version, Version)
	}
	if !m.Type.valid() {
		return fmt.Errorf("protocol: unknown message type %d", m.Type)
	}
	payloadType, err := messageTypeForPayload(m.Payload)
	if err != nil {
		return err
	}
	if payloadType != m.Type {
		return fmt.Errorf("protocol: message type %d does not match payload %T", m.Type, m.Payload)
	}
	if err := validatePayload(m.Payload); err != nil {
		return fmt.Errorf("protocol: invalid %T payload: %w", m.Payload, err)
	}
	return nil
}

func (m Message) MarshalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	payload, err := marshalPayload(m.Payload)
	if err != nil {
		return nil, fmt.Errorf("protocol: encode %T payload: %w", m.Payload, err)
	}
	if len(payload) > MaxPayloadSize {
		return nil, fmt.Errorf("protocol: encoded payload size %d exceeds maximum %d", len(payload), MaxPayloadSize)
	}
	data, err := json.Marshal(wireMessage{Version: m.Version, Type: m.Type, Payload: payload})
	if err != nil {
		return nil, fmt.Errorf("protocol: encode message envelope: %w", err)
	}
	return data, nil
}

func (m *Message) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("protocol: cannot unmarshal into nil Message")
	}
	if len(data) > MaxPayloadSize+messageEnvelopeAllowance {
		return fmt.Errorf("protocol: total message size %d exceeds maximum %d", len(data), MaxPayloadSize+messageEnvelopeAllowance)
	}

	var wire wireMessage
	if err := decodeStrictJSON(data, &wire); err != nil {
		return fmt.Errorf("protocol: decode message envelope: %w", err)
	}
	if len(wire.Payload) > MaxPayloadSize {
		return fmt.Errorf("protocol: raw payload size %d exceeds maximum %d", len(wire.Payload), MaxPayloadSize)
	}
	if wire.Version != Version {
		return fmt.Errorf("protocol: message version %d must equal %d", wire.Version, Version)
	}
	if !wire.Type.valid() {
		return fmt.Errorf("protocol: unknown message type %d", wire.Type)
	}
	if bytes.Equal(bytes.TrimSpace(wire.Payload), []byte("null")) {
		return fmt.Errorf("protocol: payload for message type %d must be a JSON object", wire.Type)
	}

	payload, err := decodePayload(wire.Type, wire.Payload)
	if err != nil {
		return err
	}
	decoded := Message{Version: wire.Version, Type: wire.Type, Payload: payload}
	if err := decoded.Validate(); err != nil {
		return err
	}
	*m = decoded
	return nil
}

func marshalPayload(payload any) ([]byte, error) {
	switch p := payload.(type) {
	case Initialize:
		return json.Marshal(wireInitialize{Startup: wireStartup{
			Active:       p.Startup.Active,
			Config:       toWireConfig(p.Startup.Config),
			Subscription: p.Startup.Subscription,
		}})
	case ConfigChanged:
		return json.Marshal(wireConfigChanged{Config: toWireConfig(p.Config)})
	default:
		return json.Marshal(payload)
	}
}

func toWireConfig(config pluginapi.Config) wireConfig {
	return wireConfig{Revision: config.Revision, Data: config.Data}
}

func fromWireConfig(config wireConfig) pluginapi.Config {
	return pluginapi.Config{Revision: config.Revision, Data: config.Data}
}

func (t MessageType) valid() bool {
	return t >= MessageHello && t <= MessageError
}

func messageTypeForPayload(payload any) (MessageType, error) {
	switch payload.(type) {
	case Hello:
		return MessageHello, nil
	case Initialize:
		return MessageInitialize, nil
	case Ready:
		return MessageReady, nil
	case Heartbeat:
		return MessageHeartbeat, nil
	case TrackingFrame:
		return MessageTrackingFrame, nil
	case Status:
		return MessageStatus, nil
	case Log:
		return MessageLog, nil
	case ConfigChanged:
		return MessageConfigChanged, nil
	case SubscriptionChanged:
		return MessageSubscriptionChanged, nil
	case ActiveChanged:
		return MessageActiveChanged, nil
	case Shutdown:
		return MessageShutdown, nil
	case ShutdownAck:
		return MessageShutdownAck, nil
	case Error:
		return MessageError, nil
	default:
		return 0, fmt.Errorf("protocol: unsupported payload type %T; payload must be a protocol value", payload)
	}
}

func validatePayload(payload any) error {
	switch p := payload.(type) {
	case Hello:
		if strings.TrimSpace(p.Token) == "" {
			return errors.New("Hello.Token must be nonblank")
		}
		if err := p.Descriptor.Validate(); err != nil {
			return fmt.Errorf("Hello.Descriptor: %w", err)
		}
		if p.ProtocolMin != Version || p.ProtocolMax != Version {
			return fmt.Errorf("Hello protocol range must be [%d, %d]", Version, Version)
		}
	case Initialize:
		if err := p.Startup.Config.Validate(); err != nil {
			return fmt.Errorf("Initialize.Startup.Config: %w", err)
		}
		if err := p.Startup.Subscription.Validate(p.Startup.Active); err != nil {
			return fmt.Errorf("Initialize.Startup.Subscription: %w", err)
		}
	case Ready, Heartbeat, ActiveChanged, Shutdown, ShutdownAck:
		return nil
	case TrackingFrame:
		if p.Generation == 0 {
			return errors.New("TrackingFrame.Generation must be positive")
		}
		known := trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression
		if p.Frame.Capabilities&^known != 0 {
			return errors.New("TrackingFrame.Frame.Capabilities contains unknown capability bits")
		}
	case Status:
		if err := p.Status.Validate(); err != nil {
			return fmt.Errorf("Status.Status: %w", err)
		}
	case Log:
		if err := p.Level.Validate(); err != nil {
			return fmt.Errorf("Log.Level: %w", err)
		}
		if strings.TrimSpace(p.Message) == "" {
			return errors.New("Log.Message must be nonblank")
		}
	case ConfigChanged:
		if err := p.Config.Validate(); err != nil {
			return fmt.Errorf("ConfigChanged.Config: %w", err)
		}
		if p.Config.Revision == 0 {
			return errors.New("ConfigChanged.Config.Revision must be positive")
		}
	case SubscriptionChanged:
		if err := p.Subscription.Validate(true); err != nil {
			return fmt.Errorf("SubscriptionChanged.Subscription: %w", err)
		}
	case Error:
		if strings.TrimSpace(p.Code) == "" {
			return errors.New("Error.Code must be nonblank")
		}
		if strings.TrimSpace(p.Message) == "" {
			return errors.New("Error.Message must be nonblank")
		}
	default:
		return fmt.Errorf("unsupported payload type %T", payload)
	}
	return nil
}

func decodePayload(messageType MessageType, raw json.RawMessage) (any, error) {
	if messageType == MessageInitialize {
		var payload wireInitialize
		if err := decodeStrictJSON(raw, &payload); err != nil {
			return nil, fmt.Errorf("protocol: decode payload for message type %d: %w", messageType, err)
		}
		return Initialize{Startup: pluginapi.Startup{
			Active:       payload.Startup.Active,
			Config:       fromWireConfig(payload.Startup.Config),
			Subscription: payload.Startup.Subscription,
		}}, nil
	}
	if messageType == MessageConfigChanged {
		var payload wireConfigChanged
		if err := decodeStrictJSON(raw, &payload); err != nil {
			return nil, fmt.Errorf("protocol: decode payload for message type %d: %w", messageType, err)
		}
		return ConfigChanged{Config: fromWireConfig(payload.Config)}, nil
	}

	var payload any
	switch messageType {
	case MessageHello:
		payload = new(Hello)
	case MessageReady:
		payload = new(Ready)
	case MessageHeartbeat:
		payload = new(Heartbeat)
	case MessageTrackingFrame:
		payload = new(TrackingFrame)
	case MessageStatus:
		payload = new(Status)
	case MessageLog:
		payload = new(Log)
	case MessageSubscriptionChanged:
		payload = new(SubscriptionChanged)
	case MessageActiveChanged:
		payload = new(ActiveChanged)
	case MessageShutdown:
		payload = new(Shutdown)
	case MessageShutdownAck:
		payload = new(ShutdownAck)
	case MessageError:
		payload = new(Error)
	default:
		return nil, fmt.Errorf("protocol: unknown message type %d", messageType)
	}

	if err := decodeStrictJSON(raw, payload); err != nil {
		return nil, fmt.Errorf("protocol: decode payload for message type %d: %w", messageType, err)
	}

	switch p := payload.(type) {
	case *Hello:
		return *p, nil
	case *Ready:
		return *p, nil
	case *Heartbeat:
		return *p, nil
	case *TrackingFrame:
		return *p, nil
	case *Status:
		return *p, nil
	case *Log:
		return *p, nil
	case *SubscriptionChanged:
		return *p, nil
	case *ActiveChanged:
		return *p, nil
	case *Shutdown:
		return *p, nil
	case *ShutdownAck:
		return *p, nil
	case *Error:
		return *p, nil
	default:
		return nil, fmt.Errorf("protocol: internal unsupported payload type %T", payload)
	}
}

func decodeStrictJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return ensureJSONEOF(decoder)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}
