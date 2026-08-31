package projectstatus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxCapturedOutput = 64 * 1024

type commandDefinition struct {
	executable string
	prefix     []string
	noArgs     bool
}

var defaultCommandRegistry = map[string]commandDefinition{
	"go-list":           {executable: "go", prefix: []string{"list"}},
	"go-test":           {executable: "go", prefix: []string{"test"}},
	"go-test-build":     {executable: "go", prefix: []string{"test", "-run", "^$"}},
	"go-test-race":      {executable: "go", prefix: []string{"test", "-race"}},
	"go-vet":            {executable: "go", prefix: []string{"vet"}},
	"go-generate-check": {executable: "go", prefix: []string{"generate"}},
	"frontend-test":     {executable: "pnpm", prefix: []string{"--dir", "frontend", "run", "check"}, noArgs: true},
	"frontend-build":    {executable: "pnpm", prefix: []string{"--dir", "frontend", "run", "build"}, noArgs: true},
	"git-head":          {executable: "git", prefix: []string{"rev-parse", "HEAD"}, noArgs: true},
	"git-status":        {executable: "git", prefix: []string{"status", "--porcelain"}, noArgs: true},
}

type Runner struct {
	root     string
	registry map[string]commandDefinition
}

func NewRunner(root string) *Runner {
	return newRunner(root, defaultCommandRegistry)
}

func newRunner(root string, registry map[string]commandDefinition) *Runner {
	return &Runner{root: root, registry: registry}
}

func (runner *Runner) Run(parent context.Context, request CommandRequest) CommandOutput {
	started := time.Now()
	definition, exists := runner.registry[request.ID]
	if !exists {
		return commandValidationFailure(started, fmt.Errorf("unknown command id %q", request.ID))
	}
	if definition.noArgs && len(request.Args) > 0 {
		return commandValidationFailure(started, fmt.Errorf("command %s does not accept arguments", request.ID))
	}
	if err := validateCommandArgs(request.Args); err != nil {
		return commandValidationFailure(started, err)
	}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	args := append(append([]string(nil), definition.prefix...), request.Args...)
	cmd := exec.CommandContext(ctx, definition.executable, args...)
	cmd.Dir = request.Dir
	if cmd.Dir == "" {
		cmd.Dir = runner.root
	}
	var stdout, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	output := CommandOutput{
		Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(started),
		Truncated: stdout.Truncated() || stderr.Truncated(),
	}
	if ctx.Err() == context.DeadlineExceeded {
		output.ExitCode = -1
		output.TimedOut = true
		if output.Stderr == "" {
			output.Stderr = "command timed out"
		}
		return output
	}
	if err == nil {
		return output
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		output.ExitCode = exitErr.ExitCode()
	} else {
		output.ExitCode = 2
		if output.Stderr == "" {
			output.Stderr = err.Error()
		}
	}
	return output
}

func commandValidationFailure(started time.Time, err error) CommandOutput {
	return CommandOutput{ExitCode: 2, Stderr: err.Error(), Duration: time.Since(started)}
}

func validateCommandArgs(args []string) error {
	operators := map[string]bool{"|": true, "||": true, "&&": true, ";": true, ">": true, ">>": true, "<": true}
	environment := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	for _, arg := range args {
		if strings.ContainsRune(arg, 0) || operators[arg] || environment.MatchString(arg) {
			return fmt.Errorf("unsafe command argument %q", arg)
		}
	}
	return nil
}

type limitedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	truncated bool
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	original := len(data)
	remaining := maxCapturedOutput - buffer.buffer.Len()
	if remaining <= 0 {
		buffer.truncated = true
		return original, nil
	}
	if len(data) > remaining {
		data = data[:remaining]
		buffer.truncated = true
	}
	_, _ = buffer.buffer.Write(data)
	return original, nil
}

func (buffer *limitedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}

func (buffer *limitedBuffer) Truncated() bool {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.truncated
}

type CheckState string

const (
	CheckPassed  CheckState = "passed"
	CheckFailed  CheckState = "failed"
	CheckBlocked CheckState = "blocked"
)

type CheckResult struct {
	SpecID   string
	CheckID  string
	State    CheckState
	Weight   int
	Required bool
	Evidence string
	Duration time.Duration
}

type SpecResult struct {
	Spec   Spec
	Checks []CheckResult
}

type Evaluator struct {
	root   string
	runner CommandRunner
	cache  map[string]CommandOutput
}

func NewEvaluator(root string, runner CommandRunner) *Evaluator {
	return &Evaluator{root: root, runner: runner, cache: make(map[string]CommandOutput)}
}

func (evaluator *Evaluator) Evaluate(ctx context.Context, specs []Spec) []SpecResult {
	results := make([]SpecResult, 0, len(specs))
	byID := make(map[string]SpecResult, len(specs))
	for _, spec := range specs {
		result := SpecResult{Spec: spec, Checks: make([]CheckResult, 0, len(spec.Checks))}
		for _, check := range spec.Checks {
			result.Checks = append(result.Checks, evaluator.evaluateCheck(ctx, spec, check, byID))
		}
		results = append(results, result)
		byID[spec.ID] = result
	}
	return results
}

func (evaluator *Evaluator) evaluateCheck(ctx context.Context, spec Spec, check CheckSpec, prior map[string]SpecResult) CheckResult {
	result := CheckResult{SpecID: spec.ID, CheckID: check.ID, Weight: check.Weight, Required: check.Required, State: CheckFailed}
	started := time.Now()
	defer func() { result.Duration = time.Since(started) }()
	switch check.Type {
	case CheckCommand:
		timeout := time.Duration(check.TimeoutSeconds) * time.Second
		if timeout <= 0 {
			timeout = 120 * time.Second
		}
		request := CommandRequest{ID: check.Command, Args: check.Args, Timeout: timeout, Dir: evaluator.root}
		key := request.ID + "\x00" + strings.Join(request.Args, "\x00") + "\x00" + strconv.FormatInt(int64(request.Timeout), 10)
		output, exists := evaluator.cache[key]
		if !exists {
			output = evaluator.runner.Run(ctx, request)
			evaluator.cache[key] = output
		}
		result.Duration = output.Duration
		if output.ExitCode == 0 && !output.TimedOut {
			result.State = CheckPassed
			result.Evidence = strings.TrimSpace(output.Stdout)
		} else {
			result.Evidence = commandEvidence(output)
		}

	case CheckFile:
		path, err := safeRepositoryPath(evaluator.root, check.Path)
		if err != nil {
			result.Evidence = err.Error()
			break
		}
		if _, err := os.Stat(path); err != nil {
			result.Evidence = err.Error()
		} else {
			result.State = CheckPassed
			result.Evidence = filepath.ToSlash(check.Path)
		}

	case CheckSymbol, CheckNotPlaceholder:
		content, err := evaluator.readSafe(check.Path)
		if err != nil {
			result.Evidence = err.Error()
			break
		}
		patterns := check.Patterns
		if check.Pattern != "" {
			patterns = append(patterns, check.Pattern)
		}
		matched := false
		for _, pattern := range patterns {
			expression, err := regexp.Compile(pattern)
			if err != nil {
				result.Evidence = err.Error()
				return result
			}
			if expression.Match(content) {
				matched = true
				result.Evidence = "matched " + pattern
				break
			}
		}
		if (check.Type == CheckSymbol && matched) || (check.Type == CheckNotPlaceholder && !matched) {
			result.State = CheckPassed
			if result.Evidence == "" {
				result.Evidence = "no placeholder matched"
			}
		} else if result.Evidence == "" {
			result.Evidence = "required symbol not found"
		}

	case CheckDependsComplete:
		result.State = CheckPassed
		for _, dependency := range spec.DependsOn {
			dependencyResult, exists := prior[dependency]
			if !exists || !requiredChecksPassed(dependencyResult) {
				result.State = CheckBlocked
				result.Evidence = "dependency incomplete: " + dependency
				break
			}
		}

	case CheckAggregate:
		result.State = CheckPassed
		for _, member := range check.Members {
			parts := strings.SplitN(member, ":", 2)
			if len(parts) != 2 || !checkPassed(prior[parts[0]], parts[1]) {
				result.State = CheckBlocked
				result.Evidence = "aggregate member incomplete: " + member
				break
			}
		}

	case CheckGeneratedClean:
		if err := evaluator.generatedClean(ctx, check); err != nil {
			result.Evidence = err.Error()
		} else {
			result.State = CheckPassed
			result.Evidence = "generated files unchanged"
		}
	default:
		result.Evidence = "unsupported check type " + string(check.Type)
	}
	return result
}

func (evaluator *Evaluator) readSafe(relative string) ([]byte, error) {
	path, err := safeRepositoryPath(evaluator.root, relative)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func safeRepositoryPath(root, relative string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes repository", relative)
	}
	return target, nil
}

func (evaluator *Evaluator) generatedClean(ctx context.Context, check CheckSpec) error {
	temporary, err := os.MkdirTemp("", "projectstatus-generate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	for _, member := range check.Members {
		source, err := safeRepositoryPath(evaluator.root, member)
		if err != nil {
			return err
		}
		destination := filepath.Join(temporary, filepath.FromSlash(member))
		if err := copyPath(source, destination); err != nil {
			return err
		}
	}
	before, err := hashTree(temporary)
	if err != nil {
		return err
	}
	command := check.Command
	if command == "" {
		command = "go-generate-check"
	}
	output := evaluator.runner.Run(ctx, CommandRequest{ID: command, Args: check.Args, Timeout: 120 * time.Second, Dir: temporary})
	if output.ExitCode != 0 {
		return fmt.Errorf("generator failed: %s", commandEvidence(output))
	}
	after, err := hashTree(temporary)
	if err != nil {
		return err
	}
	if !bytes.Equal(before[:], after[:]) {
		return errors.New("generated files changed")
	}
	return nil
}

func copyPath(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			relative, _ := filepath.Rel(source, path)
			target := filepath.Join(destination, relative)
			if info.IsDir() {
				return os.MkdirAll(target, info.Mode())
			}
			return copyFile(path, target, info.Mode())
		})
	}
	return copyFile(source, destination, info.Mode())
}

func copyFile(source, destination string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func hashTree(root string) ([32]byte, error) {
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			paths = append(paths, path)
		}
		return err
	})
	if err != nil {
		return [32]byte{}, err
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		relative, _ := filepath.Rel(root, path)
		_, _ = io.WriteString(hash, filepath.ToSlash(relative)+"\x00")
		content, err := os.ReadFile(path)
		if err != nil {
			return [32]byte{}, err
		}
		_, _ = hash.Write(content)
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result, nil
}

func commandEvidence(output CommandOutput) string {
	evidence := strings.TrimSpace(output.Stderr)
	if evidence == "" {
		evidence = strings.TrimSpace(output.Stdout)
	}
	if output.TimedOut && evidence == "" {
		evidence = "command timed out"
	}
	return evidence
}

func requiredChecksPassed(result SpecResult) bool {
	for _, check := range result.Checks {
		if check.Required && check.State != CheckPassed {
			return false
		}
	}
	return true
}

func checkPassed(result SpecResult, id string) bool {
	for _, check := range result.Checks {
		if check.CheckID == id {
			return check.State == CheckPassed
		}
	}
	return false
}
