package projectstatus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerRejectsUnknownAndUnsafeCommands(t *testing.T) {
	runner := NewRunner(t.TempDir())
	for _, request := range []CommandRequest{
		{ID: "unknown"},
		{ID: "go-test", Args: []string{"./...", "&&", "echo"}},
		{ID: "go-test", Args: []string{"TOKEN=value"}},
		{ID: "git-head", Args: []string{"extra"}},
	} {
		output := runner.Run(context.Background(), request)
		if output.ExitCode != 2 {
			t.Fatalf("request %#v exit = %d, want 2", request, output.ExitCode)
		}
	}
}

func TestRunnerTimesOutAndBoundsOutput(t *testing.T) {
	registry := map[string]commandDefinition{
		"helper": {executable: os.Args[0], prefix: []string{"-test.run=TestProjectStatusHelperProcess", "--"}},
	}
	runner := newRunner(t.TempDir(), registry)
	timedOut := runner.Run(context.Background(), CommandRequest{ID: "helper", Args: []string{"sleep"}, Timeout: 20 * time.Millisecond})
	if !timedOut.TimedOut || timedOut.ExitCode == 0 {
		t.Fatalf("timeout output = %#v", timedOut)
	}
	spam := runner.Run(context.Background(), CommandRequest{ID: "helper", Args: []string{"spam"}, Timeout: 10 * time.Second})
	if spam.ExitCode != 0 || !spam.Truncated || len(spam.Stdout) > maxCapturedOutput {
		t.Fatalf("spam output = exit %d truncated=%v len=%d", spam.ExitCode, spam.Truncated, len(spam.Stdout))
	}
}

func TestEvaluatorChecksFilesSymbolsAndPlaceholders(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "internal", "demo", "demo.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package demo\n\nfunc Ready() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := Spec{
		ID: "demo", Path: "internal/demo", Checks: []CheckSpec{
			{ID: "file", Type: CheckFile, Path: "internal/demo/demo.go", Weight: 1, Required: true},
			{ID: "symbol", Type: CheckSymbol, Path: "internal/demo/demo.go", Pattern: `func Ready\(\)`, Weight: 1, Required: true},
			{ID: "implemented", Type: CheckNotPlaceholder, Path: "internal/demo/demo.go", Patterns: []string{`panic\("unimplemented"\)`}, Weight: 1, Required: true},
		},
	}
	results := NewEvaluator(root, NewRunner(root)).Evaluate(context.Background(), []Spec{spec})
	for _, result := range results[0].Checks {
		if result.State != CheckPassed {
			t.Fatalf("check %#v did not pass", result)
		}
	}
}

func TestEvaluatorRejectsPathEscape(t *testing.T) {
	root := t.TempDir()
	spec := Spec{ID: "escape", Checks: []CheckSpec{{
		ID: "escape", Type: CheckFile, Path: "../outside", Weight: 1, Required: true,
	}}}
	result := NewEvaluator(root, NewRunner(root)).Evaluate(context.Background(), []Spec{spec})[0].Checks[0]
	if result.State != CheckFailed || !strings.Contains(result.Evidence, "escapes repository") {
		t.Fatalf("result = %#v", result)
	}
}

type countingRunner struct{ calls int }

func (runner *countingRunner) Run(_ context.Context, _ CommandRequest) CommandOutput {
	runner.calls++
	return CommandOutput{}
}

func TestEvaluatorCachesIdenticalCommands(t *testing.T) {
	runner := &countingRunner{}
	check := CheckSpec{ID: "test", Type: CheckCommand, Command: "go-test", Args: []string{"./internal/demo"}, Weight: 1, Required: true}
	specs := []Spec{{ID: "one", Checks: []CheckSpec{check}}, {ID: "two", Checks: []CheckSpec{check}}}
	NewEvaluator(t.TempDir(), runner).Evaluate(context.Background(), specs)
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
}

func TestProjectStatusHelperProcess(t *testing.T) {
	separator := -1
	for index, arg := range os.Args {
		if arg == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		return
	}
	switch os.Args[separator+1] {
	case "sleep":
		time.Sleep(time.Second)
	case "spam":
		fmt.Print(strings.Repeat("x", maxCapturedOutput*2))
	}
}
