package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wzhqwq/vrcft-go/internal/projectstatus"
)

type evaluateFunc func(context.Context, string) (projectstatus.Status, error)

func main() {
	root, err := projectstatus.FindRepositoryRoot(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, root, evaluateProject))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, root string, evaluate evaluateFunc) int {
	flags := flag.NewFlagSet("projectstatus", flag.ContinueOnError)
	flags.SetOutput(stderr)
	write := flags.Bool("write", false, "write docs/project/status.md")
	check := flags.Bool("check", false, "check whether docs/project/status.md is current")
	format := flags.String("format", "markdown", "output format: markdown or json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || (*write && *check) || (*format != "markdown" && *format != "json") || (*write && *format != "markdown") || (*check && *format != "markdown") {
		fmt.Fprintln(stderr, "invalid projectstatus arguments")
		return 2
	}
	status, err := evaluate(ctx, root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	var content []byte
	if *format == "json" {
		content, err = projectstatus.RenderJSON(status)
	} else {
		content, err = projectstatus.RenderMarkdown(status)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	statusPath := filepath.Join(root, "docs", "project", "status.md")
	switch {
	case *write:
		if err := atomicWrite(statusPath, content); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		fmt.Fprintln(stdout, filepath.ToSlash(statusPath))
	case *check:
		committed, err := os.ReadFile(statusPath)
		if err != nil || !bytes.Equal(projectstatus.NormalizeMarkdown(committed), projectstatus.NormalizeMarkdown(content)) {
			fmt.Fprintln(stderr, "project status is stale; run: go run ./cmd/projectstatus -write")
			return 1
		}
	default:
		_, _ = stdout.Write(content)
	}
	if status.State == projectstatus.StateComplete {
		return 0
	}
	return 1
}

func evaluateProject(ctx context.Context, root string) (projectstatus.Status, error) {
	catalog, err := projectstatus.LoadCatalog(root)
	if err != nil {
		return projectstatus.Status{}, err
	}
	runner := projectstatus.NewRunner(root)
	packages, err := projectstatus.DiscoverGoPackages(ctx, root, runner)
	if err != nil {
		return projectstatus.Status{}, err
	}
	if err := projectstatus.ValidateCatalog(catalog, packages); err != nil {
		return projectstatus.Status{}, err
	}
	results := projectstatus.NewEvaluator(root, runner).Evaluate(ctx, catalog.Specs)
	commitOutput := runner.Run(ctx, projectstatus.CommandRequest{ID: "git-head", Dir: root, Timeout: 10 * time.Second})
	if commitOutput.ExitCode != 0 {
		return projectstatus.Status{}, errors.New("read git commit: " + strings.TrimSpace(commitOutput.Stderr))
	}
	dirtyOutput := runner.Run(ctx, projectstatus.CommandRequest{ID: "git-status", Dir: root, Timeout: 10 * time.Second})
	if dirtyOutput.ExitCode != 0 {
		return projectstatus.Status{}, errors.New("read git status: " + strings.TrimSpace(dirtyOutput.Stderr))
	}
	fingerprint, err := projectstatus.SourceFingerprint(root)
	if err != nil {
		return projectstatus.Status{}, err
	}
	return projectstatus.BuildStatus(projectstatus.StatusInput{
		Results: results, GeneratedAt: time.Now().UTC(),
		Commit: strings.TrimSpace(commitOutput.Stdout), SourceFingerprint: fingerprint,
		Dirty: strings.TrimSpace(dirtyOutput.Stdout) != "",
	}), nil
}

func atomicWrite(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".status-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err == nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryPath, path)
}
