package userconfig

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestResolvePathsWindowsDerivesProductPaths(t *testing.T) {
	env := Environment{GOOS: "windows", RoamingDir: `C:\\Users\\Tester\\AppData\\Roaming`, UserProfile: `C:\\Users\\Tester`, Executable: `C:\\Program Files\\vrcft-go\\vrcft-go.exe`}
	paths, err := ResolvePaths(env)
	if err != nil {
		t.Fatal(err)
	}
	if paths.SettingsDir != filepath.Join(env.RoamingDir, "vrcft-go") || paths.SettingsFile != filepath.Join(env.RoamingDir, "vrcft-go", "config.json") || paths.PluginStoreFile != filepath.Join(env.RoamingDir, "vrcft-go", "plugins.json") || paths.BuiltinPluginDir != filepath.Join(filepath.Dir(env.Executable), "plugins") || paths.DefaultOSCRoot != filepath.Join(env.UserProfile, "AppData", "LocalLow", "VRChat", "VRChat", "OSC") {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestResolvePathsRejectsUnsupportedOrInvalidEnvironment(t *testing.T) {
	valid := Environment{GOOS: "windows", RoamingDir: `C:\\AppData`, UserProfile: `C:\\Users\\Tester`, Executable: `C:\\bin\\app.exe`}
	tests := []struct {
		name   string
		mutate func(*Environment)
		want   error
	}{
		{"non-windows", func(e *Environment) { e.GOOS = "linux" }, ErrUnsupportedPlatform},
		{"blank roaming", func(e *Environment) { e.RoamingDir = "" }, ErrInvalidEnvironment},
		{"blank profile", func(e *Environment) { e.UserProfile = "" }, ErrInvalidEnvironment},
		{"relative executable", func(e *Environment) { e.Executable = "app.exe" }, ErrInvalidEnvironment},
		{"nul", func(e *Environment) { e.Executable = "C:\\bin\\a\x00.exe" }, ErrInvalidEnvironment},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := valid
			tt.mutate(&env)
			_, err := ResolvePaths(env)
			if !errors.Is(err, tt.want) {
				t.Fatalf("ResolvePaths() error = %v, want %v", err, tt.want)
			}
		})
	}
}
