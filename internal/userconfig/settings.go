// Package userconfig owns the versioned, user-editable M7 configuration document.
package userconfig

import "github.com/wzhqwq/vrcft-go/internal/osc"

const (
	SchemaVersion        = 1
	MaxSettingsBytes     = 256 << 10
	MaxPluginConfigBytes = 64 << 10
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

func (settings Settings) Clone() Settings {
	clone := settings
	clone.Plugins.DevRoots = append([]string(nil), settings.Plugins.DevRoots...)
	clone.Processing = settings.Processing.Clone()
	return clone
}

func (candidate Candidate) Clone() Candidate {
	clone := candidate
	clone.Plugins.DevRoots = append([]string(nil), candidate.Plugins.DevRoots...)
	clone.Processing = candidate.Processing.Clone()
	return clone
}

func (processing Processing) Clone() Processing {
	clone := processing
	clone.Overrides = append([]ProcessingOverride(nil), processing.Overrides...)
	clone.MutualExclusion = make([][]string, len(processing.MutualExclusion))
	for i, group := range processing.MutualExclusion {
		clone.MutualExclusion[i] = append([]string(nil), group...)
	}
	return clone
}
