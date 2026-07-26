//go:build windows

package plugins

import "os/exec"

// configureProcess is deliberately platform-specific so Windows containment
// can be strengthened without exposing exec.Cmd beyond this package.
func configureProcess(command *exec.Cmd) {}
