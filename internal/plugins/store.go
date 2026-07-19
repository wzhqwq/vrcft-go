package plugins

import (
	"context"
	"encoding/json"
)

type PluginPreference struct {
	Enabled bool `json:"enabled"`

	Config json.RawMessage `json:"config,omitempty"`
}

type PluginSettings struct {
	Plugins map[string]PluginPreference `json:"plugins"`
}

type Store interface {
	Load(ctx context.Context) (PluginSettings, error)
	Save(ctx context.Context, settings PluginSettings) error
}
