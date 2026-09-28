//go:build windows

package plugins

import (
	"os/exec"
	"testing"
)

func TestConfigureProcessSuppressesManagedPluginConsole(t *testing.T) {
	command := exec.Command("plugin.exe")
	configureProcess(command)
	if command.SysProcAttr == nil || !command.SysProcAttr.HideWindow {
		t.Fatal("managed plugin console is not hidden")
	}
}
