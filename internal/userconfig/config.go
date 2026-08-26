package userconfig

import (
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/wzhqwq/vrcft-go/internal/application"
	"github.com/wzhqwq/vrcft-go/internal/avatar"
	"github.com/wzhqwq/vrcft-go/internal/osc"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/processing"
)

type ValidationError struct {
	Field string
	Err   error
}

func (e *ValidationError) Error() string       { return fmt.Sprintf("userconfig: %s: %v", e.Field, e.Err) }
func (e *ValidationError) Unwrap() error       { return e.Err }
func validation(field string, err error) error { return &ValidationError{Field: field, Err: err} }

func DefaultCandidate(paths Paths) Candidate {
	config := processingDefaultWire()
	return Candidate{Avatar: Avatar{OSCRoot: paths.DefaultOSCRoot}, Plugins: Plugins{DevRoots: []string{}}, Processing: config, OSC: OSC{TargetMode: osc.TargetModeAuto}}
}

func processingDefaultWire() Processing {
	value, err := processingToWire(processing.DefaultConfig())
	if err != nil {
		panic(err)
	}
	return value
}

func Normalize(candidate Candidate) (Candidate, error) {
	normalized := candidate.Clone()
	root, err := normalizedPath(normalized.Avatar.OSCRoot)
	if err != nil || root == "" {
		if err == nil {
			err = errors.New("required")
		}
		return Candidate{}, validation("avatar.oscRoot", err)
	}
	normalized.Avatar.OSCRoot = root
	if normalized.Avatar.FallbackPath != "" {
		fallback, err := normalizedPath(normalized.Avatar.FallbackPath)
		if err != nil {
			return Candidate{}, validation("avatar.fallbackPath", err)
		}
		normalized.Avatar.FallbackPath = fallback
	}
	roots := make([]string, len(normalized.Plugins.DevRoots))
	seen := make(map[string]struct{}, len(roots))
	for i, root := range normalized.Plugins.DevRoots {
		cleaned, err := normalizedPath(root)
		if err != nil || cleaned == "" {
			if err == nil {
				err = errors.New("required")
			}
			return Candidate{}, validation("plugins.devRoots", err)
		}
		key := strings.ToLower(cleaned)
		if _, exists := seen[key]; exists {
			return Candidate{}, validation("plugins.devRoots", errors.New("duplicate development root"))
		}
		seen[key] = struct{}{}
		roots[i] = cleaned
	}
	sort.Slice(roots, func(i, j int) bool {
		left, right := strings.ToLower(roots[i]), strings.ToLower(roots[j])
		if left == right {
			return roots[i] < roots[j]
		}
		return left < right
	})
	normalized.Plugins.DevRoots = roots
	if _, err := processingFromWire(normalized.Processing); err != nil {
		return Candidate{}, validation("processing", err)
	}
	if err := normalizeOSC(&normalized.OSC); err != nil {
		return Candidate{}, err
	}
	return normalized, nil
}

func normalizedPath(value string) (string, error) {
	if strings.IndexByte(value, 0) >= 0 {
		return "", errors.New("NUL path")
	}
	if value == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func normalizeOSC(value *OSC) error {
	if value.TargetMode == "" {
		value.TargetMode = osc.TargetModeAuto
	}
	switch value.TargetMode {
	case osc.TargetModeAuto:
		if value.ManualHost != "" {
			return validation("osc.manualHost", errors.New("must be empty in automatic mode"))
		}
		if value.ManualPort != 0 {
			return validation("osc.manualPort", errors.New("must be empty in automatic mode"))
		}
	case osc.TargetModeManual:
		if value.PreferredService != "" {
			return validation("osc.preferredService", errors.New("must be empty in manual mode"))
		}
		if value.ManualHost == "" {
			return validation("osc.manualHost", errors.New("required in manual mode"))
		}
		if value.ManualPort <= 0 || value.ManualPort > 65535 {
			return validation("osc.manualPort", errors.New("must be a valid port"))
		}
		address, err := netip.ParseAddr(value.ManualHost)
		if err != nil || address.IsUnspecified() || address.IsMulticast() || value.ManualHost == "255.255.255.255" {
			return validation("osc.manualHost", errors.New("must be a unicast IP literal"))
		}
	default:
		return validation("osc.targetMode", errors.New("must be auto or manual"))
	}
	return nil
}

func ApplicationConfig(settings Settings, paths Paths) (application.Config, error) {
	if settings.SchemaVersion != SchemaVersion {
		return application.Config{}, validation("schemaVersion", errors.New("unsupported schema version"))
	}
	if settings.Revision == 0 {
		return application.Config{}, validation("revision", errors.New("must be positive"))
	}
	candidate := Candidate{Avatar: settings.Avatar, Plugins: settings.Plugins, Processing: settings.Processing, OSC: settings.OSC}
	normalized, err := Normalize(candidate)
	if err != nil {
		return application.Config{}, err
	}
	if !reflect.DeepEqual(candidate, normalized) {
		return application.Config{}, validation("settings", errors.New("settings must be normalized"))
	}
	processingConfig, err := processingFromWire(normalized.Processing)
	if err != nil {
		return application.Config{}, validation("processing", err)
	}
	config := application.Config{Avatar: avatar.PlannerConfig{OSCRoot: normalized.Avatar.OSCRoot, FallbackPath: normalized.Avatar.FallbackPath}, PluginCatalog: plugins.DirectoryCatalogConfig{BuiltinRoot: paths.BuiltinPluginDir, DevRoots: append([]string(nil), normalized.Plugins.DevRoots...)}, PluginStorePath: paths.PluginStoreFile, PluginStoreMaxBytes: MaxPluginConfigBytes, PluginOptions: plugins.DefaultOptions(), Processing: processingConfig, OSC: osc.ControllerConfig{TargetMode: normalized.OSC.TargetMode, PreferredVRChatService: normalized.OSC.PreferredService, ManualTarget: osc.OSCTarget{Host: normalized.OSC.ManualHost, Port: normalized.OSC.ManualPort}}}
	if _, err := application.NewApp(config); err != nil {
		return application.Config{}, fmt.Errorf("userconfig: application config: %w", err)
	}
	return config, nil
}
