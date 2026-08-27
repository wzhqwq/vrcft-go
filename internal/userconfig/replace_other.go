//go:build !windows

package userconfig

import (
	"errors"
	"os"
	"syscall"
)

type documentIdentity struct {
	device uint64
	inode  uint64
}

func documentIdentityForFile(_ storeFile, info os.FileInfo) (documentIdentity, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return documentIdentity{}, errors.New("settings file does not expose device and inode")
	}
	return documentIdentity{device: uint64(stat.Dev), inode: uint64(stat.Ino)}, nil
}

func replaceSettingsFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
