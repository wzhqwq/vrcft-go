package picoconnect

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const (
	publishInterval          = 10 * time.Millisecond
	disconnectedAfter        = 2 * time.Second
	bindRetryInterval        = 2 * time.Second
	prerequisitePollInterval = 2 * time.Second
)

type Driver struct {
	listen             func(*net.UDPAddr) (*net.UDPConn, error)
	checkPrerequisites func() string
	prerequisitePoll   time.Duration
}

func New() *Driver {
	return &Driver{listen: func(addr *net.UDPAddr) (*net.UDPConn, error) { return net.ListenUDP("udp4", addr) }, checkPrerequisites: checkPrerequisites, prerequisitePoll: prerequisitePollInterval}
}

func (*Driver) Descriptor() pluginapi.Descriptor {
	return pluginapi.Descriptor{APIVersion: pluginapi.APIVersion, ID: "pico-connect", Name: "PICO Connect", Version: "0.1.0", Description: "Eye and expression tracking from PICO Connect.", Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression}
}

type receivedPacket struct {
	data  []byte
	at    time.Time
	epoch uint64
}

func acceptsPacket(packet receivedPacket, epoch uint64, fence time.Time) bool {
	return packet.epoch == epoch && !packet.at.Before(fence)
}

func (d *Driver) Run(ctx context.Context, host pluginapi.Host) error {
	started := time.Now()
	state := newStreamState(started)
	startup := host.Startup()
	active, sub := startup.Active, startup.Subscription.Normalize()
	state.reset(active, sub)
	var epoch atomic.Uint64
	var fence time.Time
	packets := make(chan receivedPacket, 64)
	var oversized atomic.Uint64
	var queueDropped atomic.Uint64
	var invalidPackets uint64
	var pendingOversized uint64
	var pendingQueueDropped uint64
	var lastDiagnostic time.Time
	var conn *net.UDPConn
	var workerDone chan error
	var cfg config
	var cfgValid bool
	var pending *config
	var retryAt time.Time
	var statusError string
	var prerequisiteWarning string
	var nextPrerequisiteCheck time.Time
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
	transition := func() { epoch.Add(1); fence = time.Now(); state.reset(active, sub) }
	bind := func(candidate config) error {
		address := net.JoinHostPort("127.0.0.1", strconv.Itoa(candidate.ListenPort))
		addr, err := net.ResolveUDPAddr("udp4", address)
		if err != nil {
			return err
		}
		next, err := d.listen(addr)
		if err != nil {
			return err
		}
		closeReceiver()
		transition()
		cfg = candidate
		cfgValid = true
		conn = next
		workerDone = make(chan error, 1)
		go func(socket *net.UDPConn, done chan<- error) {
			buffer := make([]byte, 65535)
			for {
				readEpoch := epoch.Load()
				n, _, err := socket.ReadFromUDP(buffer)
				if err != nil {
					done <- err
					return
				}
				receivedAt := time.Now()
				if n > 2048 {
					oversized.Add(1)
					continue
				}
				packet := receivedPacket{data: append([]byte(nil), buffer[:n]...), at: receivedAt, epoch: readEpoch}
				select {
				case packets <- packet:
				default:
					queueDropped.Add(1)
				}
			}
		}(next, workerDone)
		return nil
	}
	status := func(now time.Time) {
		next := pluginapi.DeviceStatus{State: pluginapi.DeviceDisconnected}
		if prerequisiteWarning != "" {
			next = pluginapi.DeviceStatus{State: pluginapi.DeviceError, Message: prerequisiteWarning}
		} else if statusError != "" {
			next = pluginapi.DeviceStatus{State: pluginapi.DeviceError, Message: statusError}
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
	checkPrerequisite := func(now time.Time) {
		if d.checkPrerequisites == nil {
			return
		}
		warning := d.checkPrerequisites()
		interval := d.prerequisitePoll
		if interval <= 0 {
			interval = prerequisitePollInterval
		}
		nextPrerequisiteCheck = now.Add(interval)
		if warning == prerequisiteWarning {
			return
		}
		prerequisiteWarning = warning
		if warning != "" {
			host.Log(pluginapi.LogWarn, warning)
			closeReceiver()
			transition()
			lastInput = time.Time{}
			pending = nil
		} else if cfgValid {
			pending, retryAt = &cfg, now
		}
	}
	initial, err := parseConfig(startup.Config.Data)
	if err != nil {
		statusError = fmt.Sprintf("invalid configuration revision %d", startup.Config.Revision)
	} else {
		cfg, cfgValid = initial, true
		checkPrerequisite(time.Now())
		if prerequisiteWarning == "" {
			if err := bind(initial); err != nil {
				statusError = "unable to listen on configured port"
				pending, retryAt = &initial, time.Now().Add(bindRetryInterval)
			}
		}
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
				transition()
			}
		case pluginapi.SubscriptionChanged:
			next := e.Subscription.Normalize()
			if next != sub {
				sub = next
				transition()
			}
		case pluginapi.ConfigChanged:
			candidate, err := parseConfig(e.Config.Data)
			if err != nil {
				statusError = fmt.Sprintf("invalid configuration revision %d", e.Config.Revision)
				break
			}
			if cfgValid && candidate == cfg {
				statusError = ""
				if prerequisiteWarning == "" && conn != nil {
					pending = nil
				}
				break
			}
			if prerequisiteWarning != "" {
				cfg, cfgValid = candidate, true
				statusError = ""
				pending = nil
				break
			}
			if err := bind(candidate); err != nil {
				statusError = fmt.Sprintf("unable to apply configuration revision %d", e.Config.Revision)
				if conn == nil {
					pending, retryAt = &candidate, time.Now().Add(bindRetryInterval)
				}
			} else {
				statusError = ""
				pending = nil
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
			if !acceptsPacket(packet, epoch.Load(), fence) || !cfgValid || prerequisiteWarning != "" {
				continue
			}
			parsed, err := parseLegacyPacket(packet.data)
			if err != nil {
				invalidPackets++
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
			if !now.Before(nextPrerequisiteCheck) {
				checkPrerequisite(now)
			}
			pendingOversized += oversized.Swap(0)
			pendingQueueDropped += queueDropped.Swap(0)
			if cfgValid && (invalidPackets > 0 || pendingOversized > 0 || pendingQueueDropped > 0) && (lastDiagnostic.IsZero() || now.Sub(lastDiagnostic) >= 5*time.Second) {
				host.Log(pluginapi.LogWarn, fmt.Sprintf("PICO Connect protocol=%d port=%d discarded malformed=%d oversized=%d queue-full=%d packets", cfg.TransferProtocol, cfg.ListenPort, invalidPackets, pendingOversized, pendingQueueDropped))
				invalidPackets, pendingOversized, pendingQueueDropped, lastDiagnostic = 0, 0, 0, now
			}
			if prerequisiteWarning == "" && pending != nil && !now.Before(retryAt) {
				if err := bind(*pending); err == nil {
					statusError = ""
					pending = nil
					lastInput = time.Time{}
				} else {
					statusError = "unable to listen on configured port"
					retryAt = now.Add(bindRetryInterval)
				}
			}
			if frame, ok := state.next(now); ok {
				host.PublishFrame(frame)
			}
			status(now)
		case err := <-workerDone:
			workerDone = nil
			if conn != nil {
				_ = conn.Close()
				conn = nil
			}
			if err != nil && !errors.Is(err, net.ErrClosed) {
				return fmt.Errorf("PICO Connect receiver stopped: %w", err)
			}
			return fmt.Errorf("PICO Connect receiver stopped unexpectedly")
		}
	}
}
