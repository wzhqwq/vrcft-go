package avatar

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wzhqwq/vrcft-go/internal/osc"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "avatar.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadConfigDecodesInputEndpoints(t *testing.T) {
	got, err := readConfig(writeConfig(t, `{
  "id":"avtr_demo",
  "name":"Demo",
  "future":{"accepted":true},
  "parameters":[
    {"name":"JawOpen","input":{"address":"/avatar/parameters/v2/JawOpen","type":"Float"}},
    {"name":"JawX","input":{"address":"/avatar/parameters/v2/JawX","type":"Int"}},
    {"name":"EyeTrackingActive","input":{"address":"/avatar/parameters/v2/EyeTrackingActive","type":"Bool"}},
    {"name":"IgnoredOutput","output":{"address":"/outside","type":"Float"}},
    {"name":"NoInput"}
  ]
}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []osc.Endpoint{
		{Address: "/avatar/parameters/v2/JawOpen", Type: "f"},
		{Address: "/avatar/parameters/v2/JawX", Type: "i"},
		{Address: "/avatar/parameters/v2/EyeTrackingActive", Type: "T"},
	}
	if got.id != "avtr_demo" || !reflect.DeepEqual(got.endpoints, want) {
		t.Fatalf("decoded = %#v", got)
	}
}

func TestReadConfigRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    error
	}{
		{name: "missing ID", content: `{"parameters":[]}`, want: ErrInvalidJSON},
		{name: "null ID", content: `{"id":null,"parameters":[]}`, want: ErrInvalidJSON},
		{name: "wrong ID type", content: `{"id":1,"parameters":[]}`, want: ErrInvalidJSON},
		{name: "missing parameters", content: `{"id":"avtr_demo"}`, want: ErrInvalidJSON},
		{name: "null parameters", content: `{"id":"avtr_demo","parameters":null}`, want: ErrInvalidJSON},
		{name: "wrong parameters type", content: `{"id":"avtr_demo","parameters":{}}`, want: ErrInvalidJSON},
		{name: "wrong known name type", content: `{"id":"avtr_demo","name":false,"parameters":[]}`, want: ErrInvalidJSON},
		{name: "input is array", content: `{"id":"avtr_demo","parameters":[{"input":[]}]}`, want: ErrInvalidJSON},
		{name: "input missing fields", content: `{"id":"avtr_demo","parameters":[{"input":{}}]}`, want: ErrInvalidInputEndpoint},
		{name: "unknown input type", content: `{"id":"avtr_demo","parameters":[{"input":{"address":"/value","type":"String"}}]}`, want: ErrInvalidInputEndpoint},
		{name: "empty input address", content: `{"id":"avtr_demo","parameters":[{"input":{"address":"","type":"Float"}}]}`, want: ErrInvalidInputEndpoint},
		{name: "relative input address", content: `{"id":"avtr_demo","parameters":[{"input":{"address":"value","type":"Float"}}]}`, want: ErrInvalidInputEndpoint},
		{name: "NUL input address", content: `{"id":"avtr_demo","parameters":[{"input":{"address":"/va\u0000lue","type":"Float"}}]}`, want: ErrInvalidInputEndpoint},
		{name: "trailing JSON", content: `{"id":"avtr_demo","parameters":[]} {}`, want: ErrInvalidJSON},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := readConfig(writeConfig(t, test.content))
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want category %v", err, test.want)
			}
		})
	}
}

func TestReadConfigEnforcesBounds(t *testing.T) {
	t.Run("exact avatar ID limit", func(t *testing.T) {
		id := strings.Repeat("a", maxAvatarIDBytes)
		content := fmt.Sprintf(`{"id":%q,"parameters":[]}`, id)
		got, err := readConfig(writeConfig(t, content))
		if err != nil {
			t.Fatal(err)
		}
		if got.id != id {
			t.Fatalf("ID = %q, want %q", got.id, id)
		}
	})

	t.Run("avatar ID exceeds limit", func(t *testing.T) {
		id := strings.Repeat("a", maxAvatarIDBytes+1)
		content := fmt.Sprintf(`{"id":%q,"parameters":[]}`, id)
		_, err := readConfig(writeConfig(t, content))
		if !errors.Is(err, ErrInvalidAvatarID) {
			t.Fatalf("error = %v, want category %v", err, ErrInvalidAvatarID)
		}
	})

	t.Run("too many parameters", func(t *testing.T) {
		content := `{"id":"avtr_demo","parameters":[` + strings.Repeat(`{},`, maxParameters) + `{}` + `]}`
		_, err := readConfig(writeConfig(t, content))
		if !errors.Is(err, ErrTooManyParameters) {
			t.Fatalf("error = %v, want category %v", err, ErrTooManyParameters)
		}
	})

	t.Run("address exceeds limit", func(t *testing.T) {
		address := "/" + strings.Repeat("a", maxOSCAddressBytes)
		content := fmt.Sprintf(`{"id":"avtr_demo","parameters":[{"input":{"address":%q,"type":"Float"}}]}`, address)
		_, err := readConfig(writeConfig(t, content))
		if !errors.Is(err, ErrInvalidInputEndpoint) {
			t.Fatalf("error = %v, want category %v", err, ErrInvalidInputEndpoint)
		}
	})

	t.Run("file exceeds limit", func(t *testing.T) {
		content := `{"id":"avtr_demo","parameters":[]}` + strings.Repeat(" ", maxConfigBytes)
		_, err := readConfig(writeConfig(t, content))
		if !errors.Is(err, ErrConfigTooLarge) {
			t.Fatalf("error = %v, want category %v", err, ErrConfigTooLarge)
		}
	})

	t.Run("exact file limit", func(t *testing.T) {
		content := `{"id":"avtr_demo","parameters":[]}`
		content += strings.Repeat(" ", maxConfigBytes-len(content))
		if _, err := readConfig(writeConfig(t, content)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("exact parameter limit", func(t *testing.T) {
		content := `{"id":"avtr_demo","parameters":[` + strings.Repeat(`{},`, maxParameters-1) + `{}` + `]}`
		got, err := readConfig(writeConfig(t, content))
		if err != nil {
			t.Fatal(err)
		}
		if len(got.endpoints) != 0 {
			t.Fatalf("endpoint count = %d, want 0", len(got.endpoints))
		}
	})

	t.Run("exact address limit", func(t *testing.T) {
		address := "/" + strings.Repeat("a", maxOSCAddressBytes-1)
		content := fmt.Sprintf(`{"id":"avtr_demo","parameters":[{"input":{"address":%q,"type":"Float"}}]}`, address)
		got, err := readConfig(writeConfig(t, content))
		if err != nil {
			t.Fatal(err)
		}
		if len(got.endpoints) != 1 {
			t.Fatalf("endpoint count = %d, want 1", len(got.endpoints))
		}
	})
}
