package protocol

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type fakeConn struct{}

func (fakeConn) Send(context.Context, Message) error      { return nil }
func (fakeConn) Receive(context.Context) (Message, error) { return Message{}, nil }
func (fakeConn) Close() error                             { return nil }

var _ Conn = fakeConn{}

func validDescriptor() pluginapi.Descriptor {
	return pluginapi.Descriptor{
		APIVersion:   pluginapi.APIVersion,
		ID:           "test.driver",
		Name:         "Test Driver",
		Version:      "1.2.3",
		Capabilities: trackingmodel.CapabilityEye,
	}
}

func validSubscription() pluginapi.Subscription {
	return pluginapi.Subscription{
		Generation:   1,
		Capabilities: trackingmodel.CapabilityEye,
	}
}

func TestMessageTypeWireOrder(t *testing.T) {
	want := []MessageType{
		MessageHello,
		MessageInitialize,
		MessageReady,
		MessageHeartbeat,
		MessageTrackingFrame,
		MessageStatus,
		MessageLog,
		MessageConfigChanged,
		MessageSubscriptionChanged,
		MessageActiveChanged,
		MessageShutdown,
		MessageShutdownAck,
		MessageError,
	}
	for i, got := range want {
		if got != MessageType(i+1) {
			t.Fatalf("message type at index %d = %d, want %d", i, got, i+1)
		}
	}
}

func TestMessageRoundTrips(t *testing.T) {
	config := pluginapi.Config{Revision: 2, Data: json.RawMessage(`{"gain":0.5}`)}
	frame := trackingmodel.TrackingFrame{
		Sequence:     7,
		TimestampNS:  12,
		Capabilities: trackingmodel.CapabilityEye,
		Eye: trackingmodel.EyeSample{
			Valid:        trackingmodel.EyeValidLeftOpenness,
			LeftOpenness: 0.75,
		},
	}
	tests := []struct {
		name     string
		payload  any
		wantType MessageType
	}{
		{"hello", Hello{Token: "secret", Descriptor: validDescriptor(), ProtocolMin: Version, ProtocolMax: Version}, MessageHello},
		{"initialize", Initialize{Startup: pluginapi.Startup{Active: true, Config: config, Subscription: validSubscription()}}, MessageInitialize},
		{"ready", Ready{}, MessageReady},
		{"heartbeat", Heartbeat{UptimeMS: 0}, MessageHeartbeat},
		{"tracking frame", TrackingFrame{Generation: 1, Frame: frame}, MessageTrackingFrame},
		{"status", Status{Status: pluginapi.DeviceStatus{State: pluginapi.DeviceReady}}, MessageStatus},
		{"log", Log{Level: pluginapi.LogInfo, Message: "started", Dropped: 3}, MessageLog},
		{"log zero dropped", Log{Level: pluginapi.LogInfo, Message: "started", Dropped: 0}, MessageLog},
		{"config changed", ConfigChanged{Config: config}, MessageConfigChanged},
		{"subscription changed", SubscriptionChanged{Subscription: validSubscription()}, MessageSubscriptionChanged},
		{"active changed false", ActiveChanged{Active: false}, MessageActiveChanged},
		{"active changed true", ActiveChanged{Active: true}, MessageActiveChanged},
		{"shutdown", Shutdown{}, MessageShutdown},
		{"shutdown ack", ShutdownAck{}, MessageShutdownAck},
		{"error", Error{Code: "bad_state", Message: "not ready"}, MessageError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, err := NewMessage(tt.payload)
			if err != nil {
				t.Fatalf("NewMessage() error = %v", err)
			}
			if message.Version != Version || message.Type != tt.wantType {
				t.Fatalf("NewMessage() header = (%d, %d), want (%d, %d)", message.Version, message.Type, Version, tt.wantType)
			}

			data, err := json.Marshal(message)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			var decoded Message
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if reflect.TypeOf(decoded.Payload) != reflect.TypeOf(tt.payload) {
				t.Fatalf("decoded payload type = %T, want %T", decoded.Payload, tt.payload)
			}
			if !reflect.DeepEqual(decoded, message) {
				t.Fatalf("decoded message = %#v, want %#v", decoded, message)
			}
		})
	}
}

func TestDefaultInitializeRoundTrips(t *testing.T) {
	message, err := NewMessage(Initialize{})
	if err != nil {
		t.Fatalf("NewMessage() error = %v", err)
	}
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, message) {
		t.Fatalf("decoded message = %#v, want %#v", decoded, message)
	}
}

func TestExplicitNullConfigDataRoundTrips(t *testing.T) {
	payload := ConfigChanged{Config: pluginapi.Config{Revision: 1, Data: json.RawMessage("null")}}
	message, err := NewMessage(payload)
	if err != nil {
		t.Fatalf("NewMessage() error = %v", err)
	}
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, message) {
		t.Fatalf("decoded message = %#v, want %#v", decoded, message)
	}
}

func TestZeroLengthConfigDataRoundTripsAsCanonicalNil(t *testing.T) {
	payload := ConfigChanged{Config: pluginapi.Config{Revision: 1, Data: json.RawMessage{}}}
	message, err := NewMessage(payload)
	if err != nil {
		t.Fatalf("NewMessage() error = %v", err)
	}
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := decoded.Payload.(ConfigChanged).Config.Data; got != nil {
		t.Fatalf("decoded Config.Data = %#v, want canonical nil", got)
	}
}

func TestNewMessageRejectsUnknownAndPointerPayloads(t *testing.T) {
	tests := []struct {
		name    string
		payload any
	}{
		{"unknown", struct{ Value string }{Value: "x"}},
		{"pointer", &Ready{}},
		{"nil", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewMessage(tt.payload); err == nil {
				t.Fatal("NewMessage() error = nil")
			}
		})
	}
}

func TestMessageValidateRejectsHeaderAndPayloadMismatch(t *testing.T) {
	tests := []struct {
		name    string
		message Message
	}{
		{"wrong version", Message{Version: Version + 1, Type: MessageReady, Payload: Ready{}}},
		{"unknown type", Message{Version: Version, Type: MessageType(99), Payload: Ready{}}},
		{"mismatched payload", Message{Version: Version, Type: MessageReady, Payload: Shutdown{}}},
		{"pointer payload", Message{Version: Version, Type: MessageReady, Payload: &Ready{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.message.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
			if _, err := json.Marshal(tt.message); err == nil {
				t.Fatal("Marshal() error = nil")
			}
		})
	}
}

func TestPayloadValidation(t *testing.T) {
	badDescriptor := validDescriptor()
	badDescriptor.ID = ""
	expressionTail := trackingmodel.ExpressionMask{}
	expressionTail.Words[len(expressionTail.Words)-1] = uint64(1) << (trackingmodel.ExpressionCount % 64)
	tests := []struct {
		name    string
		payload any
	}{
		{"hello blank token", Hello{Token: " \t", Descriptor: validDescriptor(), ProtocolMin: Version, ProtocolMax: Version}},
		{"hello descriptor", Hello{Token: "secret", Descriptor: badDescriptor, ProtocolMin: Version, ProtocolMax: Version}},
		{"hello protocol range", Hello{Token: "secret", Descriptor: validDescriptor(), ProtocolMin: 0, ProtocolMax: Version}},
		{"initialize config", Initialize{Startup: pluginapi.Startup{Config: pluginapi.Config{Revision: 1, Data: json.RawMessage(`{`)}}}},
		{"initialize subscription", Initialize{Startup: pluginapi.Startup{Active: true}}},
		{"tracking unknown capability", TrackingFrame{Generation: 1, Frame: trackingmodel.TrackingFrame{Capabilities: trackingmodel.Capability(1 << 30)}}},
		{"tracking unknown eye validity", TrackingFrame{Generation: 1, Frame: trackingmodel.TrackingFrame{Capabilities: trackingmodel.CapabilityEye, Eye: trackingmodel.EyeSample{Valid: trackingmodel.EyeValid(1 << 15)}}}},
		{"tracking expression tail", TrackingFrame{Generation: 1, Frame: trackingmodel.TrackingFrame{Capabilities: trackingmodel.CapabilityExpression, Expressions: trackingmodel.ExpressionSet{Valid: expressionTail}}}},
		{"tracking eye validity without capability", TrackingFrame{Generation: 1, Frame: trackingmodel.TrackingFrame{Eye: trackingmodel.EyeSample{Valid: trackingmodel.EyeValidLeftGaze}}}},
		{"tracking expression validity without capability", TrackingFrame{Generation: 1, Frame: trackingmodel.TrackingFrame{Expressions: trackingmodel.ExpressionSet{Valid: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen)}}}},
		{"status", Status{Status: pluginapi.DeviceStatus{State: "unknown"}}},
		{"log level", Log{Level: "verbose", Message: "message"}},
		{"log blank message", Log{Level: pluginapi.LogInfo, Message: " \n"}},
		{"config invalid", ConfigChanged{Config: pluginapi.Config{Revision: 1, Data: json.RawMessage(`{`)}}},
		{"config zero revision", ConfigChanged{Config: pluginapi.Config{Data: nil}}},
		{"subscription inactive", SubscriptionChanged{}},
		{"error blank code", Error{Message: "message"}},
		{"error blank message", Error{Code: "code", Message: " \r"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewMessage(tt.payload); err == nil {
				t.Fatal("NewMessage() error = nil")
			}
		})
	}
}

func TestTrackingFrameGenerationZeroIsRejectedButDropoutIsValid(t *testing.T) {
	if _, err := NewMessage(TrackingFrame{}); err == nil {
		t.Fatal("zero generation error = nil")
	}
	if _, err := NewMessage(TrackingFrame{Generation: 1}); err != nil {
		t.Fatalf("dropout frame error = %v", err)
	}
}

func TestJSONUnmarshalRejectsInvalidEnvelopeAndPayload(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"unknown type", `{"version":1,"type":99,"payload":{}}`},
		{"wrong version", `{"version":2,"type":3,"payload":{}}`},
		{"unknown payload field", `{"version":1,"type":3,"payload":{"extra":true}}`},
		{"unknown envelope field", `{"version":1,"type":3,"payload":{},"extra":true}`},
		{"null payload", `{"version":1,"type":3,"payload":null}`},
		{"malformed public value", `{"version":1,"type":6,"payload":{"status":{"State":"unknown","Message":""}}}`},
		{"trailing JSON", `{"version":1,"type":3,"payload":{}} {}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var message Message
			if err := json.Unmarshal([]byte(tt.data), &message); err == nil {
				t.Fatal("Unmarshal() error = nil")
			}
		})
	}
}

func TestJSONUnmarshalRequiresExactTrackingArrayWidths(t *testing.T) {
	maskWidth := len((trackingmodel.ExpressionMask{}).Words)
	valueWidth := int(trackingmodel.ExpressionCount)
	exactMask := numericJSONArray(maskWidth)
	exactValues := numericJSONArray(valueWidth)

	tests := []struct {
		name string
		data string
	}{
		{
			name: "initialize short expression mask",
			data: `{"version":1,"type":2,"payload":{"startup":{"Active":false,"Config":{"Revision":0},"Subscription":{"Generation":0,"Capabilities":0,"Eye":0,"Expressions":{"Words":` + numericJSONArray(maskWidth-1) + `}}}}}`,
		},
		{
			name: "initialize long expression mask",
			data: `{"version":1,"type":2,"payload":{"startup":{"Active":false,"Config":{"Revision":0},"Subscription":{"Generation":0,"Capabilities":0,"Eye":0,"Expressions":{"Words":` + numericJSONArray(maskWidth+1) + `}}}}}`,
		},
		{
			name: "subscription short expression mask",
			data: `{"version":1,"type":9,"payload":{"subscription":{"Generation":1,"Capabilities":2,"Eye":0,"Expressions":{"Words":` + numericJSONArray(maskWidth-1) + `}}}}`,
		},
		{
			name: "subscription long expression mask",
			data: `{"version":1,"type":9,"payload":{"subscription":{"Generation":1,"Capabilities":2,"Eye":0,"Expressions":{"Words":` + numericJSONArray(maskWidth+1) + `}}}}`,
		},
		{
			name: "tracking frame short expression validity",
			data: `{"version":1,"type":5,"payload":{"generation":1,"frame":{"Capabilities":2,"Expressions":{"Values":` + exactValues + `,"Valid":{"Words":` + numericJSONArray(maskWidth-1) + `}}}}}`,
		},
		{
			name: "tracking frame long expression validity",
			data: `{"version":1,"type":5,"payload":{"generation":1,"frame":{"Capabilities":2,"Expressions":{"Values":` + exactValues + `,"Valid":{"Words":` + numericJSONArray(maskWidth+1) + `}}}}}`,
		},
		{
			name: "tracking frame short expression values",
			data: `{"version":1,"type":5,"payload":{"generation":1,"frame":{"Capabilities":2,"Expressions":{"Values":` + numericJSONArray(valueWidth-1) + `,"Valid":{"Words":` + exactMask + `}}}}}`,
		},
		{
			name: "tracking frame long expression values",
			data: `{"version":1,"type":5,"payload":{"generation":1,"frame":{"Capabilities":2,"Expressions":{"Values":` + numericJSONArray(valueWidth+1) + `,"Valid":{"Words":` + exactMask + `}}}}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var message Message
			if err := json.Unmarshal([]byte(tt.data), &message); err == nil {
				t.Fatal("Unmarshal() error = nil, want exact-width rejection")
			}
		})
	}
}

func TestJSONPayloadSizeLimit(t *testing.T) {
	base, err := json.Marshal(Log{Level: pluginapi.LogInfo, Message: "x"})
	if err != nil {
		t.Fatal(err)
	}
	exact := Log{Level: pluginapi.LogInfo, Message: strings.Repeat("x", MaxPayloadSize-len(base)+1)}
	exactPayload, err := json.Marshal(exact)
	if err != nil {
		t.Fatal(err)
	}
	if len(exactPayload) != MaxPayloadSize {
		t.Fatalf("test payload length = %d, want %d", len(exactPayload), MaxPayloadSize)
	}

	exactMessage := Message{Version: Version, Type: MessageLog, Payload: exact}
	if err := exactMessage.Validate(); err != nil {
		t.Fatalf("Validate(exact limit) error = %v", err)
	}
	if _, err := NewMessage(exact); err != nil {
		t.Fatalf("NewMessage(exact limit) error = %v", err)
	}
	data, err := json.Marshal(exactMessage)
	if err != nil {
		t.Fatalf("Marshal(exact limit) error = %v", err)
	}
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal(exact limit) error = %v", err)
	}

	tooLarge := exact
	tooLarge.Message += "x"
	tooLargeMessage := Message{Version: Version, Type: MessageLog, Payload: tooLarge}
	if err := tooLargeMessage.Validate(); err == nil {
		t.Fatal("Validate(over limit) error = nil")
	}
	if _, err := NewMessage(tooLarge); err == nil {
		t.Fatal("NewMessage(over limit) error = nil")
	}
	if _, err := json.Marshal(tooLargeMessage); err == nil {
		t.Fatal("Marshal(over limit) error = nil")
	}

	wire := []byte(`{"version":1,"type":7,"payload":` + string(mustJSON(t, tooLarge)) + `}`)
	if len(mustJSON(t, tooLarge)) <= MaxPayloadSize {
		t.Fatal("test setup did not exceed payload limit")
	}
	if err := json.Unmarshal(wire, &decoded); err == nil {
		t.Fatal("Unmarshal(over payload limit) error = nil")
	}
}

func TestJSONTotalSizeLimit(t *testing.T) {
	data := []byte(`{"version":1,"type":3,"payload":{},"padding":"` + strings.Repeat("x", MaxPayloadSize+messageEnvelopeAllowance) + `"}`)
	if len(data) <= MaxPayloadSize+messageEnvelopeAllowance {
		t.Fatal("test setup did not exceed total limit")
	}
	var message Message
	if err := json.Unmarshal(data, &message); err == nil {
		t.Fatal("Unmarshal(over total limit) error = nil")
	}
}

func TestJSONDecodeOwnsConfigData(t *testing.T) {
	input := []byte(`{"version":1,"type":8,"payload":{"config":{"Revision":1,"Data":{"gain":0.5}}}}`)
	original := append([]byte(nil), input...)
	var message Message
	if err := json.Unmarshal(input, &message); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for i := range input {
		input[i] = 'x'
	}
	config := message.Payload.(ConfigChanged).Config
	if string(config.Data) != `{"gain":0.5}` {
		t.Fatalf("decoded config data after source mutation = %q (input was %q)", config.Data, original)
	}
}

func TestJSONDecodeOwnsInitializeConfigData(t *testing.T) {
	input := []byte(`{"version":1,"type":2,"payload":{"startup":{"Active":false,"Config":{"Revision":1,"Data":{"gain":0.5}},"Subscription":{"Generation":0,"Capabilities":0,"Eye":0,"Expressions":{"Words":[0,0]}}}}}`)
	var message Message
	if err := json.Unmarshal(input, &message); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for i := range input {
		input[i] = 'x'
	}
	config := message.Payload.(Initialize).Startup.Config
	if string(config.Data) != `{"gain":0.5}` {
		t.Fatalf("decoded config data after source mutation = %q", config.Data)
	}
}

func TestUnmarshalDoesNotModifyReceiverOnError(t *testing.T) {
	want, err := NewMessage(Ready{})
	if err != nil {
		t.Fatal(err)
	}
	got := want
	err = json.Unmarshal([]byte(`{"version":1,"type":3,"payload":{"extra":true}}`), &got)
	if err == nil {
		t.Fatal("Unmarshal() error = nil")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("receiver changed on error: got %#v, want %#v", got, want)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal(%T) error = %v", value, err)
	}
	return data
}

func numericJSONArray(length int) string {
	if length == 0 {
		return "[]"
	}
	return "[" + strings.TrimSuffix(strings.Repeat("0,", length), ",") + "]"
}
