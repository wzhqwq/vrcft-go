package specparser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidDocument(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "spec", "vrcft_osc_parameters.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	doc, err := Load(data)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := len(doc.DetailedParameters), 88; got != want {
		t.Fatalf("detailed count = %d, want %d", got, want)
	}
	if got, want := len(doc.SimplifiedParameters), 36; got != want {
		t.Fatalf("simplified count = %d, want %d", got, want)
	}
	if got, want := len(doc.TrackingActiveParameters), 3; got != want {
		t.Fatalf("status count = %d, want %d", got, want)
	}
	if got, want := len(doc.All()), 127; got != want {
		t.Fatalf("total count = %d, want %d", got, want)
	}
}

func TestLoadRejectsDuplicateOSCName(t *testing.T) {
	input := []byte(`
schema_version: 1
counts:
  detailed_float_parameters: 2
  simplified_float_parameters: 0
  float_parameters_total: 2
  status_bool_parameters: 0
  all_parameters_total: 2
detailed_parameters:
  - name: A
    osc_name: v2/A
    group: test
    value_type: float
    supported_encodings: [float]
    range: {min: 0, max: 1}
    semantics:
      - range: [0, 1]
        meaning: a
  - name: B
    osc_name: v2/A
    group: test
    value_type: float
    supported_encodings: [float]
    range: {min: 0, max: 1}
    semantics:
      - range: [0, 1]
        meaning: b
simplified_parameters: []
tracking_active_parameters: []
`)
	if _, err := Load(input); err == nil {
		t.Fatal("Load() succeeded for duplicate osc_name")
	}
}

func TestLoadParsesBoolSemantics(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "spec", "vrcft_osc_parameters.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := doc.TrackingActiveParameters[0].ParseBoolSemantics()
	if err != nil {
		t.Fatal(err)
	}
	if got.True == "" || got.False == "" {
		t.Fatalf("bool semantics not parsed: %+v", got)
	}
}
