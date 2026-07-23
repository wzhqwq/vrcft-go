//go:build windows

package ipc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

func randomPipeName(t *testing.T) string {
	t.Helper()
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(value[:])
}

func TestNamedPipeBidirectionalExchange(t *testing.T) {
	name := randomPipeName(t)
	listener, err := Listen(ServerConfig{PipeName: name})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan struct {
		conn protocol.Conn
		err  error
	}, 1)
	go func() {
		conn, err := listener.Accept(context.Background())
		accepted <- struct {
			conn protocol.Conn
			err  error
		}{conn: conn, err: err}
	}()
	client, err := Connect(context.Background(), ClientConfig{PipeName: name})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	serverResult := <-accepted
	if serverResult.err != nil {
		t.Fatal(serverResult.err)
	}
	server := serverResult.conn
	t.Cleanup(func() { _ = server.Close() })

	toServer := testHeartbeatMessage(t)
	serverReceive := receiveAsync(server)
	if err := client.Send(context.Background(), toServer); err != nil {
		t.Fatal(err)
	}
	if result := <-serverReceive; result.err != nil || !reflect.DeepEqual(result.message, toServer) {
		t.Fatalf("server Receive() = (%#v, %v)", result.message, result.err)
	}

	toClient, err := protocol.NewMessage(protocol.Ready{})
	if err != nil {
		t.Fatal(err)
	}
	clientReceive := receiveAsync(client)
	if err := server.Send(context.Background(), toClient); err != nil {
		t.Fatal(err)
	}
	if result := <-clientReceive; result.err != nil || !reflect.DeepEqual(result.message, toClient) {
		t.Fatalf("client Receive() = (%#v, %v)", result.message, result.err)
	}
	if _, err := listener.Accept(context.Background()); !errors.Is(err, ErrListenerConsumed) {
		t.Fatalf("second Accept() error = %v", err)
	}
}

func TestNamedPipeAcceptCancellationCanResume(t *testing.T) {
	name := randomPipeName(t)
	listener, err := Listen(ServerConfig{PipeName: name})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := listener.Accept(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Accept(canceled) error = %v", err)
	}
	client, err := Connect(context.Background(), ClientConfig{PipeName: name})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	server, err := listener.Accept(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
}

func TestNamedPipeConnectCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Connect(ctx, ClientConfig{PipeName: randomPipeName(t)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Connect() error = %v, want canceled", err)
	}
}

func TestNamedPipeListenerCloseWakesAccept(t *testing.T) {
	listener, err := Listen(ServerConfig{PipeName: randomPipeName(t)})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := listener.Accept(context.Background())
		result <- err
	}()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Accept() error = nil")
		}
	case <-time.After(ipcTestTimeout):
		t.Fatal("Close() did not wake Accept")
	}
}

func TestWindowsPipeSecurityConfiguration(t *testing.T) {
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	if sid == "" {
		t.Fatal("currentUserSID() returned blank SID")
	}
	want := "D:P(A;;GA;;;SY)(A;;GA;;;" + sid + ")"
	if got := pipeSecurityDescriptor(sid); got != want {
		t.Fatalf("pipeSecurityDescriptor() = %q, want %q", got, want)
	}
	config := windowsPipeConfig(sid)
	if config.MessageMode {
		t.Fatal("windowsPipeConfig.MessageMode = true")
	}
	if config.InputBufferSize <= 0 || config.OutputBufferSize <= 0 {
		t.Fatalf("windowsPipeConfig buffers = (%d, %d)", config.InputBufferSize, config.OutputBufferSize)
	}
}

func TestNamedPipeRejectsInvalidNamesBeforePlatformCall(t *testing.T) {
	if _, err := Listen(ServerConfig{PipeName: `\\server\pipe\x`}); !errors.Is(err, ErrInvalidPipeName) {
		t.Fatalf("Listen(invalid) error = %v", err)
	}
	if _, err := Connect(context.Background(), ClientConfig{PipeName: "a/b"}); !errors.Is(err, ErrInvalidPipeName) {
		t.Fatalf("Connect(invalid) error = %v", err)
	}
}
