package main

import (
	"context"
	"math"
	"sync"
	"time"
)

// moduleEnvelope is intentionally package-private: generic values are never
// bound through Wails. Concrete response DTOs copy its metadata instead.
type moduleEnvelope[T any] struct {
	Revision  uint64    `json:"revision"`
	UpdatedAt time.Time `json:"updatedAt"`
	Value     T         `json:"value"`
	Problem   *Problem  `json:"problem,omitempty"`
}

type moduleStore[T any] struct {
	mu          sync.Mutex
	now         func() time.Time
	clone       func(T) T
	current     moduleEnvelope[T]
	subscribers map[chan moduleEnvelope[T]]struct{}
}

func newModuleStore[T any](initial T, clone func(T) T, now func() time.Time) *moduleStore[T] {
	if clone == nil {
		clone = func(value T) T { return value }
	}
	if now == nil {
		now = time.Now
	}
	return &moduleStore[T]{
		now:   now,
		clone: clone,
		current: moduleEnvelope[T]{
			Revision:  1,
			UpdatedAt: now(),
			Value:     clone(initial),
		},
		subscribers: make(map[chan moduleEnvelope[T]]struct{}),
	}
}

func (store *moduleStore[T]) snapshot() moduleEnvelope[T] {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.cloneEnvelope(store.current)
}

func (store *moduleStore[T]) update(value T, problem *Problem) moduleEnvelope[T] {
	store.mu.Lock()
	defer store.mu.Unlock()

	updatedAt := store.now()
	if updatedAt.Before(store.current.UpdatedAt) {
		updatedAt = store.current.UpdatedAt
	}
	store.current = moduleEnvelope[T]{
		Revision:  nextModuleRevision(store.current.Revision),
		UpdatedAt: updatedAt,
		Value:     store.clone(value),
		Problem:   cloneProblem(problem),
	}
	store.publishLocked()
	return store.cloneEnvelope(store.current)
}

func (store *moduleStore[T]) subscribe(ctx context.Context) <-chan moduleEnvelope[T] {
	if ctx == nil {
		ctx = context.Background()
	}
	updates := make(chan moduleEnvelope[T], 1)
	store.mu.Lock()
	store.subscribers[updates] = struct{}{}
	store.offerLatestLocked(updates)
	store.mu.Unlock()

	go func() {
		<-ctx.Done()
		store.mu.Lock()
		if _, ok := store.subscribers[updates]; ok {
			delete(store.subscribers, updates)
			close(updates)
		}
		store.mu.Unlock()
	}()
	return updates
}

func (store *moduleStore[T]) publishLocked() {
	for subscriber := range store.subscribers {
		store.offerLatestLocked(subscriber)
	}
}

func (store *moduleStore[T]) offerLatestLocked(subscriber chan moduleEnvelope[T]) {
	select {
	case <-subscriber:
	default:
	}
	select {
	case subscriber <- store.cloneEnvelope(store.current):
	default:
	}
}

func (store *moduleStore[T]) cloneEnvelope(value moduleEnvelope[T]) moduleEnvelope[T] {
	value.Value = store.clone(value.Value)
	value.Problem = cloneProblem(value.Problem)
	return value
}

func nextModuleRevision(current uint64) uint64 {
	if current == 0 {
		return 1
	}
	if current == math.MaxUint64 {
		return math.MaxUint64
	}
	return current + 1
}

func cloneProblem(problem *Problem) *Problem {
	if problem == nil {
		return nil
	}
	clone := *problem
	return &clone
}
