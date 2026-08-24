package application

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/osc"
)

type LifecycleState string

const (
	LifecycleCreated  LifecycleState = "created"
	LifecycleStarting LifecycleState = "starting"
	LifecycleRunning  LifecycleState = "running"
	LifecycleDegraded LifecycleState = "degraded"
	LifecycleClosing  LifecycleState = "closing"
	LifecycleClosed   LifecycleState = "closed"
)

type PluginControlFailure struct {
	PluginID  string
	Operation string
	Message   string
}

type Status struct {
	Revision  uint64
	UpdatedAt time.Time
	Lifecycle LifecycleState

	AvatarID            string
	PlanGeneration      uint64
	PlanStatus          avatar.Status
	PlanSource          avatar.Source
	ConfigPath          string
	ConfigID            string
	GenerationExhausted bool

	OSC            osc.OSCStatus
	PluginFailures []PluginControlFailure
	PlanError      string
	RuntimeError   string
}

type statusStore struct {
	mu          sync.Mutex
	now         func() time.Time
	current     Status
	subscribers map[chan Status]struct{}
}

func newStatusStore(now func() time.Time) *statusStore {
	if now == nil {
		now = time.Now
	}
	return &statusStore{
		now: now,
		current: Status{
			Revision:  1,
			UpdatedAt: now(),
			Lifecycle: LifecycleCreated,
		},
		subscribers: make(map[chan Status]struct{}),
	}
}

func (store *statusStore) snapshot() Status {
	store.mu.Lock()
	defer store.mu.Unlock()
	return cloneStatus(store.current)
}

func (store *statusStore) update(update func(*Status)) Status {
	store.mu.Lock()
	defer store.mu.Unlock()

	next := cloneStatus(store.current)
	if update != nil {
		update(&next)
	}
	next.Revision = nextRevision(store.current.Revision)
	next.UpdatedAt = store.now()
	store.current = cloneStatus(next)
	store.publishLocked()
	return cloneStatus(store.current)
}

func (store *statusStore) subscribe(ctx context.Context) <-chan Status {
	if ctx == nil {
		ctx = context.Background()
	}
	values := make(chan Status, 1)
	store.mu.Lock()
	store.subscribers[values] = struct{}{}
	store.offerLatestLocked(values)
	store.mu.Unlock()

	go func() {
		<-ctx.Done()
		store.mu.Lock()
		if _, ok := store.subscribers[values]; ok {
			delete(store.subscribers, values)
			close(values)
		}
		store.mu.Unlock()
	}()
	return values
}

func (store *statusStore) publishLocked() {
	for subscriber := range store.subscribers {
		store.offerLatestLocked(subscriber)
	}
}

func (store *statusStore) offerLatestLocked(subscriber chan Status) {
	select {
	case <-subscriber:
	default:
	}
	select {
	case subscriber <- cloneStatus(store.current):
	default:
	}
}

func nextRevision(current uint64) uint64 {
	if current == 0 {
		return 1
	}
	if current == math.MaxUint64 {
		return math.MaxUint64
	}
	return current + 1
}

func cloneStatus(status Status) Status {
	status.PluginFailures = append([]PluginControlFailure(nil), status.PluginFailures...)
	return status
}
