// Package userconfig owns the versioned, user-editable M7 configuration document.
package userconfig

import (
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/wzhqwq/vrcft-go/internal/osc"
)

const (
	SchemaVersion        = 1
	MaxSettingsBytes     = 256 << 10
	MaxPluginConfigBytes = 64 << 10

	// Windows supports extended paths up to roughly 32 KiB. The path limits
	// preserve that practical maximum while the document limit keeps the Wails
	// payload finite. Processing currently has 85 fixed channels (10 eye plus
	// the generated expression catalog), so 128 leaves room for catalog growth
	// without allowing caller-controlled unbounded nested collections.
	MaxSettingsPathBytes              = 32 << 10
	MaxSettingsEndpointBytes          = 255
	MaxSettingsTargetModeBytes        = 16
	MaxSettingsFilterModeBytes        = 32
	MaxSettingsChannelNameBytes       = 128
	MaxSettingsDevRoots               = 128
	MaxSettingsOverrides              = 128
	MaxSettingsMutualExclusionGroups  = 128
	MaxSettingsMutualExclusionMembers = 128
)

// Settings is the persisted SettingsV1 document.
type Settings struct {
	SchemaVersion int        `json:"schemaVersion"`
	Revision      uint64     `json:"revision"`
	Avatar        Avatar     `json:"avatar"`
	Plugins       Plugins    `json:"plugins"`
	Processing    Processing `json:"processing"`
	OSC           OSC        `json:"osc"`
}

// Candidate is user-editable settings content without persistence metadata.
type Candidate struct {
	Avatar     Avatar     `json:"avatar"`
	Plugins    Plugins    `json:"plugins"`
	Processing Processing `json:"processing"`
	OSC        OSC        `json:"osc"`
}

type Avatar struct {
	OSCRoot      string `json:"oscRoot"`
	FallbackPath string `json:"fallbackPath"`
}

type Plugins struct {
	DevRoots []string `json:"devRoots"`
}

// Processing is an explicit, millisecond-based processing wire DTO.
type Processing struct {
	DefaultChannel     ProcessingChannel    `json:"defaultChannel"`
	Overrides          []ProcessingOverride `json:"overrides"`
	ActiveStaleAfterMs int64                `json:"activeStaleAfterMs"`
	MutualExclusion    [][]string           `json:"mutualExclusion"`
}

type ProcessingOverride struct {
	Name    string            `json:"name"`
	Channel ProcessingChannel `json:"channel"`
}

type ProcessingChannel struct {
	Calibration Calibration `json:"calibration"`
	Tuning      Tuning      `json:"tuning"`
	Filter      Filter      `json:"filter"`
	Dropout     Dropout     `json:"dropout"`
}

type Calibration struct {
	Enabled bool    `json:"enabled"`
	Neutral float32 `json:"neutral"`
	Min     float32 `json:"min"`
	Max     float32 `json:"max"`
	Gain    float32 `json:"gain"`
	Invert  bool    `json:"invert"`
}

type Tuning struct {
	Deadzone     float32 `json:"deadzone"`
	Gain         float32 `json:"gain"`
	Exponent     float32 `json:"exponent"`
	ClampEnabled bool    `json:"clampEnabled"`
	ClampMin     float32 `json:"clampMin"`
	ClampMax     float32 `json:"clampMax"`
}

type Filter struct {
	Mode             string  `json:"mode"`
	EMAAlpha         float32 `json:"emaAlpha"`
	MinCutoff        float32 `json:"minCutoff"`
	Beta             float32 `json:"beta"`
	DerivativeCutoff float32 `json:"derivativeCutoff"`
}

type Dropout struct {
	HoldDurationMs  int64 `json:"holdDurationMs"`
	DecayDurationMs int64 `json:"decayDurationMs"`
	StaleAfterMs    int64 `json:"staleAfterMs"`
}

type OSC struct {
	TargetMode       osc.TargetMode `json:"targetMode"`
	PreferredService string         `json:"preferredService"`
	ManualHost       string         `json:"manualHost"`
	ManualPort       int            `json:"manualPort"`
}

// ValidateCandidateBounds rejects Wails-facing user input before a clone,
// normalizer, backend, or response builder can allocate from its collection
// shape. It deliberately performs only shape/encoding admission; Normalize
// remains the owner of semantic and lower-level configuration validation.
func ValidateCandidateBounds(candidate Candidate) error {
	if err := validateSettingsString("avatar.oscRoot", candidate.Avatar.OSCRoot, MaxSettingsPathBytes); err != nil {
		return err
	}
	if err := validateSettingsString("avatar.fallbackPath", candidate.Avatar.FallbackPath, MaxSettingsPathBytes); err != nil {
		return err
	}
	if len(candidate.Plugins.DevRoots) > MaxSettingsDevRoots {
		return validation("plugins.devRoots", errors.New("too many development roots"))
	}
	for _, root := range candidate.Plugins.DevRoots {
		if err := validateSettingsString("plugins.devRoots", root, MaxSettingsPathBytes); err != nil {
			return err
		}
	}
	if len(candidate.Processing.Overrides) > MaxSettingsOverrides {
		return validation("processing.overrides", errors.New("too many overrides"))
	}
	if err := validateProcessingChannelBounds("processing.defaultChannel", candidate.Processing.DefaultChannel); err != nil {
		return err
	}
	for _, override := range candidate.Processing.Overrides {
		if err := validateSettingsString("processing.overrides", override.Name, MaxSettingsChannelNameBytes); err != nil {
			return err
		}
		if err := validateProcessingChannelBounds("processing.overrides", override.Channel); err != nil {
			return err
		}
	}
	if len(candidate.Processing.MutualExclusion) > MaxSettingsMutualExclusionGroups {
		return validation("processing.mutualExclusion", errors.New("too many mutual exclusion groups"))
	}
	for _, group := range candidate.Processing.MutualExclusion {
		if len(group) > MaxSettingsMutualExclusionMembers {
			return validation("processing.mutualExclusion", errors.New("too many mutual exclusion members"))
		}
		for _, name := range group {
			if err := validateSettingsString("processing.mutualExclusion", name, MaxSettingsChannelNameBytes); err != nil {
				return err
			}
		}
	}
	if err := validateSettingsString("osc.targetMode", string(candidate.OSC.TargetMode), MaxSettingsTargetModeBytes); err != nil {
		return err
	}
	if err := validateSettingsString("osc.preferredService", candidate.OSC.PreferredService, MaxSettingsEndpointBytes); err != nil {
		return err
	}
	if err := validateSettingsString("osc.manualHost", candidate.OSC.ManualHost, MaxSettingsEndpointBytes); err != nil {
		return err
	}

	// json.Marshal sees the exact unnormalized Candidate wire shape without
	// cloning its lists. A numeric semantic error is left to Normalize, whose
	// field-specific validation is more useful than an encoding error here.
	settings := Settings{SchemaVersion: SchemaVersion, Revision: 1, Avatar: candidate.Avatar, Plugins: candidate.Plugins, Processing: candidate.Processing, OSC: candidate.OSC}
	if data, err := json.Marshal(settings); err == nil && len(data) > MaxSettingsBytes {
		return validation("settings", errors.New("encoded settings exceed maximum size"))
	}
	return nil
}

func validateProcessingChannelBounds(field string, channel ProcessingChannel) error {
	return validateSettingsString(field, channel.Filter.Mode, MaxSettingsFilterModeBytes)
}

func validateSettingsString(field, value string, maximum int) error {
	if !utf8.ValidString(value) {
		return validation(field, errors.New("must be valid UTF-8"))
	}
	if len(value) > maximum {
		return validation(field, errors.New("exceeds maximum length"))
	}
	return nil
}

func (settings Settings) Clone() Settings {
	clone := settings
	if settings.Plugins.DevRoots != nil {
		clone.Plugins.DevRoots = make([]string, len(settings.Plugins.DevRoots))
		copy(clone.Plugins.DevRoots, settings.Plugins.DevRoots)
	}
	clone.Processing = settings.Processing.Clone()
	return clone
}

func (candidate Candidate) Clone() Candidate {
	clone := candidate
	if candidate.Plugins.DevRoots != nil {
		clone.Plugins.DevRoots = make([]string, len(candidate.Plugins.DevRoots))
		copy(clone.Plugins.DevRoots, candidate.Plugins.DevRoots)
	}
	clone.Processing = candidate.Processing.Clone()
	return clone
}

func (processing Processing) Clone() Processing {
	clone := processing
	if processing.Overrides != nil {
		clone.Overrides = make([]ProcessingOverride, len(processing.Overrides))
		copy(clone.Overrides, processing.Overrides)
	}
	if processing.MutualExclusion != nil {
		clone.MutualExclusion = make([][]string, len(processing.MutualExclusion))
		for i, group := range processing.MutualExclusion {
			if group != nil {
				clone.MutualExclusion[i] = make([]string, len(group))
				copy(clone.MutualExclusion[i], group)
			}
		}
	}
	return clone
}
