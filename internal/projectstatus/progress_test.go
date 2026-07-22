package projectstatus

import (
	"testing"
	"time"
)

func TestCalculateSpecStatus(t *testing.T) {
	tests := []struct {
		name     string
		previous State
		result   SpecResult
		want     State
		passed   int
		total    int
	}{
		{name: "not started", result: resultWith(CheckFailed, CheckFailed), want: StateNotStarted, passed: 0, total: 4},
		{name: "in progress", result: resultWith(CheckPassed, CheckFailed), want: StateInProgress, passed: 1, total: 4},
		{name: "complete", result: resultWith(CheckPassed, CheckPassed), want: StateComplete, passed: 4, total: 4},
		{name: "degraded", previous: StateComplete, result: resultWith(CheckPassed, CheckFailed), want: StateDegraded, passed: 1, total: 4},
		{name: "blocked wins", previous: StateComplete, result: resultWith(CheckPassed, CheckBlocked), want: StateBlocked, passed: 1, total: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := CalculateSpecStatus(test.previous, test.result)
			if got.State != test.want || got.PassedWeight != test.passed || got.TotalWeight != test.total {
				t.Fatalf("status = %#v", got)
			}
		})
	}
}

func TestOptionalFailureDoesNotPreventComplete(t *testing.T) {
	result := SpecResult{Spec: Spec{ID: "optional"}, Checks: []CheckResult{
		{CheckID: "required", State: CheckPassed, Weight: 3, Required: true},
		{CheckID: "optional", State: CheckFailed, Weight: 1, Required: false},
	}}
	got := CalculateSpecStatus("", result)
	if got.State != StateComplete || got.PassedWeight != 3 || got.TotalWeight != 4 {
		t.Fatalf("status = %#v", got)
	}
}

func TestBuildStatusAggregatesWeights(t *testing.T) {
	results := []SpecResult{
		{Spec: Spec{ID: "b", Milestone: "M2"}, Checks: []CheckResult{{CheckID: "x", State: CheckFailed, Weight: 2, Required: true}}},
		{Spec: Spec{ID: "a", Milestone: "M1"}, Checks: []CheckResult{{CheckID: "x", State: CheckPassed, Weight: 3, Required: true}}},
	}
	status := BuildStatus(StatusInput{
		Results: results, Commit: "abc", SourceFingerprint: "fingerprint",
		GeneratedAt: time.Unix(1, 0).UTC(),
	})
	if status.PassedWeight != 3 || status.TotalWeight != 5 || status.Progress != 60 {
		t.Fatalf("project weights = %#v", status)
	}
	if len(status.Milestones) != 2 || status.Milestones[0].ID != "M1" || status.Specs[0].ID != "a" {
		t.Fatalf("status is not sorted: %#v", status)
	}
}

func resultWith(first, second CheckState) SpecResult {
	return SpecResult{Spec: Spec{ID: "test"}, Checks: []CheckResult{
		{CheckID: "one", State: first, Weight: 1, Required: true},
		{CheckID: "two", State: second, Weight: 3, Required: true},
	}}
}
