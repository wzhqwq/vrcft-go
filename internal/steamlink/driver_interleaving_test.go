package steamlink

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

// Each C call marks a completed loop iteration and holds the next select until
// the test releases it. Tick timestamps and current time advance independently.
type controlledClock struct {
	*fakeClock
	ticks   chan time.Time
	entered chan struct{}
	proceed chan struct{}
}

func newControlledClock() *controlledClock {
	return &controlledClock{fakeClock: newFakeClock(time.Unix(100, 0)), ticks: make(chan time.Time, 1), entered: make(chan struct{}, 1), proceed: make(chan struct{})}
}

func (c *controlledClock) NewTicker(time.Duration) ticker { return c }
func (c *controlledClock) C() <-chan time.Time {
	c.entered <- struct{}{}
	<-c.proceed
	return c.ticks
}
func (c *controlledClock) Stop() {}
func (c *controlledClock) awaitLoop(t *testing.T) {
	t.Helper()
	select {
	case <-c.entered:
	case <-time.After(time.Second):
		t.Fatal("driver did not reach next select")
	}
}
func (c *controlledClock) step(t *testing.T) {
	t.Helper()
	select {
	case c.proceed <- struct{}{}:
	case <-time.After(time.Second):
		t.Fatal("driver did not resume select")
	}
	c.awaitLoop(t)
}

func runControlledDriver(t *testing.T, c *controlledClock, driver *Driver, host *fakeHost) chan error {
	t.Helper()
	driver.clock = c
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- driver.Run(ctx, host) }()
	t.Cleanup(func() {
		cancel()
		close(c.proceed)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("driver did not stop")
		}
	})
	c.awaitLoop(t)
	return done
}

func TestDriverDelayedTickUsesCurrentTime(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "first_observation"
		if existing {
			name = "existing_stream"
		}
		t.Run(name, func(t *testing.T) {
			c := newControlledClock()
			start := c.Now()
			driver := New()
			var r *receiver
			driver.listen = func(string) (*receiver, error) {
				var err error
				r, err = listenReceiver("127.0.0.1:0")
				return r, err
			}
			host := newFakeHost(pluginapi.Startup{Active: true, Subscription: expressionSubscription(trackingmodel.ExpressionJawOpen)})
			runControlledDriver(t, c, driver, host)
			if existing {
				sendJawDrop(t, r.localAddr(), .2)
				c.step(t)
				c.Advance(10 * time.Millisecond)
				c.ticks <- start.Add(10 * time.Millisecond)
				c.step(t)
			}
			// A tick scheduled at 20ms is serviced after an input received at
			// 25ms. The input wins the select before the delayed tick is read.
			c.Advance(start.Add(25 * time.Millisecond).Sub(c.Now()))
			sendJawDrop(t, r.localAddr(), .7)
			c.step(t)
			before := host.frameCount()
			c.Advance(2 * time.Millisecond)
			c.ticks <- start.Add(20 * time.Millisecond)
			c.step(t)
			frames := host.framesCopy()
			if len(frames) != before+1 {
				t.Fatalf("delayed tick lost fresh observation: frames=%d, want %d", len(frames), before+1)
			}
			frame := frames[len(frames)-1]
			if value, valid := frame.Expressions.Get(trackingmodel.ExpressionJawOpen); !valid || value != .7 {
				t.Fatalf("delayed tick invalidated fresh observation: value=%v valid=%v", value, valid)
			}
			if frame.TimestampNS != int64(27*time.Millisecond) {
				t.Fatalf("publication timestamp=%d, want current time 27ms", frame.TimestampNS)
			}
			// The next scheduled tick is only 3ms after this publication.
			// Keep the new observation dirty until a full 10ms has elapsed.
			c.Advance(time.Millisecond)
			sendJawDrop(t, r.localAddr(), .8)
			c.step(t)
			c.Advance(2 * time.Millisecond)
			c.ticks <- start.Add(30 * time.Millisecond)
			c.step(t)
			if host.frameCount() != len(frames) {
				t.Fatal("published twice within 10ms")
			}
			c.Advance(10 * time.Millisecond)
			c.ticks <- start.Add(40 * time.Millisecond)
			c.step(t)
			frames = host.framesCopy()
			if len(frames) != before+2 {
				t.Fatal("rate limiting discarded pending observation")
			}
			if value, valid := frames[len(frames)-1].Expressions.Get(trackingmodel.ExpressionJawOpen); !valid || value != .8 {
				t.Fatalf("pending observation=%v,%v", value, valid)
			}
		})
	}
}

func TestDriverRetryPreservesNewerConfigError(t *testing.T) {
	c := newControlledClock()
	driver := New()
	calls := 0
	driver.listen = func(string) (*receiver, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("occupied")
		}
		return listenReceiver("127.0.0.1:0")
	}
	host := newFakeHost(pluginapi.Startup{Config: pluginapi.Config{Revision: 1, Data: []byte(`{"listenPort":9016}`)}})
	runControlledDriver(t, c, driver, host)
	host.events <- pluginapi.ConfigChanged{Config: pluginapi.Config{Revision: 2, Data: []byte(`{`)}}
	c.step(t)
	statuses := host.statusesCopy()
	unapplied := statuses[len(statuses)-1]
	if unapplied.State != pluginapi.DeviceError {
		t.Fatalf("invalid revision status=%v", unapplied)
	}
	c.Advance(2 * time.Second)
	c.ticks <- c.Now()
	c.step(t)
	if calls != 2 {
		t.Fatalf("retry attempts=%d", calls)
	}
	statuses = host.statusesCopy()
	if got := statuses[len(statuses)-1]; got != unapplied {
		t.Fatalf("older retry cleared newer revision error: got=%v want=%v", got, unapplied)
	}
	host.events <- pluginapi.ConfigChanged{Config: pluginapi.Config{Revision: 3, Data: []byte(`{"listenPort":9016}`)}}
	c.step(t)
	statuses = host.statusesCopy()
	if got := statuses[len(statuses)-1]; got.State != pluginapi.DeviceDisconnected || got.Message != "" {
		t.Fatalf("valid correction did not clear error: %v", got)
	}
}

// A real socket supplies close/deadline/address behavior. Its read commits a
// substantive error under test control, independent of retirement closing it.
type failingPacketConn struct {
	*net.UDPConn
	fail   <-chan struct{}
	failed chan struct{}
	closed chan struct{}
	err    error
}

func (c *failingPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	select {
	case <-c.fail:
		close(c.failed)
		return 0, nil, c.err
	case <-c.closed:
		return 0, nil, net.ErrClosed
	}
}

func (c *failingPacketConn) Close() error {
	close(c.closed)
	return c.UDPConn.Close()
}

func TestDriverReturnsWorkerErrorDuringRebind(t *testing.T) {
	c := newControlledClock()
	driver := New()
	fail := make(chan struct{})
	failed := make(chan struct{})
	want := errors.New("substantive receiver failure")
	var candidate *receiver
	calls := 0
	driver.listen = func(string) (*receiver, error) {
		calls++
		r, err := listenReceiver("127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		if calls == 1 {
			r.conn = &failingPacketConn{UDPConn: r.conn.(*net.UDPConn), fail: fail, failed: failed, closed: make(chan struct{}), err: want}
		} else {
			candidate = r
			// Commit the old worker's error while ConfigChanged owns the
			// control loop, before it closes and joins that worker.
			close(fail)
			<-failed
		}
		return r, nil
	}
	host := newFakeHost(pluginapi.Startup{})
	done := runControlledDriver(t, c, driver, host)
	host.events <- pluginapi.ConfigChanged{Config: pluginapi.Config{Revision: 2, Data: []byte(`{"listenPort":9016}`)}}
	c.proceed <- struct{}{}
	select {
	case err := <-done:
		// Leave the result available for the common cleanup to join Run.
		done <- err
		if !errors.Is(err, want) {
			t.Fatalf("Run error=%v, want worker error", err)
		}
	case <-c.entered:
		t.Fatalf("worker failure was swallowed as a recoverable bind failure: %v", host.statusesCopy())
	case <-time.After(time.Second):
		t.Fatal("Run did not return worker failure")
	}
	if candidate == nil {
		t.Fatal("candidate was not created")
	}
	if err := candidate.conn.SetReadDeadline(time.Now()); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("unused candidate socket was not closed: %v", err)
	}
}
