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
	Address string
	Type    string
}

type rawConfigEndpoint struct {
	Address json.RawMessage `json:"address"`
	Type    json.RawMessage `json:"type"`
}

type rawConfigParameter struct {
	Name   json.RawMessage `json:"name"`
	Input  json.RawMessage `json:"input"`
	Output json.RawMessage `json:"output"`
}

type rawConfigDocument struct {
	ID         json.RawMessage `json:"id"`
	Name       json.RawMessage `json:"name"`
	Parameters json.RawMessage `json:"parameters"`
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

	var document rawConfigDocument
	if err := json.Unmarshal(root, &document); err != nil {
		return decodedConfig{}, fmt.Errorf("%w: root object: %v", ErrInvalidJSON, err)
	}
	if len(document.ID) == 0 {
		return decodedConfig{}, fmt.Errorf("%w: required id is missing or null", ErrInvalidJSON)
	}
	id, err := decodeKnownString(document.ID, "id")
	if err != nil {
		return decodedConfig{}, err
	}
	if len(document.Name) != 0 {
		if _, err := decodeKnownString(document.Name, "name"); err != nil {
			return decodedConfig{}, err
		}
	}
	if len(document.Parameters) == 0 || isJSONNull(document.Parameters) {
		return decodedConfig{}, fmt.Errorf("%w: required parameters are missing or null", ErrInvalidJSON)
	}

	var parameters []json.RawMessage
	if err := json.Unmarshal(document.Parameters, &parameters); err != nil {
		return decodedConfig{}, fmt.Errorf("%w: parameters: %v", ErrInvalidJSON, err)
	}
	if len(id) > maxAvatarIDBytes {
		return decodedConfig{}, fmt.Errorf("%w: configuration ID is %d bytes, maximum %d", ErrInvalidAvatarID, len(id), maxAvatarIDBytes)
	}
	if len(parameters) > maxParameters {
		return decodedConfig{}, fmt.Errorf("%w: %d parameters exceeds %d", ErrTooManyParameters, len(parameters), maxParameters)
	}

	endpoints := make([]osc.Endpoint, 0, len(parameters))
	for index, rawParameter := range parameters {
		input, ok, err := decodeConfigParameter(rawParameter, index)
		if err != nil {
			return decodedConfig{}, err
		}
		if !ok {
			continue
		}
		endpoint, err := decodeInputEndpoint(input)
		if err != nil {
			return decodedConfig{}, fmt.Errorf("%w: parameter %d: %v", ErrInvalidInputEndpoint, index, err)
		}
		endpoints = append(endpoints, endpoint)
	}

	return decodedConfig{id: id, endpoints: endpoints}, nil
}

func decodeConfigParameter(raw json.RawMessage, index int) (configEndpoint, bool, error) {
	if isJSONNull(raw) {
		return configEndpoint{}, false, fmt.Errorf("%w: parameter %d must be an object", ErrInvalidJSON, index)
	}
	var parameter rawConfigParameter
	if err := json.Unmarshal(raw, &parameter); err != nil {
		return configEndpoint{}, false, fmt.Errorf("%w: parameter %d: %v", ErrInvalidJSON, index, err)
	}
	if len(parameter.Name) != 0 {
		if _, err := decodeKnownString(parameter.Name, fmt.Sprintf("parameter %d name", index)); err != nil {
			return configEndpoint{}, false, err
		}
	}
	if len(parameter.Output) != 0 {
		if _, err := decodeConfigEndpoint(parameter.Output, fmt.Sprintf("parameter %d output", index)); err != nil {
			return configEndpoint{}, false, err
		}
	}
	if len(parameter.Input) == 0 {
		return configEndpoint{}, false, nil
	}
	input, err := decodeConfigEndpoint(parameter.Input, fmt.Sprintf("parameter %d input", index))
	return input, true, err
}

func decodeConfigEndpoint(raw json.RawMessage, field string) (configEndpoint, error) {
	if isJSONNull(raw) {
		return configEndpoint{}, fmt.Errorf("%w: %s must be an object", ErrInvalidJSON, field)
	}
	var encoded rawConfigEndpoint
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return configEndpoint{}, fmt.Errorf("%w: %s: %v", ErrInvalidJSON, field, err)
	}
	var endpoint configEndpoint
	var err error
	if len(encoded.Address) != 0 {
		endpoint.Address, err = decodeKnownString(encoded.Address, field+" address")
		if err != nil {
			return configEndpoint{}, err
		}
	}
	if len(encoded.Type) != 0 {
		endpoint.Type, err = decodeKnownString(encoded.Type, field+" type")
		if err != nil {
			return configEndpoint{}, err
		}
	}
	return endpoint, nil
}

func decodeKnownString(raw json.RawMessage, field string) (string, error) {
	if isJSONNull(raw) {
		return "", fmt.Errorf("%w: %s must be a string", ErrInvalidJSON, field)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrInvalidJSON, field, err)
	}
	return value, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
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
