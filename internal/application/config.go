package application

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/processing"
)

type Config struct {
	Avatar               avatar.PlannerConfig
	PluginCatalog        plugins.DirectoryCatalogConfig
	PluginStorePath      string
	PluginStoreMaxBytes  int64
	PluginOptions        plugins.Options
	Processing           processing.Config
	OSC                  osc.ControllerConfig
	FrameInterval        time.Duration
	PluginControlTimeout time.Duration
}

const (
	DefaultFrameInterval        = 10 * time.Millisecond
	DefaultPluginControlTimeout = 2 * time.Second
)

var ErrInvalidConfig = errors.New("application: invalid config")

type normalizedConfig struct {
	avatar               avatar.PlannerConfig
	pluginCatalog        plugins.DirectoryCatalogConfig
	pluginStorePath      string
	pluginStoreMaxBytes  int64
	pluginOptions        plugins.Options
	processing           processing.Config
	osc                  osc.ControllerConfig
	frameInterval        time.Duration
	pluginControlTimeout time.Duration
}

func normalizeConfig(config Config) (normalizedConfig, error) {
	if config.FrameInterval < 0 {
		return normalizedConfig{}, fmt.Errorf("%w: frame interval must not be negative", ErrInvalidConfig)
	}
	if config.PluginControlTimeout < 0 {
		return normalizedConfig{}, fmt.Errorf("%w: plugin control timeout must not be negative", ErrInvalidConfig)
	}

	normalized := normalizedConfig{
		avatar:               config.Avatar,
		pluginCatalog:        cloneDirectoryCatalogConfig(config.PluginCatalog),
		pluginStorePath:      config.PluginStorePath,
		pluginStoreMaxBytes:  config.PluginStoreMaxBytes,
		pluginOptions:        config.PluginOptions,
		processing:           cloneProcessingConfig(config.Processing),
		osc:                  cloneControllerConfig(config.OSC),
		frameInterval:        config.FrameInterval,
		pluginControlTimeout: config.PluginControlTimeout,
	}
	if normalized.frameInterval == 0 {
		normalized.frameInterval = DefaultFrameInterval
	}
	if normalized.pluginControlTimeout == 0 {
		normalized.pluginControlTimeout = DefaultPluginControlTimeout
	}
	normalized.osc.CatalogMode = osc.CatalogExternal

	if _, err := avatar.NewPlanner(normalized.avatar); err != nil {
		return normalizedConfig{}, fmt.Errorf("%w: avatar planner: %w", ErrInvalidConfig, err)
	}
	if _, err := plugins.NewDirectoryCatalog(normalized.pluginCatalog); err != nil {
		return normalizedConfig{}, fmt.Errorf("%w: plugin catalog: %w", ErrInvalidConfig, err)
	}
	if _, err := plugins.NewJSONStore(normalized.pluginStorePath, normalized.pluginStoreMaxBytes); err != nil {
		return normalizedConfig{}, fmt.Errorf("%w: plugin store: %w", ErrInvalidConfig, err)
	}
	if _, err := processing.NewPipeline(normalized.processing); err != nil {
		return normalizedConfig{}, fmt.Errorf("%w: processing: %w", ErrInvalidConfig, err)
	}
	if _, err := osc.NewOSCService(normalized.osc); err != nil {
		return normalizedConfig{}, fmt.Errorf("%w: OSC: %w", ErrInvalidConfig, err)
	}
	return normalized, nil
}

func cloneDirectoryCatalogConfig(config plugins.DirectoryCatalogConfig) plugins.DirectoryCatalogConfig {
	config.DevRoots = append([]string(nil), config.DevRoots...)
	return config
}

func cloneProcessingConfig(config processing.Config) processing.Config {
	if config.Overrides != nil {
		overrides := make(map[processing.ChannelID]processing.ChannelConfig, len(config.Overrides))
		for id, channel := range config.Overrides {
			overrides[id] = channel
		}
		config.Overrides = overrides
	}
	if config.MutualExclusion != nil {
		groups := make([][]processing.ChannelID, len(config.MutualExclusion))
		for index, group := range config.MutualExclusion {
			groups[index] = append([]processing.ChannelID(nil), group...)
		}
		config.MutualExclusion = groups
	}
	return config
}

func cloneControllerConfig(config osc.ControllerConfig) osc.ControllerConfig {
	if config.Interfaces == nil {
		return config
	}
	interfaces := make([]net.Interface, len(config.Interfaces))
	for index, iface := range config.Interfaces {
		iface.HardwareAddr = append(net.HardwareAddr(nil), iface.HardwareAddr...)
		interfaces[index] = iface
	}
	config.Interfaces = interfaces
	return config
}
