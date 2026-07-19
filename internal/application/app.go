package application

import (
	"context"
	"fmt"

	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/tracking"
)

type Application struct {
	wailsCtx context.Context

	plugins  plugins.Manager
	tracking tracking.Service
	osc      osc.OSCService
}

func NewApp() *Application {
	a := &Application{
		osc: osc.NewOSCService(),
	}

	return a
}

func (a *Application) Start(ctx context.Context) error {
	if err := a.osc.Start(ctx); err != nil {
		return fmt.Errorf("failed to start osc service: %w", err)
	}

	return nil
}
func (a *Application) Close(ctx context.Context) error {
	// reverse order
	if err := a.osc.Close(ctx); err != nil {
		return fmt.Errorf("failed to stop osc service: %w", err)
	}

	return nil
}
