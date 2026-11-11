//go:build !windows

package picoconnect

import "errors"

func picoConnectRunning() (bool, error) {
	return false, errors.New("PICO Connect process detection requires Windows")
}
