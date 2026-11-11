package picoconnect

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type driverHost struct {
	startup  pluginapi.Startup
	events   chan pluginapi.ControlEvent
	mu       sync.Mutex
	statuses []pluginapi.DeviceStatus
	frames   []trackingmodel.TrackingFrame
	logs     []string
}

func (h *driverHost) Startup() pluginapi.Startup            { return h.startup }
func (h *driverHost) Events() <-chan pluginapi.ControlEvent { return h.events }
func (h *driverHost) PublishFrame(f trackingmodel.TrackingFrame) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frames = append(h.frames, f)
	return true
}
func (h *driverHost) PublishStatus(s pluginapi.DeviceStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.statuses = append(h.statuses, s)
}
func (h *driverHost) Log(_ pluginapi.LogLevel, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.logs = append(h.logs, message)
}
func (h *driverHost) snapshot() ([]pluginapi.DeviceStatus, []trackingmodel.TrackingFrame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]pluginapi.DeviceStatus(nil), h.statuses...), append([]trackingmodel.TrackingFrame(nil), h.frames...)
}
func (h *driverHost) logsCopy() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.logs...)
}

func TestDriverProtocolZeroReportsUnsupported(t *testing.T) {
	h := &driverHost{events: make(chan pluginapi.ControlEvent, 1)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- New().Run(ctx, h) }()
	deadline := time.After(500 * time.Millisecond)
	for {
		statuses, _ := h.snapshot()
		for _, s := range statuses {
			if s.State == pluginapi.DeviceError && s.Message != "" {
				cancel()
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				return
			}
		}
		select {
		case <-deadline:
			t.Fatalf("missing unsupported status: %+v", statuses)
		case <-time.After(time.Millisecond):
		}
	}
}

func TestDriverProtocolTwoPublishesLegacyPacket(t *testing.T) {
	probe, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()
	configData, _ := json.Marshal(map[string]int{"transferProtocol": 2, "listenPort": port})
	h := &driverHost{startup: pluginapi.Startup{Active: true, Config: pluginapi.Config{Revision: 1, Data: configData}, Subscription: pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityExpression}}, events: make(chan pluginapi.ControlEvent, 1)}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- New().Run(ctx, h) }()
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	packet := legacyFixture(912)
	binary.LittleEndian.PutUint32(packet[24+shapeJawOpen*4:], 0x3f400000)
	deadline := time.After(time.Second)
	for {
		_, _ = conn.Write(packet)
		_, frames := h.snapshot()
		for _, frame := range frames {
			if value, ok := frame.Expressions.Get(trackingmodel.ExpressionJawOpen); ok && value == .75 {
				cancel()
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				return
			}
		}
		select {
		case <-deadline:
			t.Fatalf("no jaw frame: %+v", frames)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestDriverRejectsOversizedDatagram(t *testing.T) {
	probe, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()
	configData, _ := json.Marshal(map[string]int{"transferProtocol": 2, "listenPort": port})
	h := &driverHost{startup: pluginapi.Startup{Active: true, Config: pluginapi.Config{Revision: 1, Data: configData}, Subscription: pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityExpression}}, events: make(chan pluginapi.ControlEvent, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- New().Run(ctx, h) }()
	defer func() { cancel(); <-done }()
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	packet := legacyFixture(2049)
	binary.LittleEndian.PutUint32(packet[24+shapeJawOpen*4:], 0x3f400000)
	for i := 0; i < 20; i++ {
		_, _ = conn.Write(packet)
		time.Sleep(10 * time.Millisecond)
	}
	_, frames := h.snapshot()
	if len(frames) != 0 {
		t.Fatalf("oversized datagram produced %d frames", len(frames))
	}
	valid := legacyFixture(912)
	binary.LittleEndian.PutUint32(valid[24+shapeJawOpen*4:], 0x3f400000)
	deadline := time.After(500 * time.Millisecond)
	for {
		_, _ = conn.Write(valid)
		_, frames = h.snapshot()
		if len(frames) > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("receiver did not recover after oversized datagram")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestDriverReceivesAfterSubscriptionChange(t *testing.T) {
	probe, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()
	configData, _ := json.Marshal(map[string]int{"transferProtocol": 2, "listenPort": port})
	h := &driverHost{startup: pluginapi.Startup{Active: true, Config: pluginapi.Config{Revision: 1, Data: configData}, Subscription: pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityExpression, Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionJawOpen)}}, events: make(chan pluginapi.ControlEvent, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- New().Run(ctx, h) }()
	defer func() { cancel(); <-done }()
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	packet := legacyFixture(912)
	binary.LittleEndian.PutUint32(packet[24+shapeJawOpen*4:], 0x3f400000)
	deadline := time.After(time.Second)
	for {
		_, _ = conn.Write(packet)
		_, frames := h.snapshot()
		if len(frames) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no first frame")
		case <-time.After(10 * time.Millisecond):
		}
	}
	h.events <- pluginapi.SubscriptionChanged{Subscription: pluginapi.Subscription{Generation: 2, Capabilities: trackingmodel.CapabilityExpression, Expressions: trackingmodel.ExpressionMaskOf(trackingmodel.ExpressionTongueOut)}}
	binary.LittleEndian.PutUint32(packet[24+shapeTongueOut*4:], 0x3f400000)
	deadline = time.After(time.Second)
	for {
		_, _ = conn.Write(packet)
		_, frames := h.snapshot()
		for _, frame := range frames[1:] {
			if frame.Expressions.Valid.Has(trackingmodel.ExpressionTongueOut) && !frame.Expressions.Valid.Has(trackingmodel.ExpressionJawOpen) {
				return
			}
		}
		select {
		case <-deadline:
			t.Fatal("no new-generation tongue frame")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestPacketFenceRejectsPriorRead(t *testing.T) {
	fence := time.Now()
	if acceptsPacket(receivedPacket{epoch: 2, at: fence.Add(-time.Nanosecond)}, 2, fence) {
		t.Fatal("old receive crossed control transition")
	}
	if acceptsPacket(receivedPacket{epoch: 1, at: fence.Add(time.Nanosecond)}, 2, fence) {
		t.Fatal("old epoch accepted")
	}
	if !acceptsPacket(receivedPacket{epoch: 2, at: fence.Add(time.Nanosecond)}, 2, fence) {
		t.Fatal("fresh receive rejected")
	}
}

func TestDriverRetriesStartupPortConflict(t *testing.T) {
	occupied, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := occupied.LocalAddr().(*net.UDPAddr).Port
	configData, _ := json.Marshal(map[string]int{"transferProtocol": 2, "listenPort": port})
	h := &driverHost{startup: pluginapi.Startup{Active: true, Config: pluginapi.Config{Revision: 1, Data: configData}, Subscription: pluginapi.Subscription{Generation: 1, Capabilities: trackingmodel.CapabilityExpression}}, events: make(chan pluginapi.ControlEvent, 1)}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	done := make(chan error, 1)
	go func() { done <- New().Run(ctx, h) }()
	defer func() { cancel(); <-done }()
	deadline := time.After(time.Second)
	for {
		statuses, _ := h.snapshot()
		found := false
		for _, s := range statuses {
			if s.State == pluginapi.DeviceError {
				found = true
			}
		}
		if found {
			break
		}
		select {
		case <-deadline:
			t.Fatal("missing bind error")
		case <-time.After(10 * time.Millisecond):
		}
	}
	occupied.Close()
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	packet := legacyFixture(912)
	binary.LittleEndian.PutUint32(packet[24+shapeJawOpen*4:], 0x3f400000)
	deadline = time.After(3500 * time.Millisecond)
	for {
		_, _ = conn.Write(packet)
		_, frames := h.snapshot()
		if len(frames) > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("bind never recovered")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestDriverReturnsUnexpectedReceiverFailure(t *testing.T) {
	configData, _ := json.Marshal(map[string]int{"transferProtocol": 2, "listenPort": 29765})
	h := &driverHost{startup: pluginapi.Startup{Config: pluginapi.Config{Revision: 1, Data: configData}}, events: make(chan pluginapi.ControlEvent, 1)}
	sockets := make(chan *net.UDPConn, 1)
	driver := &Driver{listen: func(addr *net.UDPAddr) (*net.UDPConn, error) {
		socket, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err == nil {
			sockets <- socket
		}
		return socket, err
	}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- driver.Run(ctx, h) }()
	var socket *net.UDPConn
	select {
	case socket = <-sockets:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("receiver never started")
	}
	_ = socket.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("receiver error suppressed")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("driver did not stop after receiver error")
	}
}

func TestDriverLogsBoundedMalformedPacketSummary(t *testing.T) {
	probe, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()
	configData, _ := json.Marshal(map[string]int{"transferProtocol": 2, "listenPort": port})
	h := &driverHost{startup: pluginapi.Startup{Config: pluginapi.Config{Revision: 1, Data: configData}}, events: make(chan pluginapi.ControlEvent, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- New().Run(ctx, h) }()
	defer func() { cancel(); <-done }()
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	deadline := time.After(500 * time.Millisecond)
	for {
		_, _ = conn.Write([]byte{1, 2, 3})
		logs := h.logsCopy()
		if len(logs) > 0 {
			if len(logs) != 1 || !strings.Contains(logs[0], "protocol=2") || !strings.Contains(logs[0], "port=") {
				t.Fatalf("unsafe or unbounded logs: %v", logs)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("no malformed summary")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
