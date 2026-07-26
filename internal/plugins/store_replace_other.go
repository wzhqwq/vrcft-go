//go:build !windows

package plugins

import "os"

func replaceJSONStoreFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
