package plugins

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type Process interface {
	PID() int
	Wait() error
	Kill() error
}

type ProcessLauncher interface {
	Start(context.Context, ProcessSpec) (Process, error)
}

type ProcessSpec struct {
	Executable string
	Args       []string
	WorkingDir string
	Env        []string
}

type processLauncher struct{}

type commandProcess struct {
	command *exec.Cmd

	waitOnce sync.Once
	waitErr  error
}

type startFailureTarget uint8

const (
	startFailureExecutable startFailureTarget = iota + 1
	startFailureWorkingDirectory
)

func NewProcessLauncher() ProcessLauncher {
	return processLauncher{}
}

func (processLauncher) Start(ctx context.Context, spec ProcessSpec) (Process, error) {
	if ctx == nil {
		return nil, errors.New("plugins: process context is required")
	}
	if !filepath.IsAbs(spec.Executable) {
		return nil, errors.New("plugins: process executable must be absolute")
	}
	if spec.WorkingDir != "" {
		info, err := os.Stat(spec.WorkingDir)
		if err != nil {
			return nil, sanitizedStartError(err, startFailureWorkingDirectory)
		}
		if !info.IsDir() {
			return nil, errors.New("plugins: process working directory is unavailable")
		}
	}
	command := exec.Command(spec.Executable, spec.Args...)
	command.Dir = spec.WorkingDir
	command.Env = append([]string(nil), spec.Env...)
	configureProcess(command)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, sanitizedStartError(err, startFailureExecutable)
	}
	return &commandProcess{command: command}, nil
}

func sanitizedStartError(err error, target startFailureTarget) error {
	cause := err
	var executableError *exec.Error
	if errors.As(cause, &executableError) {
		cause = executableError.Err
	}
	var pathError *os.PathError
	if errors.As(cause, &pathError) {
		cause = pathError.Err
	}

	switch {
	case errors.Is(cause, os.ErrNotExist), os.IsNotExist(cause), errors.Is(cause, exec.ErrNotFound):
		return classifiedStartError(target, "not found", os.ErrNotExist)
	case errors.Is(cause, os.ErrPermission), os.IsPermission(cause):
		return classifiedStartError(target, "access denied", os.ErrPermission)
	default:
		return errors.New("plugins: start process failed")
	}
}

func classifiedStartError(target startFailureTarget, detail string, cause error) error {
	if target == startFailureWorkingDirectory {
		return fmt.Errorf("plugins: process working directory %s: %w", detail, cause)
	}
	return fmt.Errorf("plugins: process executable %s: %w", detail, cause)
}

func (process *commandProcess) PID() int {
	if process == nil || process.command == nil || process.command.Process == nil {
		return 0
	}
	return process.command.Process.Pid
}

func (process *commandProcess) Wait() error {
	process.waitOnce.Do(func() {
		process.waitErr = process.command.Wait()
	})
	return process.waitErr
}

func (process *commandProcess) Kill() error {
	if process == nil || process.command == nil || process.command.Process == nil {
		return errors.New("plugins: process is not running")
	}
	return process.command.Process.Kill()
}

func newSessionCredentials() (pipeName, token string, err error) {
	pipeBytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, pipeBytes); err != nil {
		return "", "", errors.New("plugins: generate session credentials")
	}
	tokenBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, tokenBytes); err != nil {
		return "", "", errors.New("plugins: generate session credentials")
	}
	return "session-" + base64.RawURLEncoding.EncodeToString(pipeBytes), base64.RawURLEncoding.EncodeToString(tokenBytes), nil
}

func launchEnvironment(base []string, pipeName, token string) ([]string, error) {
	if !validCredentialPipeName(pipeName) || strings.TrimSpace(token) == "" ||
		strings.IndexByte(pipeName, 0) >= 0 || strings.IndexByte(token, 0) >= 0 {
		return nil, errors.New("plugins: invalid launch credentials")
	}

	environment := make([]string, 0, len(base)+2)
	for _, entry := range base {
		if strings.IndexByte(entry, 0) >= 0 {
			return nil, errors.New("plugins: invalid launch environment")
		}
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "VRCFT_PIPE_NAME") || strings.EqualFold(key, "VRCFT_SESSION_TOKEN") {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment,
		"VRCFT_PIPE_NAME="+pipeName,
		"VRCFT_SESSION_TOKEN="+token,
	)
	return environment, nil
}

func validCredentialPipeName(name string) bool {
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
