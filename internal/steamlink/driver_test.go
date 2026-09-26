package steamlink

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/osc"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func TestDriverDescriptorIsValid(t *testing.T) {
	if err := New().Descriptor().Validate(); err != nil {
		t.Fatalf("descriptor: %v", err)
	}
}

func TestDriverPublishesSubscribedUDPInput(t *testing.T) {
	clock := newFakeClock(time.Unix(100, 0))
	driver := New()
	driver.clock = clock
	created := make(chan *receiver, 1)
	driver.listen = func(address string) (*receiver, error) {
		r, err := listenReceiver("127.0.0.1:0")
		if err == nil {
			created <- r
		}
		return r, err
	}
	host := newFakeHost(pluginapi.Startup{Active: true, Subscription: expressionSubscription(trackingmodel.ExpressionJawOpen)})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- driver.Run(ctx, host) }()

	var receiver *receiver
	select {
	case receiver = <-created:
	case <-time.After(time.Second):
		t.Fatal("listener not created")
	}
	payload, err := osc.MarshalMessage(osc.Message{Address: "/sl/xrfb/facew/JawDrop", Args: []osc.Value{{Kind: osc.ValueFloat32, F32: .4}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	sendDatagram(t, receiver.localAddr(), payload)
	awaitFrameCount(t, host, 0)
	advanceUntilFrames(t, clock, host, 1)
	frame := host.framesCopy()[0]
	if frame.Expressions.Valid != trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen) {
		t.Fatalf("frame validity = %v", frame.Expressions.Valid)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not finish")
	}
}

func TestDriverClearsStateAcrossActivation(t *testing.T) {
	clock := newFakeClock(time.Unix(100, 0))
	driver, host, receiver, cancel, done := startDriver(t, clock, true, expressionSubscription(trackingmodel.ExpressionJawOpen))
	_ = driver
	sendJawDrop(t, receiver.localAddr(), .4)
	advanceUntilFrames(t, clock, host, 1)
	host.events <- pluginapi.ActiveChanged{Active: false}
	host.events <- pluginapi.ActiveChanged{Active: true}
	clock.Advance(10 * time.Millisecond)
	if host.frameCount() != 1 {
		t.Fatal("cached frame published after activation transition")
	}
	cancel()
	<-done
}

func TestDriverClosedEventsDoesNotBusyLoop(t *testing.T) {
	clock := newFakeClock(time.Unix(100, 0))
	_, host, _, cancel, done := startDriver(t, clock, true, expressionSubscription(trackingmodel.ExpressionJawOpen))
	close(host.events)
	clock.Advance(10 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestDriverRetriesInitialBindForValidStartupConfig(t *testing.T) {
	clock := newFakeClock(time.Unix(100, 0))
	driver := New()
	driver.clock = clock
	var calls atomic.Int32
	addresses := make(chan string, 2)
	created := make(chan *receiver, 1)
	driver.listen = func(address string) (*receiver, error) {
		addresses <- address
		if calls.Add(1) == 1 {
			return nil, errors.New("occupied")
		}
		r, err := listenReceiver("127.0.0.1:0")
		if err == nil {
			created <- r
		}
		return r, err
	}
	host := newFakeHost(pluginapi.Startup{Active: true, Config: pluginapi.Config{Revision: 1, Data: []byte(`{"listenPort":9016}`)}, Subscription: expressionSubscription(trackingmodel.ExpressionJawOpen)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- driver.Run(ctx, host) }()
	awaitStatus(t, host, pluginapi.DeviceError)
	var r *receiver
	advanceUntil(t, clock, func() bool {
		select {
		case r = <-created:
			return true
		default:
			return false
		}
	})
	if r == nil {
		t.Fatal("nil retried listener")
	}
	if calls.Load() != 2 || <-addresses != "127.0.0.1:9016" || <-addresses != "127.0.0.1:9016" {
		t.Fatalf("initial retry calls=%d", calls.Load())
	}
	awaitStatus(t, host, pluginapi.DeviceDisconnected)
	cancel()
	<-done
}

func TestDriverRecoversFromMalformedConfig(t *testing.T) {
	clock := newFakeClock(time.Unix(100, 0))
	driver := New()
	driver.clock = clock
	created := make(chan *receiver, 1)
	driver.listen = func(string) (*receiver, error) {
		r, err := listenReceiver("127.0.0.1:0")
		if err == nil {
			created <- r
		}
		return r, err
	}
	host := newFakeHost(pluginapi.Startup{Active: true, Config: pluginapi.Config{Revision: 1, Data: []byte(`{`)}})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- driver.Run(ctx, host) }()
	awaitStatus(t, host, pluginapi.DeviceError)
	host.events <- pluginapi.ConfigChanged{Config: pluginapi.Config{Revision: 2, Data: []byte(`{}`)}}
	var r *receiver
	advanceUntil(t, clock, func() bool {
		select {
		case r = <-created:
			return true
		default:
			return false
		}
	})
	if r == nil {
		t.Fatal("valid correction did not bind")
	}
	awaitStatus(t, host, pluginapi.DeviceDisconnected)
	cancel()
	<-done
}

func TestDriverFailedRebindPreservesOldReceiverAndSamePortCorrection(t *testing.T) {
	clock := newFakeClock(time.Unix(100, 0))
	driver := New()
	driver.clock = clock
	var calls atomic.Int32
	created := make(chan *receiver, 1)
	driver.listen = func(string) (*receiver, error) {
		if calls.Add(1) == 2 {
			return nil, errors.New("occupied")
		}
		r, err := listenReceiver("127.0.0.1:0")
		if err == nil {
			created <- r
		}
		return r, err
	}
	host := newFakeHost(pluginapi.Startup{Active: true, Subscription: expressionSubscription(trackingmodel.ExpressionJawOpen)})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- driver.Run(ctx, host) }()
	var old *receiver
	select {
	case old = <-created:
	case <-time.After(time.Second):
		t.Fatal("initial listener")
	}
	sendJawDrop(t, old.localAddr(), .4)
	advanceUntilFrames(t, clock, host, 1)
	host.events <- pluginapi.ConfigChanged{Config: pluginapi.Config{Revision: 2, Data: []byte(`{"listenPort":9016}`)}}
	advanceUntil(t, clock, func() bool { return calls.Load() == 2 })
	awaitStatus(t, host, pluginapi.DeviceError)
	sendJawDrop(t, old.localAddr(), .5)
	advanceUntilFrames(t, clock, host, 2)
	host.events <- pluginapi.ConfigChanged{Config: pluginapi.Config{Revision: 3, Data: []byte(`{}`)}}
	advanceUntil(t, clock, func() bool { return calls.Load() == 2 })
	awaitStatus(t, host, pluginapi.DeviceReady)
	cancel()
	<-done
}

func TestDriverSuccessfulRebindDiscardsRetiredReceiverPacket(t *testing.T) {
	clock := newFakeClock(time.Unix(100, 0))
	driver := New()
	driver.clock = clock
	created := make(chan *receiver, 2)
	addresses := make(chan string, 2)
	candidateStarted := make(chan struct{})
	allowCandidate := make(chan struct{})
	var calls atomic.Int32
	driver.listen = func(address string) (*receiver, error) {
		addresses <- address
		if calls.Add(1) == 2 {
			close(candidateStarted)
			<-allowCandidate
		}
		r, err := listenReceiver("127.0.0.1:0")
		if err == nil {
			created <- r
		}
		return r, err
	}
	host := newFakeHost(pluginapi.Startup{Active: true, Subscription: expressionSubscription(trackingmodel.ExpressionJawOpen)})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- driver.Run(ctx, host) }()
	var old *receiver
	select {
	case old = <-created:
	case <-time.After(time.Second):
		t.Fatal("initial listener")
	}
	host.events <- pluginapi.ConfigChanged{Config: pluginapi.Config{Revision: 2, Data: []byte(`{"listenPort":9016}`)}}
	select {
	case <-candidateStarted:
	case <-time.After(time.Second):
		t.Fatal("candidate bind did not start")
	}
	// The candidate bind holds the control loop before it can close the old
	// receiver. Fill the packet queue from that receiver so its epoch is
	// definitely retired when the rebind completes.
	for range 100 {
		sendJawDrop(t, old.localAddr(), .2)
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	if dropped := awaitDropped(t, waitCtx, old); dropped == 0 {
		t.Fatal("old receiver did not queue packets")
	}
	waitCancel()
	close(allowCandidate)
	var replacement *receiver
	select {
	case replacement = <-created:
	case <-time.After(time.Second):
		t.Fatal("replacement listener")
	}
	if <-addresses != "127.0.0.1:9015" || <-addresses != "127.0.0.1:9016" {
		t.Fatal("rebind did not use the configured ports")
	}
	clock.Advance(10 * time.Millisecond)
	time.Sleep(time.Millisecond)
	if host.frameCount() != 0 {
		t.Fatal("retired receiver packet published after rebind")
	}
	sendJawDrop(t, replacement.localAddr(), .7)
	advanceUntilFrames(t, clock, host, 1)
	frame := host.framesCopy()[0]
	if value, ok := frame.Expressions.Get(trackingmodel.ExpressionJawOpen); !ok || value != .7 {
		t.Fatalf("replacement frame=%v,%v", value, ok)
	}
	cancel()
	<-done
}

func TestDriverShutdownStopsWorkerAndTicker(t *testing.T) {
	clock := newFakeClock(time.Unix(100, 0))
	_, host, _, _, done := startDriver(t, clock, true, expressionSubscription(trackingmodel.ExpressionJawOpen))
	host.events <- pluginapi.ShutdownRequested{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish")
	}
	clock.mu.Lock()
	tickers := append([]*fakeTicker(nil), clock.tickers...)
	clock.mu.Unlock()
	if len(tickers) != 1 || !tickers[0].stoppedState() {
		t.Fatal("publication ticker was not stopped")
	}
}

func TestDriverHealthReceivesInputWithoutSubscription(t *testing.T) {
	clock := newFakeClock(time.Unix(100, 0))
	_, host, r, cancel, done := startDriver(t, clock, true, pluginapi.Subscription{})
	sendJawDrop(t, r.localAddr(), .4)
	advanceUntil(t, clock, func() bool {
		statuses := host.statusesCopy()
		return len(statuses) > 0 && statuses[len(statuses)-1].State == pluginapi.DeviceReady
	})
	if host.frameCount() != 0 {
		t.Fatal("unsubscribed input published a frame")
	}
	advanceUntil(t, clock, func() bool {
		statuses := host.statusesCopy()
		return len(statuses) > 0 && statuses[len(statuses)-1].State == pluginapi.DeviceDisconnected
	})
	cancel()
	<-done
}

func TestDriverDoesNotRetryRejectedFrame(t *testing.T) {
	clock := newFakeClock(time.Unix(100, 0))
	_, host, r, cancel, done := startDriver(t, clock, true, expressionSubscription(trackingmodel.ExpressionJawOpen))
	host.setAccept(false)
	sendJawDrop(t, r.localAddr(), .4)
	advanceUntilFrames(t, clock, host, 1)
	clock.Advance(100 * time.Millisecond)
	time.Sleep(time.Millisecond)
	if host.frameCount() != 1 {
		t.Fatalf("frames = %d, want 1", host.frameCount())
	}
	cancel()
	<-done
}

func TestDriverCancellationDuringSustainedUDP(t *testing.T) {
	clock := newFakeClock(time.Unix(100, 0))
	_, _, r, cancel, done := startDriver(t, clock, true, expressionSubscription(trackingmodel.ExpressionJawOpen))
	payload, err := osc.MarshalMessage(osc.Message{Address: "/sl/xrfb/facew/JawDrop", Args: []osc.Value{{Kind: osc.ValueFloat32, F32: .4}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	conn, err := net.DialUDP("udp4", nil, r.localAddr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	stopSending := make(chan struct{})
	sent := make(chan struct{}, 1)
	senderDone := make(chan struct{})
	go func() {
		defer close(senderDone)
		for {
			select {
			case <-stopSending:
				return
			default:
			}
			_, _ = conn.Write(payload)
			select {
			case sent <- struct{}{}:
			default:
			}
		}
	}()
	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("sender did not send")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop during UDP traffic")
	}
	close(stopSending)
	select {
	case <-senderDone:
	case <-time.After(time.Second):
		t.Fatal("sender did not stop")
	}
}

func TestDriverDiagnosticsDoNotLogRawInputOrConfig(t *testing.T) {
	clock := newFakeClock(time.Unix(100, 0))
	_, host, r, cancel, done := startDriver(t, clock, true, expressionSubscription(trackingmodel.ExpressionJawOpen))
	sendJawDrop(t, r.localAddr(), 123.456)
	host.events <- pluginapi.ConfigChanged{Config: pluginapi.Config{Revision: 2, Data: []byte(`{"listenPort":9015,"secret":"configuration-should-not-log"}`)}}
	advanceUntil(t, clock, func() bool {
		statuses := host.statusesCopy()
		return len(statuses) > 0 && statuses[len(statuses)-1].State == pluginapi.DeviceError
	})
	advanceUntil(t, clock, func() bool { return len(host.logsCopy()) > 0 })
	for _, message := range host.logsCopy() {
		if strings.Contains(message, "123.456") || strings.Contains(message, "configuration-should-not-log") || strings.Contains(message, "listenPort") {
			t.Fatalf("diagnostic exposed raw data: %q", message)
		}
	}
	cancel()
	<-done
}

func TestDriverReturnsUnrecoverableWorkerError(t *testing.T) {
	driver := New()
	driver.listen = func(string) (*receiver, error) {
		r, err := listenReceiver("127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		if err := r.conn.SetReadDeadline(time.Now()); err != nil {
			return nil, err
		}
		return r, nil
	}
	done := make(chan error, 1)
	go func() { done <- driver.Run(context.Background(), newFakeHost(pluginapi.Startup{})) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run accepted unrecoverable read error")
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return worker error")
	}
}

func startDriver(t *testing.T, clock *fakeClock, active bool, sub pluginapi.Subscription) (*Driver, *fakeHost, *receiver, context.CancelFunc, <-chan error) {
	t.Helper()
	driver := New()
	driver.clock = clock
	created := make(chan *receiver, 1)
	driver.listen = func(string) (*receiver, error) {
		r, err := listenReceiver("127.0.0.1:0")
		if err == nil {
			created <- r
		}
		return r, err
	}
	host := newFakeHost(pluginapi.Startup{Active: active, Subscription: sub})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- driver.Run(ctx, host) }()
	select {
	case r := <-created:
		return driver, host, r, cancel, done
	case <-time.After(time.Second):
		cancel()
		t.Fatal("listener not created")
		return nil, nil, nil, nil, nil
	}
}

func sendJawDrop(t *testing.T, address *net.UDPAddr, value float32) {
	t.Helper()
	payload, err := osc.MarshalMessage(osc.Message{Address: "/sl/xrfb/facew/JawDrop", Args: []osc.Value{{Kind: osc.ValueFloat32, F32: value}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	sendDatagram(t, address, payload)
}

func awaitFrameCount(t *testing.T, host *fakeHost, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if host.frameCount() >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("frames = %d, want at least %d", host.frameCount(), count)
}

func advanceUntilFrames(t *testing.T, clock *fakeClock, host *fakeHost, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		clock.Advance(10 * time.Millisecond)
		if host.frameCount() >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("frames = %d, want at least %d", host.frameCount(), count)
}

func advanceUntil(t *testing.T, clock *fakeClock, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		clock.Advance(10 * time.Millisecond)
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition did not become true")
}

func awaitStatus(t *testing.T, host *fakeHost, state pluginapi.DeviceState) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		statuses := host.statusesCopy()
		if len(statuses) > 0 && statuses[len(statuses)-1].State == state {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("last status = %#v, want %s", host.statusesCopy(), state)
}
