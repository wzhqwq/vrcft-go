//go:build ignore

// Run from the repository root with: go run ./scripts/diagnose-controller-avatar.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/osc"
)

func main() {
	queryURL := flag.String("url", "", "optional OSCQuery URL to inspect directly")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if *queryURL != "" {
		node, err := osc.NewQueryClient(3*time.Second).Node(ctx, *queryURL, "/avatar")
		if err != nil {
			fail(err)
		}
		change := node.Contents["change"]
		fmt.Printf("direct query: avatar=%q change=%+v\n", node.FullPath, change)
	}
	controller, err := osc.NewController(osc.ControllerConfig{
		ServiceName: "VRCFT-Go-Avatar-Diagnostic",
		CatalogMode: osc.CatalogExternal,
	}, nil, nil)
	if err != nil {
		fail(err)
	}
	changes := controller.AvatarChanges(ctx)
	if err := controller.Start(ctx); err != nil {
		fail(err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer closeCancel()
		if err := controller.Close(closeCtx); err != nil {
			fmt.Fprintln(os.Stderr, "close controller:", err)
		}
	}()

	for {
		select {
		case change, ok := <-changes:
			if !ok {
				fail(fmt.Errorf("avatar change subscription closed"))
			}
			fmt.Printf("avatar ID: %s\nrevision: %d\nstatus: %+v\n", change.AvatarID, change.Revision, controller.Status())
			return
		case event := <-controller.Events():
			fmt.Printf("event: kind=%s service=%q message=%q error=%v\n", event.Kind, event.Service, event.Message, event.Err)
		case <-ctx.Done():
			fail(fmt.Errorf("no avatar after 15 seconds; status: %+v", controller.Status()))
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
