package osc_test

import (
	"context"
	"fmt"
	"net"

	"github.com/wzhqwq/vrcft-go/pkg/osc"
)

func ExampleServer() {
	server, err := osc.ListenUDP("127.0.0.1:0")
	if err != nil {
		return
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = server.Serve(ctx, func(message osc.Message, remote *net.UDPAddr) {
			fmt.Printf("%s from %s\n", message.Address, remote)
		})
	}()
}

func ExampleMarshalMessage() {
	packet, err := osc.MarshalMessage(osc.Message{
		Address: "/plugin/status",
		Args:    []osc.Value{osc.Float32(1)},
	})
	fmt.Println(err == nil && len(packet) > 0)
	// Output: true
}
