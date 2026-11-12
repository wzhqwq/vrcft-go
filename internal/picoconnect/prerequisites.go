package picoconnect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func checkPrerequisites() string {
	running, err := picoConnectRunning()
	if err != nil {
		return "Cannot check whether PICO Connect is running: " + err.Error()
	}
	if !running {
		return "Start PICO Connect to enable face tracking"
	}
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData, err = os.UserConfigDir()
		if err != nil {
			return "Cannot locate PICO Connect settings: " + err.Error()
		}
	}
	return checkTransferSetting(filepath.Join(appData, "PICO Connect", "settings.json"))
}

func checkTransferSetting(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "Cannot read PICO Connect settings.json"
	}
	var settings struct {
		Lab struct {
			Protocol *int `json:"faceTrackingTransferProtocol"`
		} `json:"lab"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return "Cannot parse PICO Connect settings.json: " + err.Error()
	}
	if settings.Lab.Protocol == nil {
		return "Set lab.faceTrackingTransferProtocol to 2 in PICO Connect settings.json"
	}
	if *settings.Lab.Protocol != 2 {
		return fmt.Sprintf("PICO Connect lab.faceTrackingTransferProtocol is %d; set it to 2 and restart PICO Connect", *settings.Lab.Protocol)
	}
	return ""
}
