package avatar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wzhqwq/vrcft-go/internal/osc"
)

const (
	maxConfigBytes     = 4 << 20
	maxParameters      = 4096
	maxOSCAddressBytes = 1024
	maxAvatarIDBytes   = 256
)

type decodedConfig struct {
	id        string
	endpoints []osc.Endpoint
}

type configEndpoint struct {
	Address string `json:"address"`
	Type    string `json:"type"`
}

type configParameter struct {
	Name   string          `json:"name"`
	Input  *configEndpoint `json:"input"`
	Output *configEndpoint `json:"output"`
}

type configDocument struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Parameters []configParameter `json:"parameters"`
}

func readConfig(path string) (decodedConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return decodedConfig{}, fmt.Errorf("%w: open %q: %v", ErrInvalidConfigPath, path, err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		return decodedConfig{}, fmt.Errorf("%w: read %q: %v", ErrInvalidConfigPath, path, err)
	}
	if len(data) > maxConfigBytes {
		return decodedConfig{}, fmt.Errorf("%w: %q exceeds %d bytes", ErrConfigTooLarge, path, maxConfigBytes)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	var root json.RawMessage
	if err := decoder.Decode(&root); err != nil {
		return decodedConfig{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return decodedConfig{}, fmt.Errorf("%w: trailing JSON value", ErrInvalidJSON)
		}
		return decodedConfig{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(root, &fields); err != nil {
		return decodedConfig{}, fmt.Errorf("%w: root object: %v", ErrInvalidJSON, err)
	}
	if value, ok := fields["id"]; !ok || bytes.Equal(value, []byte("null")) {
		return decodedConfig{}, fmt.Errorf("%w: required id is missing or null", ErrInvalidJSON)
	}
	if value, ok := fields["parameters"]; !ok || bytes.Equal(value, []byte("null")) {
		return decodedConfig{}, fmt.Errorf("%w: required parameters are missing or null", ErrInvalidJSON)
	}

	var document configDocument
	if err := json.Unmarshal(root, &document); err != nil {
		return decodedConfig{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if len(document.Parameters) > maxParameters {
		return decodedConfig{}, fmt.Errorf("%w: %d parameters exceeds %d", ErrTooManyParameters, len(document.Parameters), maxParameters)
	}

	endpoints := make([]osc.Endpoint, 0, len(document.Parameters))
	for index, parameter := range document.Parameters {
		if parameter.Input == nil {
			continue
		}
		endpoint, err := decodeInputEndpoint(*parameter.Input)
		if err != nil {
			return decodedConfig{}, fmt.Errorf("%w: parameter %d: %v", ErrInvalidInputEndpoint, index, err)
		}
		endpoints = append(endpoints, endpoint)
	}

	return decodedConfig{id: document.ID, endpoints: endpoints}, nil
}

func decodeInputEndpoint(input configEndpoint) (osc.Endpoint, error) {
	if len(input.Address) == 0 || len(input.Address) > maxOSCAddressBytes {
		return osc.Endpoint{}, fmt.Errorf("address length %d", len(input.Address))
	}
	if !strings.HasPrefix(input.Address, "/") {
		return osc.Endpoint{}, fmt.Errorf("address %q is not absolute", input.Address)
	}
	if strings.IndexByte(input.Address, 0) >= 0 {
		return osc.Endpoint{}, fmt.Errorf("address contains NUL")
	}

	var endpointType string
	switch input.Type {
	case "Int":
		endpointType = "i"
	case "Bool":
		endpointType = "T"
	case "Float":
		endpointType = "f"
	default:
		return osc.Endpoint{}, fmt.Errorf("unsupported type %q", input.Type)
	}
	return osc.Endpoint{Address: input.Address, Type: endpointType}, nil
}
