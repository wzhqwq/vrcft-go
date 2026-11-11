package picoconnect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

type config struct {
	TransferProtocol int
	ListenPort       int
}

func parseConfig(data json.RawMessage) (config, error) {
	result := config{TransferProtocol: 2, ListenPort: 29765}
	if len(data) == 0 {
		return result, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return config{}, fmt.Errorf("pico-connect configuration must be an object")
	}
	seen := map[string]bool{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return config{}, fmt.Errorf("pico-connect configuration key: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok || seen[key] {
			return config{}, fmt.Errorf("pico-connect configuration duplicate or invalid key")
		}
		seen[key] = true
		if key != "transferProtocol" && key != "listenPort" {
			return config{}, fmt.Errorf("pico-connect configuration unknown field %q", key)
		}
		valueToken, err := decoder.Token()
		if err != nil {
			return config{}, fmt.Errorf("pico-connect configuration value: %w", err)
		}
		value, ok := valueToken.(json.Number)
		if !ok {
			return config{}, fmt.Errorf("pico-connect configuration %s must be an integer", key)
		}
		integer, err := strconv.Atoi(value.String())
		if err != nil {
			return config{}, fmt.Errorf("pico-connect configuration %s must be an integer", key)
		}
		if key == "transferProtocol" {
			result.TransferProtocol = integer
		} else {
			result.ListenPort = integer
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return config{}, fmt.Errorf("pico-connect configuration incomplete object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return config{}, fmt.Errorf("pico-connect configuration trailing data")
	}
	if result.TransferProtocol != 2 {
		return config{}, fmt.Errorf("pico-connect transferProtocol must be 2")
	}
	if result.ListenPort < 1 || result.ListenPort > 65535 {
		return config{}, fmt.Errorf("pico-connect listenPort must be between 1 and 65535")
	}
	return result, nil
}
