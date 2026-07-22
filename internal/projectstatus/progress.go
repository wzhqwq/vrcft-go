package projectstatus

import (
	"path/filepath"
	"sort"
	"time"
)

type State string

const (
	StateNotStarted State = "not_started"
	StateInProgress State = "in_progress"
	StateComplete   State = "complete"
	StateDegraded   State = "degraded"
	StateBlocked    State = "blocked"
)

type SpecStatus struct {
	ID           string        `json:"id"`
	Kind         SpecKind      `json:"kind"`
	Path         string        `json:"path"`
	Milestone    string        `json:"milestone"`
	DependsOn    []string      `json:"dependsOn,omitempty"`
	State        State         `json:"state"`
	PassedWeight int           `json:"passedWeight"`
	TotalWeight  int           `json:"totalWeight"`
	Progress     float64       `json:"progress"`
	Checks       []CheckResult `json:"checks"`
}

type MilestoneStatus struct {
	ID           string  `json:"id"`
	State        State   `json:"state"`
	PassedWeight int     `json:"passedWeight"`
	TotalWeight  int     `json:"totalWeight"`
	Progress     float64 `json:"progress"`
}

type Status struct {
	SchemaVersion     int               `json:"schemaVersion"`
	GeneratedAt       time.Time         `json:"generatedAt"`
	Commit            string            `json:"commit"`
	SourceFingerprint string            `json:"sourceFingerprint"`
	Dirty             bool              `json:"dirty"`
	State             State             `json:"state"`
	PassedWeight      int               `json:"passedWeight"`
	TotalWeight       int               `json:"totalWeight"`
	Progress          float64           `json:"progress"`
	Milestones        []MilestoneStatus `json:"milestones"`
	Specs             []SpecStatus      `json:"specs"`
	FailedRequired    []CheckResult     `json:"failedRequired,omitempty"`
	NextAction        *CheckResult      `json:"nextAction,omitempty"`
}

type StatusInput struct {
	Results           []SpecResult
	Previous          map[string]State
	GeneratedAt       time.Time
	Commit            string
	SourceFingerprint string
	Dirty             bool
}

func CalculateSpecStatus(previous State, result SpecResult) SpecStatus {
	status := SpecStatus{
		ID: result.Spec.ID, Kind: result.Spec.Kind, Path: filepath.ToSlash(result.Spec.Path),
		Milestone: result.Spec.Milestone, DependsOn: append([]string(nil), result.Spec.DependsOn...),
		Checks: append([]CheckResult(nil), result.Checks...),
	}
	requiredPassed := true
	blocked := false
	for _, check := range result.Checks {
		status.TotalWeight += check.Weight
		if check.State == CheckPassed {
			status.PassedWeight += check.Weight
		}
		if check.Required && check.State != CheckPassed {
			requiredPassed = false
		}
		if check.Required && check.State == CheckBlocked {
			blocked = true
		}
	}
	for _, blocker := range result.Spec.Blockers {
		for _, check := range result.Checks {
			if check.CheckID == blocker.Check && check.State != CheckPassed {
				blocked = true
			}
		}
	}
	status.Progress = percentage(status.PassedWeight, status.TotalWeight)
	switch {
	case blocked:
		status.State = StateBlocked
	case requiredPassed:
		status.State = StateComplete
	case previous == StateComplete:
		status.State = StateDegraded
	case status.PassedWeight > 0:
		status.State = StateInProgress
	default:
		status.State = StateNotStarted
	}
	return status
}

func BuildStatus(input StatusInput) Status {
	status := Status{
		SchemaVersion: 1, GeneratedAt: input.GeneratedAt, Commit: input.Commit,
		SourceFingerprint: input.SourceFingerprint, Dirty: input.Dirty,
	}
	if status.GeneratedAt.IsZero() {
		status.GeneratedAt = time.Now().UTC()
	}
	for _, result := range input.Results {
		specStatus := CalculateSpecStatus(input.Previous[result.Spec.ID], result)
		status.Specs = append(status.Specs, specStatus)
		status.PassedWeight += specStatus.PassedWeight
		status.TotalWeight += specStatus.TotalWeight
		for _, check := range specStatus.Checks {
			if check.Required && check.State != CheckPassed {
				status.FailedRequired = append(status.FailedRequired, check)
			}
		}
	}
	sort.Slice(status.Specs, func(i, j int) bool {
		if status.Specs[i].Milestone != status.Specs[j].Milestone {
			return status.Specs[i].Milestone < status.Specs[j].Milestone
		}
		return status.Specs[i].ID < status.Specs[j].ID
	})
	sort.Slice(status.FailedRequired, func(i, j int) bool {
		if status.FailedRequired[i].SpecID != status.FailedRequired[j].SpecID {
			return status.FailedRequired[i].SpecID < status.FailedRequired[j].SpecID
		}
		return status.FailedRequired[i].CheckID < status.FailedRequired[j].CheckID
	})
	if len(status.FailedRequired) > 0 {
		next := status.FailedRequired[0]
		status.NextAction = &next
	}
	status.Milestones = aggregateMilestones(status.Specs)
	status.Progress = percentage(status.PassedWeight, status.TotalWeight)
	status.State = aggregateState(status.Specs, status.PassedWeight)
	return status
}

func aggregateMilestones(specs []SpecStatus) []MilestoneStatus {
	byID := make(map[string]*MilestoneStatus)
	states := make(map[string][]State)
	for _, spec := range specs {
		milestone := byID[spec.Milestone]
		if milestone == nil {
			milestone = &MilestoneStatus{ID: spec.Milestone}
			byID[spec.Milestone] = milestone
		}
		milestone.PassedWeight += spec.PassedWeight
		milestone.TotalWeight += spec.TotalWeight
		states[spec.Milestone] = append(states[spec.Milestone], spec.State)
	}
	result := make([]MilestoneStatus, 0, len(byID))
	for id, milestone := range byID {
		milestone.Progress = percentage(milestone.PassedWeight, milestone.TotalWeight)
		milestone.State = aggregateStates(states[id], milestone.PassedWeight)
		result = append(result, *milestone)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func aggregateState(specs []SpecStatus, passed int) State {
	states := make([]State, len(specs))
	for index := range specs {
		states[index] = specs[index].State
	}
	return aggregateStates(states, passed)
}

func aggregateStates(states []State, passed int) State {
	allComplete := len(states) > 0
	for _, state := range states {
		if state == StateBlocked {
			return StateBlocked
		}
		if state == StateDegraded {
			return StateDegraded
		}
		if state != StateComplete {
			allComplete = false
		}
	}
	if allComplete {
		return StateComplete
	}
	if passed > 0 {
		return StateInProgress
	}
	return StateNotStarted
}

func percentage(passed, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(passed) * 100 / float64(total)
}
