package picoconnect

import "testing"

func TestParseConfigModesAndPorts(t *testing.T) {
	defaultConfig, err := parseConfig(nil)
	if err != nil || defaultConfig.TransferProtocol != 2 || defaultConfig.ListenPort != 29765 {
		t.Fatalf("default: %+v %v", defaultConfig, err)
	}
	legacy, err := parseConfig([]byte(`{"transferProtocol":2,"listenPort":12345}`))
	if err != nil || legacy.TransferProtocol != 2 || legacy.ListenPort != 12345 {
		t.Fatalf("legacy: %+v %v", legacy, err)
	}
	for _, input := range []string{`{"transferProtocol":0}`, `{"transferProtocol":1}`, `{"listenPort":0}`, `{"unknown":1}`, `{"transferProtocol":2,"transferProtocol":0}`, `null`} {
		if _, err := parseConfig([]byte(input)); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}
