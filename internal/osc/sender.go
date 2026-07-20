package osc

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

type ValueSource interface {
	Float(id parameters.ParameterID) (float32, bool)
	Bool(id parameters.ParameterID) (bool, bool)
}

type SenderConfig struct {
	FloatEpsilon float32
	MaxDatagram  int
	UseBundles   bool
}

type packetSender interface {
	Send([]byte) error
}

type cachedScalar struct {
	value scalarValue
	valid bool
}

type pendingScalar struct {
	index int
	value scalarValue
}

type ParameterSender struct {
	transport packetSender
	config    SenderConfig
	catalog   atomic.Pointer[Catalog]

	mu sync.Mutex

	last           []cachedScalar
	pending        []pendingScalar
	bundleBuilder  bundleBuilder
	messageBuilder messageBuilder
}

func NewParameterSender(transport *UDPTransport, config SenderConfig) *ParameterSender {
	return newParameterSender(transport, config)
}

func newParameterSender(transport packetSender, config SenderConfig) *ParameterSender {
	if config.FloatEpsilon <= 0 {
		config.FloatEpsilon = 0.001
	}
	if config.MaxDatagram <= 0 {
		config.MaxDatagram = 1200
	}
	return &ParameterSender{
		transport:     transport,
		config:        config,
		bundleBuilder: newBundleBuilder(config.MaxDatagram),
	}
}

func (sender *ParameterSender) SetCatalog(catalog *Catalog) {
	sender.mu.Lock()
	defer sender.mu.Unlock()

	previous := sender.catalog.Load()
	sender.catalog.Store(catalog)
	if previous == nil || catalog == nil || previous.Hash != catalog.Hash || len(sender.last) != len(catalog.Outputs) {
		if catalog == nil {
			sender.last = nil
			sender.pending = nil
			return
		}
		sender.last = make([]cachedScalar, len(catalog.Outputs))
		sender.pending = make([]pendingScalar, 0, len(catalog.Outputs))
	}
}

func (sender *ParameterSender) Catalog() *Catalog {
	return sender.catalog.Load()
}

func (sender *ParameterSender) ResetChangeDetection() {
	sender.mu.Lock()
	for index := range sender.last {
		sender.last[index].valid = false
	}
	sender.mu.Unlock()
}

func (sender *ParameterSender) Send(source ValueSource) error {
	if source == nil {
		return fmt.Errorf("OSC parameter source is nil")
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()

	catalog := sender.catalog.Load()
	if catalog == nil {
		return nil
	}
	if sender.transport == nil {
		return fmt.Errorf("OSC transport is nil")
	}

	if sender.config.UseBundles {
		return sender.sendBundle(catalog.Outputs, source)
	}
	return sender.sendStandalone(catalog.Outputs, source)
}

func (sender *ParameterSender) sendStandalone(outputs []outputBinding, source ValueSource) error {
	for index := range outputs {
		value, valid, err := evaluateOutput(outputs[index], source)
		if err != nil {
			return err
		}
		if !valid || !sender.changed(index, value) {
			continue
		}
		packet, err := sender.messageBuilder.encodeScalar(outputs[index].Address, value)
		if err != nil {
			return err
		}
		if err := sender.transport.Send(packet); err != nil {
			return err
		}
		sender.commit(index, value)
	}
	return nil
}

func (sender *ParameterSender) sendBundle(outputs []outputBinding, source ValueSource) error {
	sender.bundleBuilder.reset()
	sender.pending = sender.pending[:0]

	for index := range outputs {
		output := outputs[index]
		value, valid, err := evaluateOutput(output, source)
		if err != nil {
			return err
		}
		if !valid || !sender.changed(index, value) {
			continue
		}

		appended, err := sender.bundleBuilder.appendScalar(output.Address, value)
		if err != nil {
			return err
		}
		if !appended && !sender.bundleBuilder.empty() {
			if err := sender.flushBundle(); err != nil {
				return err
			}
			appended, err = sender.bundleBuilder.appendScalar(output.Address, value)
			if err != nil {
				return err
			}
		}
		if !appended {
			packet, err := sender.messageBuilder.encodeScalar(output.Address, value)
			if err != nil {
				return err
			}
			if err := sender.transport.Send(packet); err != nil {
				return err
			}
			sender.commit(index, value)
			continue
		}
		sender.pending = append(sender.pending, pendingScalar{index: index, value: value})
	}
	return sender.flushBundle()
}

func (sender *ParameterSender) flushBundle() error {
	if sender.bundleBuilder.empty() {
		return nil
	}
	if err := sender.transport.Send(sender.bundleBuilder.bytes()); err != nil {
		return err
	}
	for _, pending := range sender.pending {
		sender.commit(pending.index, pending.value)
	}
	sender.pending = sender.pending[:0]
	sender.bundleBuilder.reset()
	return nil
}

func (sender *ParameterSender) changed(index int, value scalarValue) bool {
	entry := sender.last[index]
	return !entry.valid || !scalarEqual(entry.value, value, sender.config.FloatEpsilon)
}

func (sender *ParameterSender) commit(index int, value scalarValue) {
	sender.last[index] = cachedScalar{value: value, valid: true}
}

func evaluateOutput(output outputBinding, source ValueSource) (scalarValue, bool, error) {
	switch output.Operation {
	case outputDirectFloat:
		value, valid := source.Float(output.Parameter)
		if !valid || !finite32(value) {
			return scalarValue{}, false, nil
		}
		value = clampOutput(output, value)
		return scalarFromFloat(output.WireKind, value)

	case outputDirectBool:
		value, valid := source.Bool(output.Parameter)
		if !valid {
			return scalarValue{}, false, nil
		}
		return scalarFromBool(output.WireKind, value)

	case outputBinaryNegative:
		value, valid := source.Float(output.Parameter)
		if !valid || !finite32(value) {
			return scalarValue{}, false, nil
		}
		return scalarFromBool(output.WireKind, clampOutput(output, value) < 0)

	case outputBinaryBit:
		value, valid := source.Float(output.Parameter)
		if !valid || !finite32(value) {
			return scalarValue{}, false, nil
		}
		value = clampOutput(output, value)
		spec := ParameterSpec{Range: output.Range, HasRange: output.HasRange}
		magnitude := binaryMagnitude(spec, value)
		quantized := uint32(math.Floor(float64(magnitude * float32(output.QuantizeMax+1))))
		if quantized > output.QuantizeMax {
			quantized = output.QuantizeMax
		}
		return scalarFromBool(output.WireKind, quantized&output.Weight != 0)

	default:
		return scalarValue{}, false, fmt.Errorf("%w: output operation %d for %s", ErrUnsupportedType, output.Operation, output.Address)
	}
}

func scalarFromFloat(kind scalarKind, value float32) (scalarValue, bool, error) {
	switch kind {
	case scalarFloat32:
		return floatScalar(value), true, nil
	case scalarInt32:
		return intScalar(int32(math.Round(float64(value)))), true, nil
	case scalarFalse, scalarTrue:
		return boolScalar(value >= 0.5), true, nil
	default:
		return scalarValue{}, false, fmt.Errorf("%w: scalar wire kind %d", ErrUnsupportedType, kind)
	}
}

func scalarFromBool(kind scalarKind, value bool) (scalarValue, bool, error) {
	switch kind {
	case scalarFloat32:
		if value {
			return floatScalar(1), true, nil
		}
		return floatScalar(0), true, nil
	case scalarInt32:
		if value {
			return intScalar(1), true, nil
		}
		return intScalar(0), true, nil
	case scalarFalse, scalarTrue:
		return boolScalar(value), true, nil
	default:
		return scalarValue{}, false, fmt.Errorf("%w: scalar wire kind %d", ErrUnsupportedType, kind)
	}
}

func clampOutput(output outputBinding, value float32) float32 {
	if !output.HasRange {
		return value
	}
	return clamp(value, output.Range.Min, output.Range.Max)
}

func finite32(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

func binaryMagnitude(spec ParameterSpec, value float32) float32 {
	if !spec.HasRange || spec.Range.Max <= spec.Range.Min {
		return clamp(float32(math.Abs(float64(value))), 0, 1)
	}
	if spec.Signed() {
		limit := float32(math.Max(math.Abs(float64(spec.Range.Min)), math.Abs(float64(spec.Range.Max))))
		if limit <= 0 {
			return 0
		}
		return clamp(float32(math.Abs(float64(value)))/limit, 0, 1)
	}
	return clamp((value-spec.Range.Min)/(spec.Range.Max-spec.Range.Min), 0, 1)
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
