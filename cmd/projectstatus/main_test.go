package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/projectstatus"
)

func TestRunDefaultAndJSON(t *testing.T) {
	root := t.TempDir()
	status := sampleStatus(projectstatus.StateComplete)
	evaluate := func(context.Context, string) (projectstatus.Status, error) { return status, nil }
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), nil, &stdout, &stderr, root, evaluate); code != 0 {
		t.Fatalf("default exit = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "# Project Status") {
		t.Fatalf("default output = %s", stdout.String())
	}
	stdout.Reset()
	if code := run(context.Background(), []string{"-format", "json"}, &stdout, &stderr, root, evaluate); code != 0 {
		t.Fatalf("json exit = %d", code)
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestRunWritesAndChecksStatus(t *testing.T) {
	root := t.TempDir()
	status := sampleStatus(projectstatus.StateComplete)
	evaluate := func(context.Context, string) (projectstatus.Status, error) { return status, nil }
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-write"}, &stdout, &stderr, root, evaluate); code != 0 {
		t.Fatalf("write exit = %d, stderr=%s", code, stderr.String())
	}
	path := filepath.Join(root, "docs", "project", "status.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if code := run(context.Background(), []string{"-check"}, &stdout, &stderr, root, evaluate); code != 0 {
		t.Fatalf("check exit = %d, stderr=%s", code, stderr.String())
	}
	if err := os.WriteFile(path, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := run(context.Background(), []string{"-check"}, &stdout, &stderr, root, evaluate); code != 1 {
		t.Fatalf("stale exit = %d", code)
	}
	if !strings.Contains(stderr.String(), "go run ./cmd/projectstatus -write") {
		t.Fatalf("stale guidance = %s", stderr.String())
	}
}

func TestRunExitCodes(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name   string
		args   []string
		status projectstatus.State
		err    error
		want   int
	}{
		{name: "incomplete", status: projectstatus.StateInProgress, want: 1},
		{name: "invalid flags", args: []string{"-write", "-check"}, status: projectstatus.StateComplete, want: 2},
		{name: "unknown format", args: []string{"-format", "xml"}, status: projectstatus.StateComplete, want: 2},
		{name: "tool failure", status: projectstatus.StateComplete, err: errors.New("invalid catalog"), want: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evaluate := func(context.Context, string) (projectstatus.Status, error) {
				return sampleStatus(test.status), test.err
			}
			var stdout, stderr bytes.Buffer
			if got := run(context.Background(), test.args, &stdout, &stderr, root, evaluate); got != test.want {
				t.Fatalf("exit = %d, want %d, stderr=%s", got, test.want, stderr.String())
			}
		})
	}
}

func sampleStatus(state projectstatus.State) projectstatus.Status {
	return projectstatus.Status{
		SchemaVersion: 1, GeneratedAt: time.Unix(1, 0).UTC(), State: state,
		SourceFingerprint: "fingerprint", TotalWeight: 1,
	}
}
