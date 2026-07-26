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

// MaxMessageSize bounds the encoded payload plus its JSON envelope.
const MaxMessageSize = MaxPayloadSize + 256

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
	Subscription wireSubscription
}

type wireInitialize struct {
	Startup wireStartup `json:"startup"`
}

type wireConfigChanged struct {
	Config wireConfig `json:"config"`
}

type wireExpressionMask struct {
	Words []uint64
}

type wireSubscription struct {
	Generation   uint64
	Capabilities trackingmodel.Capability
	Eye          trackingmodel.EyeValid
	Expressions  wireExpressionMask
}

type wireSubscriptionChanged struct {
	Subscription wireSubscription `json:"subscription"`
}

type wireExpressionSet struct {
	Values []float32
	Valid  wireExpressionMask
}

type wireTrackingModelFrame struct {
	Sequence      uint64
	TimestampNS   int64
	Capabilities  trackingmodel.Capability
	SourceClockNS int64
	Eye           trackingmodel.EyeSample
	Expressions   wireExpressionSet
}

type wireTrackingFrame struct {
	Generation uint64                 `json:"generation"`
	Frame      wireTrackingModelFrame `json:"frame"`
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
	_, err := m.validatedPayload()
	return err
}

func (m Message) validateSemantics() error {
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

func (m Message) validatedPayload() ([]byte, error) {
	if err := m.validateSemantics(); err != nil {
		return nil, err
	}
	payload, err := marshalPayload(m.Payload)
	if err != nil {
		return nil, fmt.Errorf("protocol: encode %T payload: %w", m.Payload, err)
	}
	if len(payload) > MaxPayloadSize {
		return nil, fmt.Errorf("protocol: encoded payload size %d exceeds maximum %d", len(payload), MaxPayloadSize)
	}
	return payload, nil
}

func (m Message) MarshalJSON() ([]byte, error) {
	payload, err := m.validatedPayload()
	if err != nil {
		return nil, err
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
	if len(data) > MaxMessageSize {
		return fmt.Errorf("protocol: total message size %d exceeds maximum %d", len(data), MaxMessageSize)
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
	// Hello policy (credentials, descriptor, and version-range compatibility)
	// belongs to the Host handshake so it can classify the failure.  The wire
	// envelope and payload shape remain strict above; all other messages retain
	// eager semantic validation here.
	if wire.Type != MessageHello {
		if err := decoded.Validate(); err != nil {
			return err
		}
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
			Subscription: toWireSubscription(p.Startup.Subscription),
		}})
	case ConfigChanged:
		return json.Marshal(wireConfigChanged{Config: toWireConfig(p.Config)})
	case SubscriptionChanged:
		return json.Marshal(wireSubscriptionChanged{Subscription: toWireSubscription(p.Subscription)})
	case TrackingFrame:
		return json.Marshal(toWireTrackingFrame(p))
	default:
		return json.Marshal(payload)
	}
}

func toWireConfig(config pluginapi.Config) wireConfig {
	config = config.Clone()
	return wireConfig{Revision: config.Revision, Data: config.Data}
}

func fromWireConfig(config wireConfig) pluginapi.Config {
	return (pluginapi.Config{Revision: config.Revision, Data: config.Data}).Clone()
}

func toWireExpressionMask(mask trackingmodel.ExpressionMask) wireExpressionMask {
	words := make([]uint64, len(mask.Words))
	copy(words, mask.Words[:])
	return wireExpressionMask{Words: words}
}

func fromWireExpressionMask(mask wireExpressionMask, field string) (trackingmodel.ExpressionMask, error) {
	var result trackingmodel.ExpressionMask
	if len(mask.Words) != len(result.Words) {
		return trackingmodel.ExpressionMask{}, fmt.Errorf(
			"%s.Words length %d must equal %d",
			field,
			len(mask.Words),
			len(result.Words),
		)
	}
	copy(result.Words[:], mask.Words)
	return result, nil
}

func toWireSubscription(subscription pluginapi.Subscription) wireSubscription {
	return wireSubscription{
		Generation:   subscription.Generation,
		Capabilities: subscription.Capabilities,
		Eye:          subscription.Eye,
		Expressions:  toWireExpressionMask(subscription.Expressions),
	}
}

func fromWireSubscription(subscription wireSubscription, field string) (pluginapi.Subscription, error) {
	expressions, err := fromWireExpressionMask(subscription.Expressions, field+".Expressions")
	if err != nil {
		return pluginapi.Subscription{}, err
	}
	return pluginapi.Subscription{
		Generation:   subscription.Generation,
		Capabilities: subscription.Capabilities,
		Eye:          subscription.Eye,
		Expressions:  expressions,
	}, nil
}

func toWireExpressionSet(expressions trackingmodel.ExpressionSet) wireExpressionSet {
	values := make([]float32, len(expressions.Values))
	copy(values, expressions.Values[:])
	return wireExpressionSet{
		Values: values,
		Valid:  toWireExpressionMask(expressions.Valid),
	}
}

func fromWireExpressionSet(expressions wireExpressionSet, field string) (trackingmodel.ExpressionSet, error) {
	var result trackingmodel.ExpressionSet
	if len(expressions.Values) != len(result.Values) {
		return trackingmodel.ExpressionSet{}, fmt.Errorf(
			"%s.Values length %d must equal %d",
			field,
			len(expressions.Values),
			len(result.Values),
		)
	}
	valid, err := fromWireExpressionMask(expressions.Valid, field+".Valid")
	if err != nil {
		return trackingmodel.ExpressionSet{}, err
	}
	copy(result.Values[:], expressions.Values)
	result.Valid = valid
	return result, nil
}

func toWireTrackingFrame(frame TrackingFrame) wireTrackingFrame {
	return wireTrackingFrame{
		Generation: frame.Generation,
		Frame: wireTrackingModelFrame{
			Sequence:      frame.Frame.Sequence,
			TimestampNS:   frame.Frame.TimestampNS,
			Capabilities:  frame.Frame.Capabilities,
			SourceClockNS: frame.Frame.SourceClockNS,
			Eye:           frame.Frame.Eye,
			Expressions:   toWireExpressionSet(frame.Frame.Expressions),
		},
	}
}

func fromWireTrackingFrame(frame wireTrackingFrame) (TrackingFrame, error) {
	expressions, err := fromWireExpressionSet(frame.Frame.Expressions, "TrackingFrame.Frame.Expressions")
	if err != nil {
		return TrackingFrame{}, err
	}
	return TrackingFrame{
		Generation: frame.Generation,
		Frame: trackingmodel.TrackingFrame{
			Sequence:      frame.Frame.Sequence,
			TimestampNS:   frame.Frame.TimestampNS,
			Capabilities:  frame.Frame.Capabilities,
			SourceClockNS: frame.Frame.SourceClockNS,
			Eye:           frame.Frame.Eye,
			Expressions:   expressions,
		},
	}, nil
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
		if err := p.Frame.Validate(); err != nil {
			return fmt.Errorf("TrackingFrame.Frame: %w", err)
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
		subscription, err := fromWireSubscription(payload.Startup.Subscription, "Initialize.Startup.Subscription")
		if err != nil {
			return nil, fmt.Errorf("protocol: decode payload for message type %d: %w", messageType, err)
		}
		return Initialize{Startup: pluginapi.Startup{
			Active:       payload.Startup.Active,
			Config:       fromWireConfig(payload.Startup.Config),
			Subscription: subscription,
		}}, nil
	}
	if messageType == MessageConfigChanged {
		var payload wireConfigChanged
		if err := decodeStrictJSON(raw, &payload); err != nil {
			return nil, fmt.Errorf("protocol: decode payload for message type %d: %w", messageType, err)
		}
		return ConfigChanged{Config: fromWireConfig(payload.Config)}, nil
	}
	if messageType == MessageSubscriptionChanged {
		var payload wireSubscriptionChanged
		if err := decodeStrictJSON(raw, &payload); err != nil {
			return nil, fmt.Errorf("protocol: decode payload for message type %d: %w", messageType, err)
		}
		subscription, err := fromWireSubscription(payload.Subscription, "SubscriptionChanged.Subscription")
		if err != nil {
			return nil, fmt.Errorf("protocol: decode payload for message type %d: %w", messageType, err)
		}
		return SubscriptionChanged{Subscription: subscription}, nil
	}
	if messageType == MessageTrackingFrame {
		var payload wireTrackingFrame
		if err := decodeStrictJSON(raw, &payload); err != nil {
			return nil, fmt.Errorf("protocol: decode payload for message type %d: %w", messageType, err)
		}
		frame, err := fromWireTrackingFrame(payload)
		if err != nil {
			return nil, fmt.Errorf("protocol: decode payload for message type %d: %w", messageType, err)
		}
		return frame, nil
	}

	var payload any
	switch messageType {
	case MessageHello:
		payload = new(Hello)
	case MessageReady:
		payload = new(Ready)
	case MessageHeartbeat:
		payload = new(Heartbeat)
	case MessageStatus:
		payload = new(Status)
	case MessageLog:
		payload = new(Log)
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
	case *Status:
		return *p, nil
	case *Log:
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
