package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/application"
)

func TestRuntimeEmptyFailuresMarshalAsArray(t *testing.T) {
	api := newRuntimeAPI(true, time.Now)
	api.setApplicationStatus(application.Status{})
	data, err := json.Marshal(api.GetStatus())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"pluginFailures":[]`) {
		t.Fatalf("empty failure wire: %s", data)
	}
}

func TestDiagnosticLogRotationAndBoundedSnapshot(t *testing.T) {
	dir := t.TempDir()
	logs := newDiagnosticLog()
	logs.open(dir, 512)
	for i := 0; i < 240; i++ {
		logs.write(slog.LevelInfo, "runtime", "starting", strings.Repeat("x", 100))
	}
	logs.close()
	snapshot := logs.snapshot()
	if len(snapshot.Entries) != 200 {
		t.Fatalf("entries = %d", len(snapshot.Entries))
	}
	files, err := filepath.Glob(filepath.Join(dir, "application*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 {
		t.Fatalf("rotation files = %v", files)
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) > 512 {
			t.Fatalf("oversized log %s: %d", path, len(data))
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			var entry DiagnosticEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatal(err)
			}
			if entry.ID == "" {
				t.Fatal("missing ID")
			}
		}
	}
	snapshot.Entries[0].Message = "mutated"
	if logs.snapshot().Entries[0].Message == "mutated" {
		t.Fatal("snapshot aliases store")
	}
}

func TestDiagnosticLogUnwritableFallsBackAndRedacts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(dir, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	logs := newDiagnosticLog()
	logs.open(dir, 1024)
	defer logs.close()
	entry := logs.write(slog.LevelError, "runtime", "backend_start", `listen udp :9001: address already in use; token=super-secret password="two words" C:\Users\Alice\AppData\config.json`)
	if !strings.Contains(entry.Message, "address already in use") || strings.Contains(entry.Message, "super-secret") || strings.Contains(entry.Message, "two words") || strings.Contains(entry.Message, "Alice") {
		t.Fatalf("redaction: %s", entry.Message)
	}
	if logs.snapshot().DiskError == "" || len(logs.snapshot().Entries) == 0 {
		t.Fatal("missing fallback diagnostics")
	}
}

func TestAppDiagnosticsRetainStageErrorAndLogCorrelation(t *testing.T) {
	harness := newRootAppHarness(t)
	harness.backend.startErr = errors.New("listen udp :9001: address already in use")
	app := newAppWithDependencies(harness.dependencies())
	app.startup(context.Background())
	defer app.shutdown(context.Background())
	diagnostic := app.runtime.GetDiagnostics()
	if diagnostic.Failure == nil || diagnostic.Failure.Stage != "backend_start" || !strings.Contains(diagnostic.Failure.Message, "address already in use") {
		t.Fatalf("diagnostic: %+v", diagnostic)
	}
	found := false
	for _, entry := range diagnostic.Entries {
		if entry.ID == diagnostic.Failure.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("failure has no matching log")
	}
	for i := 0; i < 220; i++ {
		app.diagnostics.write(slog.LevelInfo, "test", "test", "entry")
	}
	if app.runtime.GetDiagnostics().Failure.ID != diagnostic.Failure.ID {
		t.Fatal("startup failure was evicted")
	}
}

func TestDiagnosticConcurrentLoggingAndClose(t *testing.T) {
	logs := newDiagnosticLog()
	logs.open(t.TempDir(), 4096)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				logs.logger().With("component", "plugin").Info("heartbeat", "stage", "status")
				_ = logs.snapshot()
			}
		}()
	}
	wg.Wait()
	logs.close()
	logs.close()
	before := logs.snapshot()
	logs.write(slog.LevelInfo, "test", "after_close", "ignored")
	if len(logs.snapshot().Entries) != len(before.Entries) {
		t.Fatal("accepted log after close")
	}
}

func TestFrontendDiagnosticIngressIsBoundedAndRateLimited(t *testing.T) {
	api := newRuntimeAPI(true, time.Now)
	for range 100 {
		api.ReportFrontendError("get_status", strings.Repeat("x", 10000))
	}
	api.ReportFrontendError("arbitrary-stage", "should not be accepted")
	entries := api.GetDiagnostics().Entries
	if len(entries) == 0 || len(entries) > 10 {
		t.Fatalf("frontend count %d", len(entries))
	}
	for _, entry := range entries {
		if len(entry.Message) > 4096 || entry.Stage != "get_status" {
			t.Fatalf("entry %+v", entry)
		}
	}
}

func TestDiagnosticRedactionHandlesHeadersEscapesAndProfileSpaces(t *testing.T) {
	for _, source := range []string{
		`Authorization: Basic dXNlcjpzZWNyZXQ=`,
		`Cookie: session=private; csrf=private2`,
		`{"password":"first\"secret_tail"}`,
		`C:\Users\Jane Doe\AppData\test`,
		`/home/Jane Doe/config.json`,
		`https://Jane:private@example.com`,
	} {
		result := redactDiagnostic(source)
		for _, secret := range []string{"dXNlcjpzZWNyZXQ", "private", "secret_tail", "Jane", "Doe"} {
			if strings.Contains(result, secret) {
				t.Errorf("redaction leaked %q: %s", secret, result)
			}
		}
	}
}

func TestDiagnosticDiskWriteFailureKeepsMemoryRecord(t *testing.T) {
	logs := newDiagnosticLog()
	file, err := os.Create(filepath.Join(t.TempDir(), "closed.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	entry := logs.write(slog.LevelError, "runtime", "backend_start", "address already in use")
	queue := make(chan DiagnosticEntry, 1)
	queue <- entry
	close(queue)
	done := make(chan struct{})
	logs.runWriter(file, 0, diagnosticFileBytes, queue, done)
	snapshot := logs.snapshot()
	if snapshot.DiskError == "" || len(snapshot.Entries) != 1 || snapshot.Entries[0].ID != entry.ID {
		t.Fatalf("write failure lost diagnostic: %+v", snapshot)
	}
	logs.close()
}
