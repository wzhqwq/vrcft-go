package picoconnect

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const (
	publishInterval   = 10 * time.Millisecond
	disconnectedAfter = 2 * time.Second
)

type Driver struct{}

func New() *Driver { return &Driver{} }

func (*Driver) Descriptor() pluginapi.Descriptor {
	return pluginapi.Descriptor{APIVersion: pluginapi.APIVersion, ID: "pico-connect", Name: "PICO Connect", Version: "0.1.0", Description: "Eye and expression tracking from PICO Connect.", Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression}
}

type receivedPacket struct {
	data  []byte
	at    time.Time
	epoch uint64
}

func (*Driver) Run(ctx context.Context, host pluginapi.Host) error {
	started := time.Now()
	state := newStreamState(started)
	startup := host.Startup()
	active, sub := startup.Active, startup.Subscription.Normalize()
	state.reset(active, sub)
	var epoch atomic.Uint64
	packets := make(chan receivedPacket, 64)
	var conn *net.UDPConn
	var workerDone chan error
	var cfg config
	var cfgValid bool
	var statusError string
	var lastInput time.Time
	var publishedStatus pluginapi.DeviceStatus
	var statusPublished bool
	closeReceiver := func() {
		if conn != nil {
			_ = conn.Close()
			conn = nil
		}
		if workerDone != nil {
			<-workerDone
			workerDone = nil
		}
	}
	defer closeReceiver()
	bind := func(candidate config) error {
		if candidate.TransferProtocol == 0 {
			closeReceiver()
			epoch.Add(1)
			state.reset(active, sub)
			cfg = candidate
			cfgValid = true
			return nil
		}
		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(candidate.ListenPort))
		addr, err := net.ResolveUDPAddr("udp4", address)
		if err != nil {
			return err
		}
		next, err := net.ListenUDP("udp4", addr)
		if err != nil {
			return err
		}
		closeReceiver()
		epoch.Add(1)
		state.reset(active, sub)
		cfg = candidate
		cfgValid = true
		conn = next
		workerDone = make(chan error, 1)
		go func(socket *net.UDPConn, done chan<- error) {
			buffer := make([]byte, 65535)
			for {
				n, _, err := socket.ReadFromUDP(buffer)
				if err != nil {
					done <- nil
					return
				}
				if n > 2048 {
					continue
				}
				packet := receivedPacket{data: append([]byte(nil), buffer[:n]...), at: time.Now(), epoch: epoch.Load()}
				select {
				case packets <- packet:
				default:
				}
			}
		}(next, workerDone)
		return nil
	}
	status := func(now time.Time) {
		next := pluginapi.DeviceStatus{State: pluginapi.DeviceDisconnected}
		if statusError != "" {
			next = pluginapi.DeviceStatus{State: pluginapi.DeviceError, Message: statusError}
		} else if cfgValid && cfg.TransferProtocol == 0 {
			next = pluginapi.DeviceStatus{State: pluginapi.DeviceError, Message: "PICO Connect transfer protocol 0 interface is not yet verified"}
		} else if !lastInput.IsZero() && now.Sub(lastInput) < disconnectedAfter {
			next.State = pluginapi.DeviceReady
		}
		if !statusPublished || next != publishedStatus {
			host.PublishStatus(next)
			publishedStatus = next
			statusPublished = true
		}
	}
	host.PublishStatus(pluginapi.DeviceStatus{State: pluginapi.DeviceInitializing})
	initial, err := parseConfig(startup.Config.Data)
	if err != nil {
		statusError = fmt.Sprintf("invalid configuration revision %d", startup.Config.Revision)
	} else if err := bind(initial); err != nil {
		statusError = "unable to listen on configured port"
	}
	status(time.Now())
	ticker := time.NewTicker(publishInterval)
	defer ticker.Stop()
	events := host.Events()
	handle := func(event pluginapi.ControlEvent) bool {
		switch e := event.(type) {
		case pluginapi.ShutdownRequested:
			return true
		case pluginapi.ActiveChanged:
			if active != e.Active {
				active = e.Active
				epoch.Add(1)
				state.reset(active, sub)
			}
		case pluginapi.SubscriptionChanged:
			next := e.Subscription.Normalize()
			if next != sub {
				sub = next
				epoch.Add(1)
				state.reset(active, sub)
			}
		case pluginapi.ConfigChanged:
			candidate, err := parseConfig(e.Config.Data)
			if err != nil {
				statusError = fmt.Sprintf("invalid configuration revision %d", e.Config.Revision)
				break
			}
			if cfgValid && candidate == cfg {
				statusError = ""
				break
			}
			if err := bind(candidate); err != nil {
				statusError = fmt.Sprintf("unable to apply configuration revision %d", e.Config.Revision)
			} else {
				statusError = ""
				lastInput = time.Time{}
			}
		}
		status(time.Now())
		return false
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case e, ok := <-events:
			if !ok {
				events = nil
			} else if handle(e) {
				return nil
			}
			continue
		default:
		}
		select {
		case <-ctx.Done():
			return nil
		case e, ok := <-events:
			if !ok {
				events = nil
			} else if handle(e) {
				return nil
			}
		case packet := <-packets:
			if packet.epoch != epoch.Load() || !cfgValid || cfg.TransferProtocol != 2 {
				continue
			}
			parsed, err := parseLegacyPacket(packet.data)
			if err != nil {
				continue
			}
			valid := false
			for _, present := range parsed.present {
				if present {
					valid = true
					break
				}
			}
			if valid {
				lastInput = packet.at
				state.observe(parsed, packet.at)
				status(time.Now())
			}
		case <-ticker.C:
			now := time.Now()
			if cfgValid && cfg.TransferProtocol == 2 && conn == nil {
				if err := bind(cfg); err == nil {
					statusError = ""
				}
			}
			if frame, ok := state.next(now); ok {
				host.PublishFrame(frame)
			}
			status(now)
		}
	}
}
