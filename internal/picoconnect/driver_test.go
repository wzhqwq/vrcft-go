package picoconnect

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
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
func (h *driverHost) Log(pluginapi.LogLevel, string) {}
func (h *driverHost) snapshot() ([]pluginapi.DeviceStatus, []trackingmodel.TrackingFrame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]pluginapi.DeviceStatus(nil), h.statuses...), append([]trackingmodel.TrackingFrame(nil), h.frames...)
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
