package osc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	pkgosc "github.com/wzhqwq/vrcft-go/pkg/osc"
)

func TestUDPTransportUsesPublicServer(t *testing.T) {
	transport, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transport.Close() })
	if transport.server == nil {
		t.Fatal("UDPTransport has no public OSC server")
	}
}

func TestUDPTransportTargetOwnership(t *testing.T) {
	transport := &UDPTransport{}
	input := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9000}
	transport.SetTarget(input)
	input.IP[0], input.Port = 8, 1
	first := transport.Target()
	if first.String() != "127.0.0.1:9000" {
		t.Fatalf("Target = %v", first)
	}
	first.IP[0] = 9
	if got := transport.Target().String(); got != "127.0.0.1:9000" {
		t.Fatalf("retained target changed to %s", got)
	}
}

func TestUDPTransportSendsFromBoundSocket(t *testing.T) {
	transport, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transport.Close() })

	receiver, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = receiver.Close() })
	transport.SetTarget(receiver.LocalAddr().(*net.UDPAddr))

	packet, err := pkgosc.MarshalMessage(pkgosc.Message{Address: "/sent", Args: []pkgosc.Value{pkgosc.String("value")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := transport.Send(packet); err != nil {
		t.Fatalf("Send = %v", err)
	}

	buffer := make([]byte, 1024)
	_ = receiver.SetReadDeadline(time.Now().Add(time.Second))
	count, source, err := receiver.ReadFromUDP(buffer)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := pkgosc.UnmarshalPacket(buffer[:count])
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Address != "/sent" || len(messages[0].Args) != 1 || messages[0].Args[0].Str != "value" {
		t.Fatalf("received messages = %#v", messages)
	}
	if source.Port != transport.LocalAddr().Port {
		t.Fatalf("source port = %d, want %d", source.Port, transport.LocalAddr().Port)
	}
}

func TestUDPTransportDelegatesServe(t *testing.T) {
	transport, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transport.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	received := make(chan Message, 1)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- transport.Serve(ctx, func(message Message, _ *net.UDPAddr) {
			received <- message
		})
	}()

	client, err := net.DialUDP("udp", nil, transport.LocalAddr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	packet, err := pkgosc.MarshalMessage(pkgosc.Message{Address: "/received"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write(packet); err != nil {
		t.Fatal(err)
	}

	select {
	case message := <-received:
		if message.Address != "/received" {
			t.Fatalf("message = %#v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delegated Serve")
	}
	cancel()
	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("Serve = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Serve to stop")
	}
}

func TestUDPTransportEmptyAndUnstartedSafety(t *testing.T) {
	transport := &UDPTransport{}
	if transport.LocalAddr() != nil {
		t.Fatalf("LocalAddr = %v, want nil", transport.LocalAddr())
	}
	if err := transport.Send([]byte{1}); err == nil || err.Error() != "OSC target is not configured" {
		t.Fatalf("Send without target = %v", err)
	}
	transport.SetTarget(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9000})
	if err := transport.Send([]byte{1}); err == nil {
		t.Fatal("Send on unstarted transport succeeded")
	}
	if err := transport.Send(nil); err != nil {
		t.Fatalf("Send empty packet = %v", err)
	}
	transport.SetTarget(nil)
	if target := transport.Target(); target != nil {
		t.Fatalf("Target = %v, want nil", target)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("first Close = %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
}

func TestUDPTransportCloseIsIdempotent(t *testing.T) {
	transport, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	transport.SetTarget(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9000})
	if err := transport.Close(); err != nil {
		t.Fatalf("first Close = %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
	if err := transport.Send([]byte{1}); err == nil || errors.Is(err, pkgosc.ErrServerClosed) == false {
		t.Fatalf("Send after Close = %v, want ErrServerClosed", err)
	}
}
