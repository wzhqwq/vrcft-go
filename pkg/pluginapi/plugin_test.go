package pluginapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

func validDescriptor() Descriptor {
	return Descriptor{
		APIVersion:   APIVersion,
		ID:           "acme.eye-tracker",
		Name:         "Acme Eye Tracker",
		Version:      "1.2.3",
		Description:  "Publishes eye tracking data.",
		Capabilities: trackingmodel.CapabilityEye,
	}
}

func TestDescriptorValidate(t *testing.T) {
	if err := validDescriptor().Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestDescriptorValidateAcceptsLipOnly(t *testing.T) {
	descriptor := validDescriptor()
	descriptor.Capabilities = trackingmodel.CapabilityLip
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("Validate(Lip-only descriptor) error = %v, want nil", err)
	}
}

func TestDescriptorValidateAcceptsSemVerTwoPrereleaseAndBuildSyntax(t *testing.T) {
	for _, version := range []string{
		"1.0.0-alpha",
		"1.0.0-alpha.1",
		"1.0.0-0.3.7",
		"1.0.0-x.7.z.92",
		"1.0.0-x-y-z.--",
		"1.0.0+20130313144700",
		"1.0.0-beta+exp.sha.5114f85",
		"0.0.0-rc.1+build.001",
	} {
		t.Run(version, func(t *testing.T) {
			descriptor := validDescriptor()
			descriptor.Version = version
			if err := descriptor.Validate(); err != nil {
				t.Fatalf("Validate(%q) error = %v", version, err)
			}
		})
	}
}

func TestDescriptorValidateRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Descriptor)
		field  string
	}{
		{"api version is too old", func(d *Descriptor) { d.APIVersion = 0 }, "APIVersion"},
		{"api version is too new", func(d *Descriptor) { d.APIVersion = APIVersion + 1 }, "APIVersion"},
		{"id is blank", func(d *Descriptor) { d.ID = "" }, "ID"},
		{"id contains uppercase", func(d *Descriptor) { d.ID = "Acme.tracker" }, "ID"},
		{"id starts with separator", func(d *Descriptor) { d.ID = ".acme" }, "ID"},
		{"id ends with separator", func(d *Descriptor) { d.ID = "acme-" }, "ID"},
		{"id has repeated separators", func(d *Descriptor) { d.ID = "acme..tracker" }, "ID"},
		{"name is blank", func(d *Descriptor) { d.Name = " \t" }, "Name"},
		{"version misses component", func(d *Descriptor) { d.Version = "1.2" }, "Version"},
		{"version has extra component", func(d *Descriptor) { d.Version = "1.2.3.4" }, "Version"},
		{"version has leading zero", func(d *Descriptor) { d.Version = "01.2.3" }, "Version"},
		{"version has sign", func(d *Descriptor) { d.Version = "+1.2.3" }, "Version"},
		{"version has negative component", func(d *Descriptor) { d.Version = "1.-2.3" }, "Version"},
		{"version has empty prerelease", func(d *Descriptor) { d.Version = "1.2.3-" }, "Version"},
		{"version has empty prerelease identifier", func(d *Descriptor) { d.Version = "1.2.3-alpha..1" }, "Version"},
		{"version has prerelease numeric leading zero", func(d *Descriptor) { d.Version = "1.2.3-01" }, "Version"},
		{"version has empty build", func(d *Descriptor) { d.Version = "1.2.3+" }, "Version"},
		{"version has empty build identifier", func(d *Descriptor) { d.Version = "1.2.3+build..1" }, "Version"},
		{"version has invalid identifier character", func(d *Descriptor) { d.Version = "1.2.3-alpha_1" }, "Version"},
		{"version has duplicate build separator", func(d *Descriptor) { d.Version = "1.2.3+one+two" }, "Version"},
		{"capabilities are empty", func(d *Descriptor) { d.Capabilities = 0 }, "Capabilities"},
		{"capabilities are unknown", func(d *Descriptor) { d.Capabilities = 1 << 10 }, "Capabilities"},
		{"capabilities include unknown", func(d *Descriptor) { d.Capabilities |= 1 << 10 }, "Capabilities"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validDescriptor()
			tt.mutate(&d)
			err := d.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("Validate() error = %q, want field %q", err, tt.field)
			}
		})
	}
}

func TestConfigClonePreservesNilAndDeepCopiesData(t *testing.T) {
	nilConfig := Config{Revision: 4}
	if clone := nilConfig.Clone(); clone.Data != nil {
		t.Fatalf("Clone().Data = %v, want nil", clone.Data)
	}

	emptyConfig := Config{Revision: 5, Data: json.RawMessage{}}
	emptyClone := emptyConfig.Clone()
	if emptyClone.Data != nil {
		t.Fatalf("Clone().Data = %#v, want canonical nil", emptyClone.Data)
	}

	original := Config{Revision: 6, Data: json.RawMessage(`{"enabled":true}`)}
	clone := original.Clone()
	clone.Data[2] = 'x'
	if string(original.Data) != `{"enabled":true}` {
		t.Fatalf("mutating clone changed original: %q", original.Data)
	}
	if clone.Revision != original.Revision {
		t.Fatalf("Clone().Revision = %d, want %d", clone.Revision, original.Revision)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name  string
		value Config
		want  bool
	}{
		{"nil initial config", Config{}, true},
		{"empty initial config", Config{Data: json.RawMessage{}}, true},
		{"empty later config", Config{Revision: 1}, true},
		{"object config", Config{Revision: 1, Data: json.RawMessage(`{"enabled":true}`)}, true},
		{"null config", Config{Revision: 1, Data: json.RawMessage(`null`)}, true},
		{"nonempty revision zero", Config{Data: json.RawMessage(`{}`)}, false},
		{"malformed JSON", Config{Revision: 1, Data: json.RawMessage(`{`)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.value.Validate()
			if (err == nil) != tt.want {
				t.Fatalf("Validate() error = %v, want valid=%t", err, tt.want)
			}
		})
	}
}

func TestDeviceStatusValidate(t *testing.T) {
	for _, state := range []DeviceState{DeviceInitializing, DeviceReady, DeviceDisconnected} {
		if err := (DeviceStatus{State: state}).Validate(); err != nil {
			t.Fatalf("Validate(%q) error = %v, want nil", state, err)
		}
	}
	if err := (DeviceStatus{State: DeviceError, Message: "camera unavailable"}).Validate(); err != nil {
		t.Fatalf("Validate(error status) error = %v, want nil", err)
	}

	for _, status := range []DeviceStatus{
		{},
		{State: DeviceState("unknown")},
		{State: DeviceError},
		{State: DeviceError, Message: " \n"},
	} {
		if err := status.Validate(); err == nil {
			t.Fatalf("Validate(%+v) error = nil, want error", status)
		}
	}
}

func TestLogValidation(t *testing.T) {
	for _, level := range []LogLevel{LogDebug, LogInfo, LogWarn, LogError} {
		if err := level.Validate(); err != nil {
			t.Fatalf("Validate(%q) error = %v, want nil", level, err)
		}
	}
	for _, level := range []LogLevel{"", "trace"} {
		if err := level.Validate(); err == nil {
			t.Fatalf("Validate(%q) error = nil, want error", level)
		}
	}

	if err := (LogEntry{Level: LogInfo, Message: "connected"}).Validate(); err != nil {
		t.Fatalf("LogEntry.Validate() error = %v, want nil", err)
	}
	for _, entry := range []LogEntry{
		{Level: LogLevel("trace"), Message: "connected"},
		{Level: LogInfo},
		{Level: LogInfo, Message: " \t"},
	} {
		if err := entry.Validate(); err == nil {
			t.Fatalf("LogEntry.Validate(%+v) error = nil, want error", entry)
		}
	}
}

func TestControlEventsAndStartupUseTypedPublicValues(t *testing.T) {
	subscription := Subscription{
		Generation:   9,
		Capabilities: trackingmodel.CapabilityEye | trackingmodel.CapabilityExpression,
		Eye:          trackingmodel.EyeValidLeftGaze,
		Expressions:  trackingmodel.ExpressionMaskOf(0),
	}
	events := []ControlEvent{
		ActiveChanged{Active: true},
		ConfigChanged{Config: Config{Revision: 2, Data: json.RawMessage(`null`)}},
		SubscriptionChanged{Subscription: subscription},
		ShutdownRequested{},
	}
	if len(events) != 4 {
		t.Fatalf("typed events = %d, want 4", len(events))
	}

	startup := Startup{Active: true, Config: Config{Revision: 3}, Subscription: subscription}
	if !startup.Active || startup.Config.Revision != 3 || startup.Subscription.Generation != 9 {
		t.Fatalf("Startup = %+v, want public active/config/subscription values", startup)
	}
}
