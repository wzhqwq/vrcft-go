package steamlink

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const (
	publishInterval      = 10 * time.Millisecond
	disconnectedAfter    = 2 * time.Second
	initialRetryInterval = 2 * time.Second
)

type ticker interface {
	C() <-chan time.Time
	Stop()
}

type clock interface {
	Now() time.Time
	NewTicker(time.Duration) ticker
}

type systemClock struct{}

func (systemClock) Now() time.Time                   { return time.Now() }
func (systemClock) NewTicker(d time.Duration) ticker { return systemTicker{Ticker: time.NewTicker(d)} }

type systemTicker struct{ *time.Ticker }

func (t systemTicker) C() <-chan time.Time { return t.Ticker.C }

// Driver adapts Steam Link's loopback OSC telemetry to the public plugin API.
// Mutable connection state belongs to Run, so a Driver can be reused safely.
type Driver struct {
	clock  clock
	listen func(string) (*receiver, error)
}

func New() *Driver { return &Driver{clock: systemClock{}, listen: listenReceiver} }

func (d *Driver) Descriptor() pluginapi.Descriptor {
	return pluginapi.Descriptor{
		APIVersion: pluginapi.APIVersion,
		ID:         "steamlink", Name: "Steam Link", Version: "0.1.0",
		Description:  "Receives Steam Link eye and expression tracking over loopback OSC.",
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
	}
}

func (d *Driver) Run(ctx context.Context, host pluginapi.Host) error {
	if d.clock == nil || d.listen == nil {
		return fmt.Errorf("steamlink driver is not initialized")
	}
	started := d.clock.Now()
	state := newStreamState(started)
	startup := host.Startup()
	active, sub := startup.Active, startup.Subscription.Normalize()
	state.reset(active, sub)

	packets := make(chan datagram, 64)
	var epoch atomic.Uint64
	var fence time.Time
	var current *receiver
	var workerDone <-chan error
	var currentConfig config
	haveConfig := false
	var retryConfig *config
	var retryAt time.Time
	var statusError string
	var lastInput time.Time
	hasInput := false
	diagnostics := newDiagnostics()
	tick := d.clock.NewTicker(publishInterval)
	defer tick.Stop()
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	defer func() {
		if current != nil {
			_ = current.close()
		}
		if workerDone != nil {
			<-workerDone
		}
	}()

	startWorker := func(r *receiver) {
		done := make(chan error, 1)
		go func() { done <- r.run(runCtx, &epoch, d.clock.Now, packets) }()
		workerDone = done
	}
	stopWorker := func() error {
		if current == nil {
			return nil
		}
		_ = current.close()
		err := <-workerDone
		current, workerDone = nil, nil
		return err
	}
	transition := func() {
		epoch.Add(1)
		fence = d.clock.Now()
		state.reset(active, sub)
	}
	bind := func(candidate config) error {
		r, err := d.listen("127.0.0.1:" + strconv.Itoa(candidate.ListenPort))
		if err != nil {
			return err
		}
		if err := stopWorker(); err != nil {
			_ = r.close()
			return err
		}
		currentConfig, haveConfig = candidate, true
		transition()
		current = r
		startWorker(r)
		return nil
	}
	setStatusError := func(message string) { statusError = message }
	status := pluginapi.DeviceStatus{}
	hasStatus := false
	publishStatus := func(now time.Time) {
		next := pluginapi.DeviceStatus{State: pluginapi.DeviceDisconnected}
		if statusError != "" {
			next = pluginapi.DeviceStatus{State: pluginapi.DeviceError, Message: statusError}
		} else if hasInput && now.Sub(lastInput) < disconnectedAfter {
			next.State = pluginapi.DeviceReady
		}
		if !hasStatus || next != status {
			host.PublishStatus(next)
			status, hasStatus = next, true
		}
	}

	host.PublishStatus(pluginapi.DeviceStatus{State: pluginapi.DeviceInitializing})
	initial, initialErr := parseConfig(startup.Config.Data)
	if initialErr != nil {
		setStatusError(fmt.Sprintf("configuration revision %d is invalid", startup.Config.Revision))
	} else if err := bind(initial); err != nil {
		setStatusError("unable to listen on configured port")
		retryCopy := initial
		retryConfig, retryAt = &retryCopy, d.clock.Now().Add(initialRetryInterval)
	}
	publishStatus(d.clock.Now())

	events := host.Events()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			shutdown, err := d.handleEvent(event, &active, &sub, state, &epoch, &fence, &currentConfig, &haveConfig, &retryConfig, &retryAt, &statusError, bind, d.clock.Now)
			if err != nil {
				return err
			}
			if shutdown {
				return nil
			}
			publishStatus(d.clock.Now())
			continue
		default:
		}

		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			shutdown, err := d.handleEvent(event, &active, &sub, state, &epoch, &fence, &currentConfig, &haveConfig, &retryConfig, &retryAt, &statusError, bind, d.clock.Now)
			if err != nil {
				return err
			}
			if shutdown {
				return nil
			}
			publishStatus(d.clock.Now())
		case packet := <-packets:
			if packet.Epoch != epoch.Load() || packet.ReceivedAt.Before(fence) {
				continue
			}
			observations, report, err := decodeDatagram(packet.Bytes)
			if err != nil {
				diagnostics.rejected(err)
				continue
			}
			diagnostics.report(report)
			if len(observations) > 0 {
				hasInput, lastInput = true, packet.ReceivedAt
			}
			state.observe(observations, packet.ReceivedAt)
			publishStatus(d.clock.Now())
		case now := <-tick.C():
			if current != nil {
				diagnostics.queueDrops += current.takeDropped()
			}
			if retryConfig != nil && !now.Before(retryAt) {
				candidate := *retryConfig
				if err := bind(candidate); err == nil {
					retryConfig, statusError = nil, ""
				} else {
					retryAt = now.Add(initialRetryInterval)
				}
			}
			if frame, publish := state.next(now); publish {
				host.PublishFrame(frame)
			}
			diagnostics.emit(host, now)
			publishStatus(now)
		case err := <-workerDone:
			if current != nil {
				_ = current.close()
			}
			current, workerDone = nil, nil
			if err != nil {
				return err
			}
			return nil
		}
	}
}

type bindFunc func(config) error

func (d *Driver) handleEvent(event pluginapi.ControlEvent, active *bool, sub *pluginapi.Subscription, state *streamState, epoch *atomic.Uint64, fence *time.Time, current *config, haveCurrent *bool, retry **config, retryAt *time.Time, statusError *string, bind bindFunc, now func() time.Time) (bool, error) {
	transition := func() { epoch.Add(1); *fence = now(); state.reset(*active, *sub) }
	switch event := event.(type) {
	case pluginapi.ShutdownRequested:
		return true, nil
	case pluginapi.ActiveChanged:
		if *active != event.Active {
			*active = event.Active
			transition()
		}
	case pluginapi.SubscriptionChanged:
		normalized := event.Subscription.Normalize()
		if normalized != *sub {
			*sub = normalized
			transition()
		}
	case pluginapi.ConfigChanged:
		candidate, err := parseConfig(event.Config.Data)
		if err != nil {
			*statusError = fmt.Sprintf("configuration revision %d is unapplied", event.Config.Revision)
			return false, nil
		}
		if *haveCurrent && candidate.ListenPort == current.ListenPort {
			*current, *retry, *statusError = candidate, nil, ""
			return false, nil
		}
		if err := bind(candidate); err != nil {
			*statusError = fmt.Sprintf("listen port change for revision %d is unapplied", event.Config.Revision)
			if !*haveCurrent {
				copy := candidate
				*retry, *retryAt = &copy, now().Add(initialRetryInterval)
			}
			return false, nil
		}
		*retry, *statusError = nil, ""
	}
	return false, nil
}
