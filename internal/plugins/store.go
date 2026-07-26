package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

type PluginPreference struct {
	Enabled bool             `json:"enabled"`
	Config  pluginapi.Config `json:"config"`
}

type PluginSettings struct {
	Plugins map[string]PluginPreference `json:"plugins"`
}

type Store interface {
	Load(ctx context.Context) (PluginSettings, error)
	Save(ctx context.Context, settings PluginSettings) error
}

type jsonStore struct {
	path     string
	maxBytes int64
}

type wireSettings struct {
	Plugins []wirePreference `json:"plugins"`
}

type wirePreference struct {
	ID      string           `json:"id"`
	Enabled bool             `json:"enabled"`
	Config  pluginapi.Config `json:"config"`
}

// renameJSONStoreFile is a package-private seam for exercising replacement
// failures without weakening the Store API.
var renameJSONStoreFile = os.Rename

func NewJSONStore(path string, maxBytes int64) (Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("plugins: store path must be nonblank")
	}
	if maxBytes <= 0 {
		return nil, errors.New("plugins: store maxBytes must be positive")
	}
	return &jsonStore{path: path, maxBytes: maxBytes}, nil
}

func (s *jsonStore) Load(ctx context.Context) (PluginSettings, error) {
	if err := ctx.Err(); err != nil {
		return PluginSettings{}, err
	}

	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyPluginSettings(), nil
	}
	if err != nil {
		return PluginSettings{}, fmt.Errorf("plugins: open settings: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, s.maxBytes+1))
	if err != nil {
		return PluginSettings{}, fmt.Errorf("plugins: read settings: %w", err)
	}
	if int64(len(data)) > s.maxBytes {
		return PluginSettings{}, fmt.Errorf("plugins: settings file exceeds %d bytes", s.maxBytes)
	}

	var wire wireSettings
	if err := decodeStrictStoreJSON(data, &wire); err != nil {
		return PluginSettings{}, fmt.Errorf("plugins: decode settings: %w", err)
	}
	settings, err := settingsFromWire(wire, s.maxBytes)
	if err != nil {
		return PluginSettings{}, err
	}
	return clonePluginSettings(settings), nil
}

func (s *jsonStore) Save(ctx context.Context, settings PluginSettings) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := encodeSettings(settings, s.maxBytes)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".plugins-*.tmp")
	if err != nil {
		return fmt.Errorf("plugins: create temporary settings: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("plugins: secure temporary settings: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("plugins: write temporary settings: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("plugins: sync temporary settings: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("plugins: close temporary settings: %w", err)
	}
	if err := renameJSONStoreFile(temporaryPath, s.path); err != nil {
		return fmt.Errorf("plugins: replace settings: %w", err)
	}
	return nil
}

func encodeSettings(settings PluginSettings, maxBytes int64) ([]byte, error) {
	wire := wireSettings{Plugins: make([]wirePreference, 0, len(settings.Plugins))}
	for id, preference := range settings.Plugins {
		config, err := validConfig(id, preference.Config, maxBytes)
		if err != nil {
			return nil, err
		}
		wire.Plugins = append(wire.Plugins, wirePreference{ID: id, Enabled: preference.Enabled, Config: config})
	}
	sort.Slice(wire.Plugins, func(i, j int) bool { return wire.Plugins[i].ID < wire.Plugins[j].ID })
	data, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("plugins: encode settings: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("plugins: encoded settings exceed %d bytes", maxBytes)
	}
	return data, nil
}

func settingsFromWire(wire wireSettings, maxBytes int64) (PluginSettings, error) {
	settings := emptyPluginSettings()
	for _, preference := range wire.Plugins {
		if _, exists := settings.Plugins[preference.ID]; exists {
			return PluginSettings{}, fmt.Errorf("plugins: duplicate preference ID %q", preference.ID)
		}
		config, err := validConfig(preference.ID, preference.Config, maxBytes)
		if err != nil {
			return PluginSettings{}, err
		}
		settings.Plugins[preference.ID] = PluginPreference{Enabled: preference.Enabled, Config: config}
	}
	return settings, nil
}

func validConfig(id string, config pluginapi.Config, maxBytes int64) (pluginapi.Config, error) {
	if int64(len(config.Data)) > maxBytes {
		return pluginapi.Config{}, fmt.Errorf("plugins: config for %q exceeds %d bytes", id, maxBytes)
	}
	if err := config.Validate(); err != nil {
		return pluginapi.Config{}, fmt.Errorf("plugins: config for %q is invalid", id)
	}
	return config.Clone(), nil
}

func clonePluginSettings(settings PluginSettings) PluginSettings {
	clone := emptyPluginSettings()
	for id, preference := range settings.Plugins {
		clone.Plugins[id] = PluginPreference{Enabled: preference.Enabled, Config: preference.Config.Clone()}
	}
	return clone
}

func emptyPluginSettings() PluginSettings {
	return PluginSettings{Plugins: make(map[string]PluginPreference)}
}

func decodeStrictStoreJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("unexpected trailing JSON value")
}
