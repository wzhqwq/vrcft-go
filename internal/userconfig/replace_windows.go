//go:build windows

package userconfig

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

type documentIdentity struct {
	volumeSerial uint32
	fileIndexHi  uint32
	fileIndexLo  uint32
}

func documentIdentityForFile(file storeFile, _ os.FileInfo) (documentIdentity, error) {
	connectionFile, ok := file.(interface {
		SyscallConn() (syscall.RawConn, error)
	})
	if !ok {
		return documentIdentity{}, errors.New("settings file does not expose a system handle")
	}
	connection, err := connectionFile.SyscallConn()
	if err != nil {
		return documentIdentity{}, err
	}
	var information windows.ByHandleFileInformation
	var controlErr error
	if err := connection.Control(func(handle uintptr) {
		controlErr = windows.GetFileInformationByHandle(windows.Handle(handle), &information)
	}); err != nil {
		return documentIdentity{}, err
	}
	if controlErr != nil {
		return documentIdentity{}, controlErr
	}
	return documentIdentity{volumeSerial: information.VolumeSerialNumber, fileIndexHi: information.FileIndexHigh, fileIndexLo: information.FileIndexLow}, nil
}

func replaceSettingsFile(oldPath, newPath string) error {
	oldUTF16, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	newUTF16, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(oldUTF16, newUTF16, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
