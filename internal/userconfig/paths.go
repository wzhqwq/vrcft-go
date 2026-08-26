package userconfig

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var (
	ErrUnsupportedPlatform = errors.New("userconfig: unsupported platform")
	ErrInvalidEnvironment  = errors.New("userconfig: invalid environment")
)

type Environment struct{ GOOS, RoamingDir, UserProfile, Executable string }
type Paths struct {
	SettingsDir, SettingsFile, PluginStoreFile, BuiltinPluginDir, DefaultOSCRoot string
}

func ResolvePaths(env Environment) (Paths, error) {
	if env.GOOS != "windows" {
		return Paths{}, ErrUnsupportedPlatform
	}
	for _, value := range []string{env.RoamingDir, env.UserProfile, env.Executable} {
		if value == "" || strings.IndexByte(value, 0) >= 0 {
			return Paths{}, ErrInvalidEnvironment
		}
	}
	if !filepath.IsAbs(env.Executable) {
		return Paths{}, fmt.Errorf("%w: executable must be absolute", ErrInvalidEnvironment)
	}
	paths := Paths{}
	paths.SettingsDir = filepath.Join(env.RoamingDir, "vrcft-go")
	paths.SettingsFile = filepath.Join(paths.SettingsDir, "config.json")
	paths.PluginStoreFile = filepath.Join(paths.SettingsDir, "plugins.json")
	paths.BuiltinPluginDir = filepath.Join(filepath.Dir(env.Executable), "plugins")
	paths.DefaultOSCRoot = filepath.Join(env.UserProfile, "AppData", "LocalLow", "VRChat", "VRChat", "OSC")
	return paths, nil
}
