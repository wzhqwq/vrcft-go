package plugins

import (
	"context"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

type Manifest struct {
	SchemaVersion int `json:"schemaVersion"`

	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`

	ProtocolMin uint16 `json:"protocolMin"`
	ProtocolMax uint16 `json:"protocolMax"`

	Entrypoint string `json:"entrypoint"`

	Capabilities trackingmodel.Capability `json:"capabilities"`
}

type Registry interface {
	Scan(ctx context.Context) ([]InstalledPlugin, error)
}

type InstalledPlugin struct {
	Manifest Manifest
	RootDir  string
	Source   Source
}

type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceDev     Source = "dev"
)
