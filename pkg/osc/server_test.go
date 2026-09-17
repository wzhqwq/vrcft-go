package osc_test

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/osc"
)

type delivery struct {
	message osc.Message
	remote  *net.UDPAddr
}

func TestServerServesMessagesInOrderAndDropsMalformedDatagrams(t *testing.T) {
	server := listenServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	received := make(chan string, 3)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(ctx, func(message osc.Message, _ *net.UDPAddr) {
			received <- message.Address
		})
	}()
	client := dialServer(t, server)
	defer client.Close()
	if _, err := client.Write([]byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	unsupported := mustMarshalMessage(t, "/bad")
	unsupported[9] = 'b'
	if _, err := client.Write(unsupported); err != nil {
		t.Fatal(err)
	}
	writeBundle(t, client, "/one", "/two", "/three")
	for _, want := range []string{"/one", "/two", "/three"} {
		select {
		case got := <-received:
			if got != want {
				t.Fatalf("address = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %s", want)
		}
	}
	cancel()
	if err := waitServer(t, serveErr); err != nil {
		t.Fatalf("Serve = %v", err)
	}
}

func TestServerRejectsNilContextAndHandler(t *testing.T) {
	server := listenServer(t)
	if err := server.Serve(nil, func(osc.Message, *net.UDPAddr) {}); !errors.Is(err, osc.ErrInvalidArgument) {
		t.Fatalf("Serve(nil, handler) = %v, want ErrInvalidArgument", err)
	}
	if err := server.Serve(context.Background(), nil); !errors.Is(err, osc.ErrInvalidArgument) {
		t.Fatalf("Serve(ctx, nil) = %v, want ErrInvalidArgument", err)
	}
}

func TestServerRejectsConcurrentServe(t *testing.T) {
	server := listenServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 1)
	first := make(chan error, 1)
	go func() {
		first <- server.Serve(ctx, func(osc.Message, *net.UDPAddr) { started <- struct{}{} })
	}()
	client := dialServer(t, server)
	defer client.Close()
	writeMessage(t, client, "/started")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first Serve")
	}
	if err := server.Serve(context.Background(), func(osc.Message, *net.UDPAddr) {}); !errors.Is(err, osc.ErrServerRunning) {
		t.Fatalf("second Serve = %v, want ErrServerRunning", err)
	}
	cancel()
	if err := waitServer(t, first); err != nil {
		t.Fatalf("first Serve = %v", err)
	}
}

func TestServerCanServeAgainAfterCancellation(t *testing.T) {
	server := listenServer(t)
	firstCtx, firstCancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { first <- server.Serve(firstCtx, func(osc.Message, *net.UDPAddr) {}) }()
	firstCancel()
	if err := waitServer(t, first); err != nil {
		t.Fatalf("first Serve = %v", err)
	}

	secondCtx, secondCancel := context.WithCancel(context.Background())
	defer secondCancel()
	received := make(chan string, 1)
	second := make(chan error, 1)
	go func() {
		second <- server.Serve(secondCtx, func(message osc.Message, _ *net.UDPAddr) { received <- message.Address })
	}()
	client := dialServer(t, server)
	defer client.Close()
	writeMessage(t, client, "/reused")
	select {
	case got := <-received:
		if got != "/reused" {
			t.Fatalf("address = %q, want /reused", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reused Serve")
	}
	secondCancel()
	if err := waitServer(t, second); err != nil {
		t.Fatalf("second Serve = %v", err)
	}
}

func TestServerCloseUnblocksServe(t *testing.T) {
	server := listenServer(t)
	started := make(chan struct{}, 1)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(context.Background(), func(osc.Message, *net.UDPAddr) { started <- struct{}{} })
	}()
	client := dialServer(t, server)
	defer client.Close()
	writeMessage(t, client, "/started")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Serve")
	}
	if err := server.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	if err := waitServer(t, serveErr); err != nil {
		t.Fatalf("Serve = %v", err)
	}
}

func TestServerHandlerCanCloseServer(t *testing.T) {
	server := listenServer(t)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(context.Background(), func(osc.Message, *net.UDPAddr) {
			if err := server.Close(); err != nil {
				t.Errorf("Close from handler = %v", err)
			}
		})
	}()
	client := dialServer(t, server)
	defer client.Close()
	writeMessage(t, client, "/close")
	if err := waitServer(t, serveErr); err != nil {
		t.Fatalf("Serve = %v", err)
	}
}

func TestServerCloseIsIdempotentAndLocalAddrIsCopied(t *testing.T) {
	server := listenServer(t)
	before := server.LocalAddr()
	before.Port = 1
	before.IP[0] ^= 0xff
	if got := server.LocalAddr(); got.Port == before.Port || got.IP[0] == before.IP[0] {
		t.Fatalf("LocalAddr returned shared address: got %v after mutation", got)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("first Close = %v", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
	after := server.LocalAddr()
	if after == nil || after.Port == 0 {
		t.Fatalf("LocalAddr after Close = %v, want bound address", after)
	}
}

func TestServerRejectsPostCloseOperationsAndInvalidSends(t *testing.T) {
	server := listenServer(t)
	packet := mustMarshalMessage(t, "/send")
	if err := server.SendTo(nil, server.LocalAddr()); !errors.Is(err, osc.ErrMalformedPacket) {
		t.Fatalf("SendTo(empty) = %v, want ErrMalformedPacket", err)
	}
	if err := server.SendTo(packet, nil); !errors.Is(err, osc.ErrInvalidArgument) {
		t.Fatalf("SendTo(nil target) = %v, want ErrInvalidArgument", err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if err := server.Serve(context.Background(), func(osc.Message, *net.UDPAddr) {}); !errors.Is(err, osc.ErrServerClosed) {
		t.Fatalf("Serve after Close = %v, want ErrServerClosed", err)
	}
	if err := server.SendTo(packet, server.LocalAddr()); !errors.Is(err, osc.ErrServerClosed) {
		t.Fatalf("SendTo after Close = %v, want ErrServerClosed", err)
	}
}

func TestServerSendToUsesBoundSocket(t *testing.T) {
	server := listenServer(t)
	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	packet := mustMarshalMessage(t, "/sent")
	if err := server.SendTo(packet, client.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatalf("SendTo = %v", err)
	}
	buffer := make([]byte, 128)
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	n, source, err := client.ReadFromUDP(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(buffer[:n], packet) {
		t.Fatalf("packet = %x, want %x", buffer[:n], packet)
	}
	if source.Port != server.LocalAddr().Port {
		t.Fatalf("source port = %d, want %d", source.Port, server.LocalAddr().Port)
	}
}

func TestServerDeliversRetainedMessageAndRemoteIndependently(t *testing.T) {
	server := listenServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	deliveries := make(chan delivery, 2)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(ctx, func(message osc.Message, remote *net.UDPAddr) {
			deliveries <- delivery{message: message, remote: remote}
		})
	}()
	first := dialServer(t, server)
	defer first.Close()
	second := dialServer(t, server)
	defer second.Close()
	packet, err := osc.MarshalMessage(osc.Message{Address: "/retained", Args: []osc.Value{osc.String("value")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Write(packet); err != nil {
		t.Fatal(err)
	}
	firstDelivery := waitDelivery(t, deliveries)
	writeMessage(t, second, "/later")
	secondDelivery := waitDelivery(t, deliveries)
	if firstDelivery.message.Address != "/retained" || firstDelivery.message.Args[0].Str != "value" {
		t.Fatalf("first message = %#v, want retained values", firstDelivery.message)
	}
	if firstDelivery.remote.Port != first.LocalAddr().(*net.UDPAddr).Port {
		t.Fatalf("first remote = %v, want source %v", firstDelivery.remote, first.LocalAddr())
	}
	if secondDelivery.remote.Port == firstDelivery.remote.Port {
		t.Fatalf("remote address was reused: first = %v, second = %v", firstDelivery.remote, secondDelivery.remote)
	}
	cancel()
	if err := waitServer(t, serveErr); err != nil {
		t.Fatalf("Serve = %v", err)
	}
}

func TestServerConcurrentLifecycleOperations(t *testing.T) {
	server := listenServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 1)
	var startedOnce sync.Once
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(ctx, func(osc.Message, *net.UDPAddr) {
			startedOnce.Do(func() { started <- struct{}{} })
		})
	}()
	packet := mustMarshalMessage(t, "/concurrent")
	client := dialServer(t, server)
	defer client.Close()
	if _, err := client.Write(packet); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Serve")
	}
	var group sync.WaitGroup
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			_ = server.SendTo(packet, server.LocalAddr())
			_ = server.LocalAddr()
		}()
	}
	group.Add(1)
	go func() {
		defer group.Done()
		_ = server.Close()
	}()
	group.Wait()
	if err := waitServer(t, serveErr); err != nil {
		t.Fatalf("Serve = %v", err)
	}
}

func listenServer(t *testing.T) *osc.Server {
	t.Helper()
	server, err := osc.ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func dialServer(t *testing.T, server *osc.Server) *net.UDPConn {
	t.Helper()
	client, err := net.DialUDP("udp", nil, server.LocalAddr())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func writeMessage(t *testing.T, client *net.UDPConn, address string) {
	t.Helper()
	packet := mustMarshalMessage(t, address)
	if _, err := client.Write(packet); err != nil {
		t.Fatal(err)
	}
}

func writeBundle(t *testing.T, client *net.UDPConn, addresses ...string) {
	t.Helper()
	elements := make([][]byte, 0, len(addresses))
	for _, address := range addresses {
		elements = append(elements, mustMarshalMessage(t, address))
	}
	packet, err := osc.MarshalBundle(osc.Bundle{Elements: elements})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write(packet); err != nil {
		t.Fatal(err)
	}
}

func waitServer(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Serve")
		return nil
	}
}

func waitDelivery(t *testing.T, deliveries <-chan delivery) delivery {
	t.Helper()
	select {
	case delivery := <-deliveries:
		return delivery
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delivery")
		return delivery{}
	}
}
