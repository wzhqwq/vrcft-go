package tracking

import (
	"errors"
	"reflect"
	"testing"
)

func TestRoutingConfigContainsOnlySupportedGroups(t *testing.T) {
	// Mutation target: reintroducing a Head routing group.
	if _, found := reflect.TypeOf(RoutingConfig{}).FieldByName("Head"); found {
		t.Fatal("RoutingConfig must not expose a Head routing group")
	}
}

func TestRoutingConfigExposesLipWithStableJSONName(t *testing.T) {
	// Mutation target: omitting Lip routing or changing its wire name.
	field, found := reflect.TypeOf(RoutingConfig{}).FieldByName("Lip")
	if !found {
		t.Fatal("RoutingConfig must expose a Lip routing group")
	}
	if got := field.Tag.Get("json"); got != "lip" {
		t.Fatalf("RoutingConfig.Lip json tag = %q, want %q", got, "lip")
	}
}

func TestSourceSelectionValidation(t *testing.T) {
	tests := []struct {
		name      string
		selection SourceSelection
		wantErr   bool
	}{
		{
			name:      "auto without plugin is valid",
			selection: SourceSelection{Auto: true},
		},
		{
			name:      "auto with plugin is invalid",
			selection: SourceSelection{Auto: true, PluginID: "osc"},
			wantErr:   true,
		},
		{
			name:      "manual with plugin is valid",
			selection: SourceSelection{PluginID: "osc"},
		},
		{
			name:    "manual without plugin is invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mutation target: accepting or rejecting the wrong Auto/PluginID combination.
			err := tt.selection.validate()
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Fatalf("validate() error = %v, want error = %t", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrInvalidRouting) {
				t.Fatalf("validate() error = %v, want errors.Is(err, ErrInvalidRouting)", err)
			}
		})
	}
}

func TestDefaultRoutingSelectsAutoForAllGroups(t *testing.T) {
	// Mutation target: returning a manual selection or a non-empty plugin ID by default.
	got := defaultRouting()
	if got.Eye != (SourceSelection{Auto: true}) {
		t.Fatalf("defaultRouting().Eye = %#v, want automatic selection without plugin", got.Eye)
	}
	if got.Expression != (SourceSelection{Auto: true}) {
		t.Fatalf("defaultRouting().Expression = %#v, want automatic selection without plugin", got.Expression)
	}
	if got.Lip != (SourceSelection{Auto: true}) {
		t.Fatalf("defaultRouting().Lip = %#v, want automatic selection without plugin", got.Lip)
	}
}

func TestRoutingConfigValidationRejectsEachInvalidGroup(t *testing.T) {
	tests := []struct {
		name    string
		routing RoutingConfig
	}{
		{
			name: "invalid eye",
			routing: RoutingConfig{
				Eye:        SourceSelection{Auto: true, PluginID: "osc"},
				Expression: SourceSelection{Auto: true},
				Lip:        SourceSelection{Auto: true},
			},
		},
		{
			name: "invalid expression",
			routing: RoutingConfig{
				Eye:        SourceSelection{Auto: true},
				Expression: SourceSelection{Auto: true, PluginID: "osc"},
				Lip:        SourceSelection{Auto: true},
			},
		},
		{
			name: "invalid lip",
			routing: RoutingConfig{
				Eye:        SourceSelection{Auto: true},
				Expression: SourceSelection{Auto: true},
				Lip:        SourceSelection{Auto: true, PluginID: "osc"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mutation target: validating only a subset of Eye, Expression, and Lip.
			err := tt.routing.validate()
			if !errors.Is(err, ErrInvalidRouting) {
				t.Fatalf("validate() error = %v, want errors.Is(err, ErrInvalidRouting)", err)
			}
		})
	}
}
