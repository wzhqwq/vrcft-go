//go:build !windows

package ipc

import (
	"context"
	"errors"
	"testing"
)

func TestPlatformOtherReturnsUnsupported(t *testing.T) {
	if _, err := Listen(ServerConfig{PipeName: "plugin"}); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Listen() error = %v", err)
	}
	if _, err := Connect(context.Background(), ClientConfig{PipeName: "plugin"}); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Connect() error = %v", err)
	}
}

func TestPlatformOtherStillValidatesNames(t *testing.T) {
	if _, err := Listen(ServerConfig{}); !errors.Is(err, ErrInvalidPipeName) {
		t.Fatalf("Listen(invalid) error = %v", err)
	}
	if _, err := Connect(context.Background(), ClientConfig{}); !errors.Is(err, ErrInvalidPipeName) {
		t.Fatalf("Connect(invalid) error = %v", err)
	}
}
