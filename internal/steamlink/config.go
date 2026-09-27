package steamlink

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

const defaultListenPort = 9015

type config struct {
	ListenPort int
}

func parseConfig(data json.RawMessage) (config, error) {
	result := config{ListenPort: defaultListenPort}
	if len(data) == 0 {
		return result, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil {
		return config{}, fmt.Errorf("steamlink configuration: invalid JSON: %w", err)
	}
	if first != json.Delim('{') {
		return config{}, fmt.Errorf("steamlink configuration: root must be an object")
	}

	seenListenPort := false
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return config{}, fmt.Errorf("steamlink configuration: invalid object key: %w", err)
		}
		keyName, ok := key.(string)
		if !ok {
			return config{}, fmt.Errorf("steamlink configuration: object key must be a string")
		}
		if keyName != "listenPort" {
			return config{}, fmt.Errorf("steamlink configuration: unknown field %q", keyName)
		}
		if seenListenPort {
			return config{}, fmt.Errorf("steamlink configuration: duplicate listenPort")
		}
		seenListenPort = true

		value, err := decoder.Token()
		if err != nil {
			return config{}, fmt.Errorf("steamlink configuration: invalid listenPort: %w", err)
		}
		number, ok := value.(json.Number)
		if !ok {
			return config{}, fmt.Errorf("steamlink configuration: listenPort must be an integer")
		}
		port, err := strconv.ParseInt(number.String(), 10, 32)
		if err != nil {
			return config{}, fmt.Errorf("steamlink configuration: listenPort must be an integer")
		}
		if port < 1 || port > 65535 {
			return config{}, fmt.Errorf("steamlink configuration: listenPort must be between 1 and 65535")
		}
		result.ListenPort = int(port)
	}

	closing, err := decoder.Token()
	if err != nil {
		return config{}, fmt.Errorf("steamlink configuration: invalid object: %w", err)
	}
	if closing != json.Delim('}') {
		return config{}, fmt.Errorf("steamlink configuration: root must be an object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return config{}, fmt.Errorf("steamlink configuration: trailing JSON value")
		}
		return config{}, fmt.Errorf("steamlink configuration: invalid trailing JSON: %w", err)
	}
	return result, nil
}
