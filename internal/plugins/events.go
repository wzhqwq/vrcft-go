package plugins

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

type EventType string

const (
	EventPluginDiscovered   EventType = "plugin_discovered"
	EventPluginRemoved      EventType = "plugin_removed"
	EventPluginStateChanged EventType = "plugin_state_changed"
	EventPluginStatus       EventType = "plugin_status"
	EventPluginLog          EventType = "plugin_log"
)

type Event struct {
	Sequence uint64
	Time     time.Time
	Type     EventType
	PluginID string
	Snapshot *RuntimeSnapshot
	Status   *pluginapi.DeviceStatus
	Log      *pluginapi.LogEntry
	Dropped  uint64
}

func (e Event) clone() Event {
	clone := e
	if e.Snapshot != nil {
		snapshot := e.Snapshot.clone()
		clone.Snapshot = &snapshot
	}
	if e.Status != nil {
		status := *e.Status
		clone.Status = &status
	}
	if e.Log != nil {
		log := *e.Log
		clone.Log = &log
	}
	return clone
}

type eventHub struct {
	capacity  int
	publish   chan Event
	subscribe chan subscribeRequest
	remove    chan *eventSubscriber
	done      chan struct{}
	stopped   chan struct{}
	closed    sync.Once
}

type subscribeRequest struct {
	ctx   context.Context
	reply chan chan Event
}

type eventSubscriber struct {
	out        chan Event
	notices    []Event
	logs       []Event
	states     map[string]Event
	stateOrder []string
	droppedLog uint64
}

func newEventHub(capacity int) *eventHub {
	if capacity < 1 {
		capacity = 1
	}
	hub := &eventHub{
		capacity:  capacity,
		publish:   make(chan Event, maxEventCommands(capacity)),
		subscribe: make(chan subscribeRequest, capacity),
		remove:    make(chan *eventSubscriber, capacity),
		done:      make(chan struct{}),
		stopped:   make(chan struct{}),
	}
	go hub.run()
	return hub
}

func maxEventCommands(capacity int) int {
	const minimum = 64
	if capacity > minimum/4 {
		return capacity * 4
	}
	return minimum
}

// Publish never waits for a subscriber. A false result means the bounded hub
// command queue is full or the hub has been closed.
func (h *eventHub) Publish(event Event) bool {
	event = event.clone()
	select {
	case <-h.done:
		return false
	default:
	}
	select {
	case h.publish <- event:
		return true
	case <-h.done:
		return false
	default:
		return false
	}
}

func (h *eventHub) Subscribe(ctx context.Context) <-chan Event {
	if ctx == nil {
		ctx = context.Background()
	}
	result := make(chan chan Event, 1)
	request := subscribeRequest{ctx: ctx, reply: result}
	select {
	case <-h.done:
		closed := make(chan Event)
		close(closed)
		return closed
	case h.subscribe <- request:
	}
	select {
	case events := <-result:
		return events
	case <-h.done:
		closed := make(chan Event)
		close(closed)
		return closed
	}
}

func (h *eventHub) Close() {
	h.closed.Do(func() { close(h.done) })
	<-h.stopped
}

func (h *eventHub) run() {
	defer close(h.stopped)
	subscribers := make(map[*eventSubscriber]struct{})
	var sequence uint64
	for {
		select {
		case <-h.done:
			for subscriber := range subscribers {
				close(subscriber.out)
			}
			return
		default:
		}
		// A publish burst may keep the command channel nonempty indefinitely.
		// Limit this round to the queue's entry snapshot so subscriptions,
		// cancellation, and Close remain live under sustained publication.
		quota := len(h.publish)
	drain:
		for range quota {
			select {
			case <-h.done:
				for subscriber := range subscribers {
					close(subscriber.out)
				}
				return
			case event := <-h.publish:
				h.dispatch(event, subscribers, &sequence)
			default:
				break drain
			}
		}

		cases, targets := h.selectCases(subscribers)
		chosen, value, ok := reflect.Select(cases)
		switch chosen {
		case 0:
			for subscriber := range subscribers {
				close(subscriber.out)
			}
			return
		case 1:
			if !ok {
				return
			}
			h.dispatch(value.Interface().(Event), subscribers, &sequence)
		case 2:
			request := value.Interface().(subscribeRequest)
			if request.ctx.Err() != nil {
				closed := make(chan Event)
				close(closed)
				request.reply <- closed
				continue
			}
			subscriber := &eventSubscriber{out: make(chan Event), states: make(map[string]Event)}
			subscribers[subscriber] = struct{}{}
			request.reply <- subscriber.out
			go h.watchContext(request.ctx, subscriber)
		case 3:
			subscriber := value.Interface().(*eventSubscriber)
			if _, exists := subscribers[subscriber]; exists {
				delete(subscribers, subscriber)
				close(subscriber.out)
			}
		default:
			target := targets[chosen-4]
			target.subscriber.pop(target.event)
		}
	}
}

func (h *eventHub) dispatch(event Event, subscribers map[*eventSubscriber]struct{}, sequence *uint64) {
	*sequence++
	event.Sequence = *sequence
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	for subscriber := range subscribers {
		subscriber.enqueue(event, h.capacity)
	}
}

type eventTarget struct {
	subscriber *eventSubscriber
	event      Event
}

func (h *eventHub) selectCases(subscribers map[*eventSubscriber]struct{}) ([]reflect.SelectCase, []eventTarget) {
	cases := []reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(h.done)},
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(h.publish)},
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(h.subscribe)},
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(h.remove)},
	}
	targets := make([]eventTarget, 0, len(subscribers))
	for subscriber := range subscribers {
		if event, exists := subscriber.next(); exists {
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectSend, Chan: reflect.ValueOf(subscriber.out), Send: reflect.ValueOf(event.clone())})
			targets = append(targets, eventTarget{subscriber: subscriber, event: event})
		}
	}
	return cases, targets
}

func (h *eventHub) watchContext(ctx context.Context, subscriber *eventSubscriber) {
	select {
	case <-ctx.Done():
		select {
		case h.remove <- subscriber:
		case <-h.done:
		}
	case <-h.done:
	}
}

func (s *eventSubscriber) enqueue(event Event, capacity int) {
	switch event.Type {
	case EventPluginStateChanged, EventPluginStatus:
		key := event.PluginID + "\x00" + string(event.Type)
		if _, exists := s.states[key]; !exists {
			if len(s.stateOrder) == capacity {
				delete(s.states, s.stateOrder[0])
				s.stateOrder = s.stateOrder[1:]
			}
			s.stateOrder = append(s.stateOrder, key)
		}
		s.states[key] = event.clone()
	case EventPluginLog:
		if len(s.logs) >= capacity {
			s.droppedLog = saturatingAddUint64(
				s.droppedLog,
				saturatingAddUint64(event.Dropped, 1),
			)
			return
		}
		if s.droppedLog != 0 {
			event.Dropped = saturatingAddUint64(event.Dropped, s.droppedLog)
			s.droppedLog = 0
		}
		s.logs = append(s.logs, event.clone())
	default:
		if len(s.notices) < capacity {
			s.notices = append(s.notices, event.clone())
		}
	}
}

func (s *eventSubscriber) next() (Event, bool) {
	var next Event
	hasNext := false
	if len(s.logs) != 0 {
		next = s.logs[0]
		hasNext = true
	}
	if len(s.notices) != 0 && (!hasNext || s.notices[0].Sequence < next.Sequence) {
		next = s.notices[0]
		hasNext = true
	}
	for len(s.stateOrder) != 0 {
		if _, exists := s.states[s.stateOrder[0]]; exists {
			break
		}
		s.stateOrder = s.stateOrder[1:]
	}
	for _, key := range s.stateOrder {
		if event, exists := s.states[key]; exists && (!hasNext || event.Sequence < next.Sequence) {
			next = event
			hasNext = true
		}
	}
	return next, hasNext
}

func (s *eventSubscriber) pop(event Event) {
	if len(s.logs) != 0 && s.logs[0].Sequence == event.Sequence {
		s.logs = s.logs[1:]
		return
	}
	if len(s.notices) != 0 && s.notices[0].Sequence == event.Sequence {
		s.notices = s.notices[1:]
		return
	}
	for index, key := range s.stateOrder {
		if pending, exists := s.states[key]; exists && pending.Sequence == event.Sequence {
			delete(s.states, key)
			s.stateOrder = append(s.stateOrder[:index], s.stateOrder[index+1:]...)
			return
		}
	}
}
