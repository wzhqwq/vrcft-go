//go:build !windows

package plugins

import "os/exec"

func configureProcess(command *exec.Cmd) {}
