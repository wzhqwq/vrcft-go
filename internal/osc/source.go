package osc

import "sync"

// SnapshotSource is a simple thread-safe ValueSource. A production tracking
// pipeline may replace it with a dense fixed-array implementation.
type SnapshotSource struct {
	mu     sync.RWMutex
	floats map[string]float32
	bools  map[string]bool
}

func NewSnapshotSource() *SnapshotSource {
	return &SnapshotSource{
		floats: make(map[string]float32),
		bools:  make(map[string]bool),
	}
}

func (s *SnapshotSource) SetFloat(key string, value float32) {
	s.mu.Lock()
	s.floats[key] = value
	s.mu.Unlock()
}

func (s *SnapshotSource) SetBool(key string, value bool) {
	s.mu.Lock()
	s.bools[key] = value
	s.mu.Unlock()
}

func (s *SnapshotSource) Float(key string) (float32, bool) {
	s.mu.RLock()
	value, ok := s.floats[key]
	s.mu.RUnlock()
	return value, ok
}

func (s *SnapshotSource) Bool(key string) (bool, bool) {
	s.mu.RLock()
	value, ok := s.bools[key]
	s.mu.RUnlock()
	return value, ok
}
