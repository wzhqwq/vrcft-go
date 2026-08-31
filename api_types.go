package main

import (
	"errors"
	"math"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/userconfig"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const (
	maxPublicPluginIDBytes          = 256
	maxPublicPluginNameBytes        = 256
	maxPublicPluginDescriptionBytes = 4096
	maxPublicPluginVersionBytes     = 256
	maxPublicPluginListEntries      = 1024
)

var publicPluginIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)

const (
	ProblemValidation          = "validation"
	ProblemConflict            = "conflict"
	ProblemNotFound            = "not_found"
	ProblemUnavailable         = "unavailable"
	ProblemUnsupportedPlatform = "unsupported_platform"
	ProblemTimeout             = "timeout"
	ProblemInternal            = "internal"
)

type Problem struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	Field           string `json:"field,omitempty"`
	CurrentRevision uint64 `json:"currentRevision,omitempty"`
}

// RuntimeResponse is the Wails-safe status snapshot for the runtime module.
type RuntimeResponse struct {
	Revision          uint64                 `json:"revision"`
	UpdatedAt         time.Time              `json:"updatedAt"`
	Phase             string                 `json:"phase"`
	PlatformSupported bool                   `json:"platformSupported"`
	Application       *RuntimeApplicationDTO `json:"application,omitempty"`
	Problem           *Problem               `json:"problem,omitempty"`
}

type RuntimeApplicationDTO struct {
	Lifecycle           string                    `json:"lifecycle"`
	AvatarID            string                    `json:"avatarId"`
	PlanGeneration      uint64                    `json:"planGeneration"`
	PlanStatus          string                    `json:"planStatus"`
	PlanSource          string                    `json:"planSource"`
	ConfigPath          string                    `json:"configPath"`
	ConfigID            string                    `json:"configId"`
	GenerationExhausted bool                      `json:"generationExhausted"`
	OSC                 RuntimeOSCDTO             `json:"osc"`
	PluginFailures      []PluginControlFailureDTO `json:"pluginFailures"`
	PlanError           string                    `json:"planError,omitempty"`
	RuntimeError        string                    `json:"runtimeError,omitempty"`
}

type RuntimeOSCDTO struct {
	Running    bool         `json:"running"`
	Connected  bool         `json:"connected"`
	HasTarget  bool         `json:"hasTarget"`
	TargetMode string       `json:"targetMode"`
	Target     OSCTargetDTO `json:"target"`
	LastError  string       `json:"lastError,omitempty"`
}

type OSCTargetDTO struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type PluginControlFailureDTO struct {
	PluginID  string `json:"pluginId"`
	Operation string `json:"operation"`
	Message   string `json:"message"`
}

type PluginListResponse struct {
	Revision  uint64      `json:"revision"`
	UpdatedAt time.Time   `json:"updatedAt"`
	Plugins   []PluginDTO `json:"plugins"`
	Problem   *Problem    `json:"problem,omitempty"`
}

type PluginDTO struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Description         string     `json:"description"`
	Version             string     `json:"version"`
	Capabilities        []string   `json:"capabilities"`
	Enabled             bool       `json:"enabled"`
	Active              bool       `json:"active"`
	State               string     `json:"state"`
	ConfigRevision      uint64     `json:"configRevision"`
	FrameRate           float64    `json:"frameRate"`
	ConsecutiveFailures int        `json:"consecutiveFailures"`
	RestartCount        int        `json:"restartCount"`
	StartedAt           *time.Time `json:"startedAt,omitempty"`
	LastHeartbeatAt     *time.Time `json:"lastHeartbeatAt,omitempty"`
	LastFrameAt         *time.Time `json:"lastFrameAt,omitempty"`
	NextRestartAt       *time.Time `json:"nextRestartAt,omitempty"`
	LastError           string     `json:"lastError,omitempty"`
}

type PluginConfigResponse struct {
	Revision       uint64    `json:"revision"`
	UpdatedAt      time.Time `json:"updatedAt"`
	PluginID       string    `json:"pluginId"`
	ConfigRevision uint64    `json:"configRevision"`
	Data           string    `json:"data"`
	Problem        *Problem  `json:"problem,omitempty"`
}

type PluginMutationResponse struct {
	Revision  uint64    `json:"revision"`
	UpdatedAt time.Time `json:"updatedAt"`
	PluginID  string    `json:"pluginId"`
	Problem   *Problem  `json:"problem,omitempty"`
}

type SettingsResponse struct {
	Revision     uint64               `json:"revision"`
	UpdatedAt    time.Time            `json:"updatedAt"`
	FileRevision uint64               `json:"fileRevision"`
	Settings     userconfig.Candidate `json:"settings"`
	Problem      *Problem             `json:"problem,omitempty"`
}

type SettingsValidationResponse struct {
	Revision  uint64               `json:"revision"`
	UpdatedAt time.Time            `json:"updatedAt"`
	Settings  userconfig.Candidate `json:"settings"`
	Problem   *Problem             `json:"problem,omitempty"`
}

type SettingsSaveResponse struct {
	Revision        uint64               `json:"revision"`
	UpdatedAt       time.Time            `json:"updatedAt"`
	FileRevision    uint64               `json:"fileRevision"`
	Settings        userconfig.Candidate `json:"settings"`
	RestartRequired bool                 `json:"restartRequired"`
	Problem         *Problem             `json:"problem,omitempty"`
}

func pluginDTO(snapshot plugins.RuntimeSnapshot) PluginDTO {
	return PluginDTO{
		ID:                  snapshot.ID,
		Name:                snapshot.Name,
		Description:         snapshot.Description,
		Version:             snapshot.Version,
		Capabilities:        capabilityNames(snapshot.Capabilities),
		Enabled:             snapshot.Enabled,
		Active:              snapshot.Active,
		State:               string(snapshot.State),
		ConfigRevision:      snapshot.ConfigRevision,
		FrameRate:           finiteFrameRate(snapshot.FrameRate),
		ConsecutiveFailures: snapshot.ConsecutiveFailures,
		RestartCount:        snapshot.RestartCount,
		StartedAt:           optionalTime(snapshot.StartedAt),
		LastHeartbeatAt:     optionalTime(snapshot.LastHeartbeatAt),
		LastFrameAt:         optionalTime(snapshot.LastFrameAt),
		NextRestartAt:       optionalTime(snapshot.NextRestartAt),
		LastError:           boundedMessage(snapshot.LastError),
	}
}

func validatePublicPluginID(pluginID string) error {
	if !validPublicText(pluginID, maxPublicPluginIDBytes) || !publicPluginIDPattern.MatchString(pluginID) {
		return &userconfig.ValidationError{Field: "pluginId", Err: errors.New("plugin ID is invalid")}
	}
	return nil
}

func validatePublicPluginSnapshot(snapshot plugins.RuntimeSnapshot) error {
	if err := validatePublicPluginID(snapshot.ID); err != nil {
		return &userconfig.ValidationError{Field: "plugins", Err: err}
	}
	for _, field := range []struct {
		name  string
		value string
		max   int
	}{
		{name: "name", value: snapshot.Name, max: maxPublicPluginNameBytes},
		{name: "description", value: snapshot.Description, max: maxPublicPluginDescriptionBytes},
		{name: "version", value: snapshot.Version, max: maxPublicPluginVersionBytes},
	} {
		if !validPublicText(field.value, field.max) {
			return &userconfig.ValidationError{Field: "plugins", Err: pluginDataValidation("plugin " + field.name + " violates public bounds")}
		}
	}
	return nil
}

func validPublicText(value string, maxBytes int) bool {
	return utf8.ValidString(value) && len(value) <= maxBytes
}

func capabilityNames(capabilities trackingmodel.Capability) []string {
	names := make([]string, 0, 3)
	if capabilities.Has(trackingmodel.CapabilityEye) {
		names = append(names, "eye")
	}
	if capabilities.Has(trackingmodel.CapabilityExpression) {
		names = append(names, "expression")
	}
	if capabilities.Has(trackingmodel.CapabilityLip) {
		names = append(names, "lip")
	}
	return names
}

func finiteFrameRate(frameRate float64) float64 {
	if math.IsNaN(frameRate) || math.IsInf(frameRate, 0) {
		return 0
	}
	return frameRate
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	clone := value
	return &clone
}
