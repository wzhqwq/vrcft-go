//go:build windows

package picoconnect

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func picoConnectRunning() (bool, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return false, err
	}
	for {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "PICO Connect.exe") {
			return true, nil
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				return false, nil
			}
			return false, err
		}
	}
}
