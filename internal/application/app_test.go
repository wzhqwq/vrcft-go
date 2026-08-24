package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/osc"
)

func TestNewAppStoresOSCConstructionErrorForStart(t *testing.T) {
	want := errors.New("OSC construction failed")
	previous := newOSCService
	newOSCService = func(osc.ControllerConfig) (osc.OSCService, error) {
		return nil, want
	}
	t.Cleanup(func() { newOSCService = previous })

	app := NewApp()
	if app == nil {
		t.Fatal("NewApp() = nil")
	}
	err := app.Start(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("Start() error = %v, want error matching %v", err, want)
	}
	if !strings.Contains(err.Error(), "construct OSC service") {
		t.Fatalf("Start() error = %q, want construction context", err)
	}
}

func TestNewAppCloseAfterOSCConstructionFailureIsNoOp(t *testing.T) {
	want := errors.New("OSC construction failed")
	previous := newOSCService
	newOSCService = func(osc.ControllerConfig) (osc.OSCService, error) {
		return nil, want
	}
	t.Cleanup(func() { newOSCService = previous })

	app := NewApp()
	if err := app.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}
