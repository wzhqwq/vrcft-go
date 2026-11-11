package picoconnect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckTransferSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, tc := range []struct {
		name, contents, want string
	}{
		{"enabled", `{"lab":{"faceTrackingTransferProtocol":2}}`, ""},
		{"wrong protocol", `{"lab":{"faceTrackingTransferProtocol":0}}`, "set it to 2"},
		{"missing field", `{"lab":{}}`, "Set lab.faceTrackingTransferProtocol"},
		{"invalid JSON", `{`, "Cannot parse"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.contents), 0600); err != nil {
				t.Fatal(err)
			}
			got := checkTransferSetting(path)
			if !strings.Contains(got, tc.want) || (tc.want == "" && got != "") {
				t.Fatalf("warning = %q, want %q", got, tc.want)
			}
		})
	}
	if got := checkTransferSetting(filepath.Join(t.TempDir(), "absent.json")); !strings.Contains(got, "Cannot read") {
		t.Fatalf("missing settings warning = %q", got)
	}
}
