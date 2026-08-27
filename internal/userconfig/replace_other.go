//go:build !windows

package userconfig

import "os"

func replaceSettingsFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
