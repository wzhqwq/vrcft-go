package plugins

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const helperProcessArgument = "vrcft-plugin-process-helper"

type helperProcessRecord struct {
	Args        []string `json:"args"`
	Environment []string `json:"environment"`
	WorkingDir  string   `json:"working_dir"`
}

func TestSessionCredentialsGenerateUniqueValidValues(t *testing.T) {
	pipeNames := make(map[string]struct{}, 1000)
	tokens := make(map[string]struct{}, 1000)
	for range 1000 {
		pipeName, token, err := newSessionCredentials()
		if err != nil {
			t.Fatalf("newSessionCredentials() error = %v", err)
		}
		if !validTestPipeName(pipeName) {
			t.Fatalf("newSessionCredentials() pipe name %q is not a valid logical pipe name", pipeName)
		}
		decoded, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil {
			t.Fatalf("newSessionCredentials() token is not base64: %v", err)
		}
		if len(decoded) != 32 {
			t.Fatalf("newSessionCredentials() decoded token length = %d, want 32", len(decoded))
		}
		if _, exists := pipeNames[pipeName]; exists {
			t.Fatal("newSessionCredentials() repeated a pipe name")
		}
		if _, exists := tokens[token]; exists {
			t.Fatal("newSessionCredentials() repeated a token")
		}
		pipeNames[pipeName] = struct{}{}
		tokens[token] = struct{}{}
	}
}

func TestLaunchEnvironmentReplacesCredentialKeysCaseInsensitively(t *testing.T) {
	got, err := launchEnvironment([]string{
		"UNRELATED=kept",
		"VRCFT_PIPE_NAME=old-pipe",
		"vrcft_pipe_name=older-pipe",
		"VRCFT_SESSION_TOKEN=old-token",
		"another=value",
	}, "new-pipe", "new-token")
	if err != nil {
		t.Fatalf("launchEnvironment() error = %v", err)
	}
	values := environmentValues(t, got)
	if got := values["UNRELATED"]; got != "kept" {
		t.Fatalf("unrelated environment value = %q, want kept", got)
	}
	if got := values["another"]; got != "value" {
		t.Fatalf("unrelated environment value = %q, want value", got)
	}
	if got := values["VRCFT_PIPE_NAME"]; got != "new-pipe" {
		t.Fatalf("pipe name = %q, want new-pipe", got)
	}
	if got := values["VRCFT_SESSION_TOKEN"]; got != "new-token" {
		t.Fatalf("session token = %q, want new-token", got)
	}
	if countEnvironmentKey(got, "VRCFT_PIPE_NAME") != 1 {
		t.Fatalf("pipe name key count = %d, want 1", countEnvironmentKey(got, "VRCFT_PIPE_NAME"))
	}
	if countEnvironmentKey(got, "VRCFT_SESSION_TOKEN") != 1 {
		t.Fatalf("session token key count = %d, want 1", countEnvironmentKey(got, "VRCFT_SESSION_TOKEN"))
	}
}

func TestLaunchEnvironmentRejectsInvalidCredentialsAndNUL(t *testing.T) {
	tests := []struct {
		name     string
		base     []string
		pipeName string
		token    string
		secret   string
	}{
		{name: "blank pipe name", pipeName: "", token: "token", secret: "token"},
		{name: "blank token", pipeName: "pipe", token: "", secret: "pipe"},
		{name: "NUL pipe name", pipeName: "pipe\x00name", token: "token", secret: "pipe\x00name"},
		{name: "NUL token", pipeName: "pipe", token: "token\x00value", secret: "token\x00value"},
		{name: "NUL base entry", base: []string{"UNRELATED=ok\x00bad"}, pipeName: "pipe", token: "token", secret: "ok\x00bad"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := launchEnvironment(test.base, test.pipeName, test.token)
			if err == nil {
				t.Fatal("launchEnvironment() error = nil, want error")
			}
			if strings.Contains(err.Error(), test.secret) {
				t.Fatalf("launchEnvironment() leaked sensitive input in %q", err)
			}
		})
	}
}

func TestProcessLauncherStartsHelperWithExactSpec(t *testing.T) {
	launcher := NewProcessLauncher()
	outputPath := filepath.Join(t.TempDir(), "helper.json")
	workingDir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	spec := ProcessSpec{
		Executable: executable,
		Args:       []string{"-test.run=^TestProcessLauncherHelper$", "--", helperProcessArgument},
		WorkingDir: workingDir,
		Env: []string{
			"VRCFT_HELPER_OUTPUT=" + outputPath,
			"VRCFT_HELPER_VALUE=expected",
		},
	}
	process, err := launcher.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if process.PID() <= 0 {
		t.Fatalf("PID() = %d, want positive PID", process.PID())
	}
	if err := process.Wait(); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if err := process.Wait(); err != nil {
		t.Fatalf("second Wait() error = %v, want first result", err)
	}

	encoded, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read helper record: %v", err)
	}
	var record helperProcessRecord
	if err := json.Unmarshal(encoded, &record); err != nil {
		t.Fatalf("decode helper record: %v", err)
	}
	if got, want := record.Args, spec.Args; !equalStrings(got, want) {
		t.Fatalf("helper args = %q, want %q", got, want)
	}
	if got, want := record.Environment, expectedHelperEnvironment(spec.Env); !equalStrings(got, want) {
		t.Fatalf("helper environment = %q, want %q", got, want)
	}
	if got, want := record.WorkingDir, workingDir; got != want {
		t.Fatalf("helper working directory = %q, want %q", got, want)
	}
}

func TestProcessLauncherRejectsRelativeExecutableWithoutLeakingSecrets(t *testing.T) {
	_, err := NewProcessLauncher().Start(context.Background(), ProcessSpec{
		Executable: "relative-program",
		Args:       []string{"--session-token=do-not-leak"},
		Env:        []string{"VRCFT_SESSION_TOKEN=do-not-leak"},
	})
	if err == nil {
		t.Fatal("Start() error = nil, want error")
	}
	if strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("Start() leaked secret in %q", err)
	}
}

func TestProcessLauncherClassifiesMissingExecutableWithoutLeakingSecrets(t *testing.T) {
	missingExecutable := filepath.Join(t.TempDir(), "executable-secret-marker")
	_, err := NewProcessLauncher().Start(context.Background(), ProcessSpec{
		Executable: missingExecutable,
		Args:       []string{"--argument-secret-marker"},
		Env:        []string{"VRCFT_SESSION_TOKEN=environment-secret-marker"},
	})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Start() error = %v, want an error matching os.ErrNotExist", err)
	}
	for _, secret := range []string{missingExecutable, "argument-secret-marker", "environment-secret-marker"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("Start() leaked sensitive input in %q", err)
		}
	}
}

func TestProcessLauncherClassifiesMissingWorkingDirectoryWithoutLeakingSecrets(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	missingWorkingDir := filepath.Join(t.TempDir(), "working-directory-secret-marker")
	_, err = NewProcessLauncher().Start(context.Background(), ProcessSpec{
		Executable: executable,
		Args:       []string{"--argument-secret-marker"},
		WorkingDir: missingWorkingDir,
		Env:        []string{"VRCFT_SESSION_TOKEN=environment-secret-marker"},
	})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Start() error = %v, want an error matching os.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), "working directory") {
		t.Fatalf("Start() error = %q, want working-directory classification", err)
	}
	for _, secret := range []string{executable, missingWorkingDir, "argument-secret-marker", "environment-secret-marker"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("Start() leaked sensitive input in %q", err)
		}
	}
}

func TestSanitizedStartErrorClassifiesWorkingDirectoryPathErrorsWithoutLeakingSecrets(t *testing.T) {
	workingDir := filepath.Join(t.TempDir(), "working-directory-secret-marker")
	for _, test := range []struct {
		name  string
		err   error
		cause error
	}{
		{
			name:  "chdir not found",
			err:   &os.PathError{Op: "chdir", Path: workingDir, Err: os.ErrNotExist},
			cause: os.ErrNotExist,
		},
		{
			name:  "working directory path access denied",
			err:   &os.PathError{Op: "fork/exec", Path: workingDir, Err: os.ErrPermission},
			cause: os.ErrPermission,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := sanitizedStartError(test.err, workingDir)
			if !errors.Is(err, test.cause) {
				t.Fatalf("sanitizedStartError() error = %v, want error matching %v", err, test.cause)
			}
			if !strings.Contains(err.Error(), "working directory") {
				t.Fatalf("sanitizedStartError() error = %q, want working-directory classification", err)
			}
			for _, secret := range []string{workingDir, "argument-secret-marker", "environment-secret-marker"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("sanitizedStartError() leaked sensitive input in %q", err)
				}
			}
		})
	}
}

func TestSanitizedStartErrorRechecksWorkingDirectoryAfterStartFailure(t *testing.T) {
	missingWorkingDir := filepath.Join(t.TempDir(), "working-directory-secret-marker")
	err := sanitizedStartError(errors.New("start failure"), missingWorkingDir)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sanitizedStartError() error = %v, want error matching os.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), "working directory") {
		t.Fatalf("sanitizedStartError() error = %q, want working-directory classification", err)
	}
	if strings.Contains(err.Error(), missingWorkingDir) {
		t.Fatalf("sanitizedStartError() leaked working directory in %q", err)
	}
}

func TestProcessLauncherHonorsCanceledContextBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	process, err := NewProcessLauncher().Start(ctx, ProcessSpec{Executable: filepath.Join(t.TempDir(), "does-not-matter")})
	if process != nil {
		t.Fatal("Start() returned a process for a canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context.Canceled", err)
	}
}

func TestProcessLauncherKillStopsHelper(t *testing.T) {
	launcher := NewProcessLauncher()
	outputPath := filepath.Join(t.TempDir(), "helper.json")
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	process, err := launcher.Start(context.Background(), ProcessSpec{
		Executable: executable,
		Args:       []string{"-test.run=^TestProcessLauncherHelper$", "--", helperProcessArgument},
		Env:        []string{"VRCFT_HELPER_OUTPUT=" + outputPath, "VRCFT_HELPER_BLOCK=1"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		_ = process.Kill()
		_ = process.Wait()
	})
	if err := process.Kill(); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}
	if err := process.Wait(); err == nil {
		t.Fatal("Wait() error = nil after Kill(), want process exit error")
	}
}

func TestProcessLauncherHelper(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != helperProcessArgument {
		return
	}
	record := helperProcessRecord{
		Args:        append([]string(nil), os.Args[1:]...),
		Environment: sortedCopy(os.Environ()),
	}
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	record.WorkingDir = workingDir
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("encode helper record: %v", err)
	}
	if err := os.WriteFile(os.Getenv("VRCFT_HELPER_OUTPUT"), encoded, 0o600); err != nil {
		t.Fatalf("write helper record: %v", err)
	}
	if os.Getenv("VRCFT_HELPER_BLOCK") == "1" {
		select {}
	}
	os.Exit(0)
}

func validTestPipeName(name string) bool {
	if len(name) == 0 || len(name) > 128 {
		return false
	}
	for _, value := range []byte(name) {
		if !(value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '-' || value == '_') {
			return false
		}
	}
	return true
}

func environmentValues(t *testing.T, environment []string) map[string]string {
	t.Helper()
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			t.Fatalf("malformed environment entry %q", entry)
		}
		values[key] = value
	}
	return values
}

func countEnvironmentKey(environment []string, wanted string) int {
	count := 0
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, wanted) {
			count++
		}
	}
	return count
}

func sortedCopy(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func expectedHelperEnvironment(environment []string) []string {
	result := append([]string(nil), environment...)
	if runtime.GOOS == "windows" {
		result = append(result, "SYSTEMROOT="+os.Getenv("SYSTEMROOT"))
	}
	return sortedCopy(result)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
