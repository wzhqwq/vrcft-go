package projectstatus

import (
	"context"
	"time"
)

type SpecKind string

const (
	KindGoPackage     SpecKind = "go-package"
	KindFrontend      SpecKind = "frontend"
	KindParameterSpec SpecKind = "parameter-spec"
	KindBuildRelease  SpecKind = "build-release"
	KindEndToEnd      SpecKind = "end-to-end"
)

type CheckType string

const (
	CheckCommand         CheckType = "command"
	CheckFile            CheckType = "file"
	CheckSymbol          CheckType = "symbol"
	CheckNotPlaceholder  CheckType = "not_placeholder"
	CheckGeneratedClean  CheckType = "generated_clean"
	CheckDependsComplete CheckType = "depends_complete"
	CheckAggregate       CheckType = "aggregate"
)

type CheckSpec struct {
	ID             string    `yaml:"id"`
	Description    string    `yaml:"description"`
	Type           CheckType `yaml:"type"`
	Command        string    `yaml:"command,omitempty"`
	Args           []string  `yaml:"args,omitempty"`
	Path           string    `yaml:"path,omitempty"`
	Pattern        string    `yaml:"pattern,omitempty"`
	Patterns       []string  `yaml:"patterns,omitempty"`
	Members        []string  `yaml:"members,omitempty"`
	Weight         int       `yaml:"weight"`
	Required       bool      `yaml:"required"`
	TimeoutSeconds int       `yaml:"timeout_seconds,omitempty"`
}

type BlockerSpec struct {
	Check  string   `yaml:"check"`
	Blocks []string `yaml:"blocks"`
}

type Spec struct {
	SourcePath string        `yaml:"-"`
	ID         string        `yaml:"id"`
	Kind       SpecKind      `yaml:"kind"`
	Path       string        `yaml:"path"`
	Milestone  string        `yaml:"milestone"`
	Planned    bool          `yaml:"planned,omitempty"`
	DependsOn  []string      `yaml:"depends_on,omitempty"`
	Checks     []CheckSpec   `yaml:"checks"`
	Blockers   []BlockerSpec `yaml:"blockers,omitempty"`
	Body       string        `yaml:"-"`
}

type CommandRequest struct {
	ID      string
	Args    []string
	Timeout time.Duration
	Dir     string
}

type CommandOutput struct {
	ExitCode  int
	Stdout    string
	Stderr    string
	Duration  time.Duration
	TimedOut  bool
	Truncated bool
}

type CommandRunner interface {
	Run(ctx context.Context, request CommandRequest) CommandOutput
}
