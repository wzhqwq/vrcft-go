package osc

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
)

type ValueSource interface {
	Float(key string) (float32, bool)
	Bool(key string) (bool, bool)
}

type SenderConfig struct {
	FloatEpsilon float32
	MaxDatagram  int
	UseBundles   bool
}

type ParameterSender struct {
	transport *UDPTransport
	config    SenderConfig
	catalog   atomic.Pointer[Catalog]

	mu   sync.Mutex
	last map[string]Value
}

func NewParameterSender(transport *UDPTransport, config SenderConfig) *ParameterSender {
	if config.FloatEpsilon <= 0 {
		config.FloatEpsilon = 0.001
	}
	if config.MaxDatagram <= 0 {
		config.MaxDatagram = 1200
	}
	return &ParameterSender{
		transport: transport,
		config:    config,
		last:      make(map[string]Value),
	}
}

func (s *ParameterSender) SetCatalog(catalog *Catalog) {
	previous := s.catalog.Swap(catalog)
	if previous == nil || catalog == nil || previous.Hash != catalog.Hash {
		s.mu.Lock()
		clear(s.last)
		s.mu.Unlock()
	}
}

func (s *ParameterSender) Catalog() *Catalog { return s.catalog.Load() }

func (s *ParameterSender) ResetChangeDetection() {
	s.mu.Lock()
	clear(s.last)
	s.mu.Unlock()
}

func (s *ParameterSender) Send(source ValueSource) error {
	if source == nil {
		return fmt.Errorf("OSC parameter source is nil")
	}
	catalog := s.catalog.Load()
	if catalog == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	keys := make([]string, 0, len(catalog.Bindings))
	for key := range catalog.Bindings {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	messages := make([]Message, 0, len(keys))
	for _, key := range keys {
		binding := catalog.Bindings[key]
		switch binding.Spec.Class {
		case ParameterFloat:
			value, valid := source.Float(key)
			if !valid || math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				continue
			}
			if !binding.Spec.Unbounded {
				if binding.Spec.Signed {
					value = clamp(value, -1, 1)
				} else {
					value = clamp(value, 0, 1)
				}
			}
			for _, endpoint := range binding.Direct {
				if message, ok := s.changedMessage(endpoint, floatToEndpoint(endpoint.Type, value)); ok {
					messages = append(messages, message)
				}
			}
			for _, binary := range binding.Binary {
				messages = append(messages, s.binaryMessages(binding.Spec, binary, value)...)
			}
		case ParameterBool:
			value, valid := source.Bool(key)
			if !valid {
				continue
			}
			for _, endpoint := range binding.Direct {
				if message, ok := s.changedMessage(endpoint, boolToEndpoint(endpoint.Type, value)); ok {
					messages = append(messages, message)
				}
			}
		}
	}

	if len(messages) == 0 {
		return nil
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].Address < messages[j].Address })
	return s.sendMessages(messages)
}

func (s *ParameterSender) binaryMessages(spec ParameterSpec, binding BinaryBinding, value float32) []Message {
	var messages []Message
	if spec.Signed && binding.Negative != nil {
		if message, ok := s.changedMessage(*binding.Negative, boolToEndpoint(binding.Negative.Type, value < 0)); ok {
			messages = append(messages, message)
		}
	}

	magnitude := float32(math.Abs(float64(value)))
	var max uint32
	for _, bit := range binding.Bits {
		max |= bit.Weight
	}
	if max == 0 {
		return messages
	}
	quantized := uint32(math.Floor(float64(magnitude * float32(max+1))))
	if quantized > max {
		quantized = max
	}
	for _, bit := range binding.Bits {
		set := quantized&bit.Weight != 0
		if message, ok := s.changedMessage(bit.Endpoint, boolToEndpoint(bit.Endpoint.Type, set)); ok {
			messages = append(messages, message)
		}
	}
	return messages
}

func (s *ParameterSender) changedMessage(endpoint Endpoint, value Value) (Message, bool) {
	previous, exists := s.last[endpoint.Address]
	if exists && valuesEqual(previous, value, s.config.FloatEpsilon) {
		return Message{}, false
	}
	s.last[endpoint.Address] = value
	return Message{Address: endpoint.Address, Args: []Value{value}}, true
}

func (s *ParameterSender) sendMessages(messages []Message) error {
	if !s.config.UseBundles {
		for _, message := range messages {
			packet, err := MarshalMessage(message)
			if err != nil {
				return err
			}
			if err := s.transport.Send(packet); err != nil {
				return err
			}
		}
		return nil
	}

	var elements [][]byte
	currentSize := 16 // #bundle + timetag
	flush := func() error {
		if len(elements) == 0 {
			return nil
		}
		packet, err := MarshalBundle(Bundle{Timetag: 1, Elements: elements})
		if err != nil {
			return err
		}
		if err := s.transport.Send(packet); err != nil {
			return err
		}
		elements = nil
		currentSize = 16
		return nil
	}

	for _, message := range messages {
		packet, err := MarshalMessage(message)
		if err != nil {
			return err
		}
		elementSize := 4 + len(packet)
		if len(elements) > 0 && currentSize+elementSize > s.config.MaxDatagram {
			if err := flush(); err != nil {
				return err
			}
		}
		if 16+elementSize > s.config.MaxDatagram {
			// A single oversized message cannot benefit from a bundle.
			if err := s.transport.Send(packet); err != nil {
				return err
			}
			continue
		}
		elements = append(elements, packet)
		currentSize += elementSize
	}
	return flush()
}

func floatToEndpoint(typ string, value float32) Value {
	switch typ {
	case "i":
		return Int32(int32(math.Round(float64(value))))
	case "T", "F":
		return Bool(value >= 0.5)
	default:
		return Float32(value)
	}
}

func boolToEndpoint(typ string, value bool) Value {
	switch typ {
	case "i":
		if value {
			return Int32(1)
		}
		return Int32(0)
	case "f":
		if value {
			return Float32(1)
		}
		return Float32(0)
	default:
		return Bool(value)
	}
}

func valuesEqual(left, right Value, epsilon float32) bool {
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case ValueFloat32:
		return float32(math.Abs(float64(left.F32-right.F32))) <= epsilon
	case ValueInt32:
		return left.I32 == right.I32
	case ValueString:
		return left.Str == right.Str
	case ValueBool:
		return left.Bool == right.Bool
	default:
		return false
	}
}

func clamp(value, minValue, maxValue float32) float32 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
