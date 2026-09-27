package steamlink

import (
	"encoding/json"
	"testing"
)

func TestConfigAcceptsDefaultAndPortEndpoints(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
		want int
	}{
		{name: "nil uses default", want: 9015},
		{name: "empty bytes use default", data: json.RawMessage{}, want: 9015},
		{name: "empty object uses default", data: json.RawMessage(`{}`), want: 9015},
		{name: "explicit port", data: json.RawMessage(`{"listenPort":9017}`), want: 9017},
		{name: "lowest port", data: json.RawMessage(`{"listenPort":1}`), want: 1},
		{name: "highest port", data: json.RawMessage(`{"listenPort":65535}`), want: 65535},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseConfig(tt.data)
			if err != nil {
				t.Fatal(err)
			}
			if got.ListenPort != tt.want {
				t.Fatalf("ListenPort = %d, want %d", got.ListenPort, tt.want)
			}
		})
	}
}

func TestConfigRejectsInvalidPortAndDocument(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
	}{
		{name: "zero", data: json.RawMessage(`{"listenPort":0}`)},
		{name: "too high", data: json.RawMessage(`{"listenPort":65536}`)},
		{name: "negative", data: json.RawMessage(`{"listenPort":-1}`)},
		{name: "fraction", data: json.RawMessage(`{"listenPort":9015.5}`)},
		{name: "string", data: json.RawMessage(`{"listenPort":"9015"}`)},
		{name: "null", data: json.RawMessage(`null`)},
		{name: "null port", data: json.RawMessage(`{"listenPort":null}`)},
		{name: "array", data: json.RawMessage(`[]`)},
		{name: "unknown field", data: json.RawMessage(`{"other":9015}`)},
		{name: "trailing value", data: json.RawMessage(`{} {}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseConfig(tt.data); err == nil {
				t.Fatal("parseConfig accepted invalid configuration")
			}
		})
	}
}

func TestConfigRejectsDuplicatePort(t *testing.T) {
	_, err := parseConfig(json.RawMessage(`{"listenPort":9015,"listenPort":9017}`))
	if err == nil {
		t.Fatal("duplicate listenPort accepted")
	}
}
