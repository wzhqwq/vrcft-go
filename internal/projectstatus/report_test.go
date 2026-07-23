package projectstatus

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRenderMarkdownIsDeterministic(t *testing.T) {
	status := BuildStatus(StatusInput{
		Results: []SpecResult{
			{Spec: Spec{ID: "z", Path: `internal\z`, Milestone: "M2"}, Checks: []CheckResult{{CheckID: "build", State: CheckFailed, Weight: 1, Required: true, Evidence: "failed"}}},
			{Spec: Spec{ID: "a", Path: "internal/a", Milestone: "M1"}, Checks: []CheckResult{{CheckID: "test", State: CheckPassed, Weight: 1, Required: true}}},
		},
		Commit: "one", SourceFingerprint: "same", Dirty: true,
		GeneratedAt: time.Unix(10, 0).UTC(),
	})
	first, err := RenderMarkdown(status)
	if err != nil {
		t.Fatal(err)
	}
	status.Commit = "two"
	status.Dirty = false
	status.GeneratedAt = time.Unix(20, 0).UTC()
	second, err := RenderMarkdown(status)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(NormalizeMarkdown(first), NormalizeMarkdown(second)) {
		t.Fatalf("normalized reports differ:\n%s\n%s", first, second)
	}
	text := string(first)
	if strings.Index(text, "| M1 ") > strings.Index(text, "| M2 ") || !strings.Contains(text, "internal/z") {
		t.Fatalf("report ordering/path =\n%s", text)
	}
	if strings.Contains(text, "ns") {
		t.Fatalf("committed Markdown contains duration: %s", text)
	}
}

func TestNormalizeMarkdownIgnoresFailedEvidenceClockPrefixes(t *testing.T) {
	first := []byte("- `frontend/type-check` (failed): 02:14:08 tool failed\n")
	second := []byte("- `frontend/type-check` (failed): 02:15:30 tool failed\n")
	if !bytes.Equal(NormalizeMarkdown(first), NormalizeMarkdown(second)) {
		t.Fatalf("normalized failed evidence differs:\n%s\n%s", NormalizeMarkdown(first), NormalizeMarkdown(second))
	}
}

func TestRenderJSONIncludesSchemaAndExactWeights(t *testing.T) {
	status := BuildStatus(StatusInput{Results: []SpecResult{{
		Spec:   Spec{ID: "a", Milestone: "M1", DependsOn: []string{"root"}},
		Checks: []CheckResult{{CheckID: "test", State: CheckPassed, Weight: 3, Required: true, Duration: time.Millisecond}},
	}}, SourceFingerprint: "hash"})
	content, err := RenderJSON(status)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		SchemaVersion int `json:"schemaVersion"`
		PassedWeight  int `json:"passedWeight"`
		TotalWeight   int `json:"totalWeight"`
	}
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != 1 || decoded.PassedWeight != 3 || decoded.TotalWeight != 3 {
		t.Fatalf("json = %s", content)
	}
}
