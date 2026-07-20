package osc

import (
	"sync"

	"github.com/wzhqwq/vrcft-go/internal/parameters"
)

// SnapshotSource is a dense, thread-safe ValueSource keyed by ParameterID.
// Production code may replace it with an immutable parameter snapshot.
type SnapshotSource struct {
	mu sync.RWMutex

	floats     [parameters.ParameterCount]float32
	floatValid [parameters.ParameterCount]bool
	bools      [parameters.ParameterCount]bool
	boolValid  [parameters.ParameterCount]bool
}

func NewSnapshotSource() *SnapshotSource { return &SnapshotSource{} }

func (s *SnapshotSource) SetFloat(id parameters.ParameterID, value float32) bool {
	definition, ok := parameters.Definition(id)
	if !ok || definition.ValueType != parameters.ValueFloat {
		return false
	}
	s.mu.Lock()
	s.floats[id] = value
	s.floatValid[id] = true
	s.mu.Unlock()
	return true
}

func (s *SnapshotSource) SetBool(id parameters.ParameterID, value bool) bool {
	definition, ok := parameters.Definition(id)
	if !ok || definition.ValueType != parameters.ValueBool {
		return false
	}
	s.mu.Lock()
	s.bools[id] = value
	s.boolValid[id] = true
	s.mu.Unlock()
	return true
}

func (s *SnapshotSource) SetFloatByOSCName(name string, value float32) bool {
	id, ok := parameters.LookupOSCName(name)
	return ok && s.SetFloat(id, value)
}

func (s *SnapshotSource) SetBoolByOSCName(name string, value bool) bool {
	id, ok := parameters.LookupOSCName(name)
	return ok && s.SetBool(id, value)
}

func (s *SnapshotSource) Invalidate(id parameters.ParameterID) bool {
	if _, ok := parameters.Definition(id); !ok {
		return false
	}
	s.mu.Lock()
	s.floatValid[id] = false
	s.boolValid[id] = false
	s.mu.Unlock()
	return true
}

func (s *SnapshotSource) Float(id parameters.ParameterID) (float32, bool) {
	if id >= parameters.ParameterCount {
		return 0, false
	}
	s.mu.RLock()
	value, ok := s.floats[id], s.floatValid[id]
	s.mu.RUnlock()
	return value, ok
}

func (s *SnapshotSource) Bool(id parameters.ParameterID) (bool, bool) {
	if id >= parameters.ParameterCount {
		return false, false
	}
	s.mu.RLock()
	value, ok := s.bools[id], s.boolValid[id]
	s.mu.RUnlock()
	return value, ok
}
