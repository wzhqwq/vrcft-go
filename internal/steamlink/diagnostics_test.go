package steamlink

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

func TestDiagnosticsBoundsUnknownNamesAndRateLimits(t *testing.T) {
	d := newDiagnostics()
	now := time.Unix(100, 0)
	for i := 0; i < maxUnknownAddresses+3; i++ {
		d.unknown("/unknown/" + string(rune('a'+i)))
	}
	if len(d.unknownNames) != maxUnknownAddresses || d.unknownOverflow != 3 {
		t.Fatalf("names=%d overflow=%d", len(d.unknownNames), d.unknownOverflow)
	}
	if !d.ready(now) {
		t.Fatal("first diagnostics summary not ready")
	}
	d.emitted(now)
	if d.ready(now.Add(4 * time.Second)) {
		t.Fatal("diagnostics emitted before rate limit")
	}
	if !d.ready(now.Add(5 * time.Second)) {
		t.Fatal("diagnostics not ready at rate limit")
	}
	for name := range d.unknownNames {
		if strings.Contains(name, "0.42") {
			t.Fatal("raw value retained")
		}
	}
}

func TestDiagnosticsLogsFirstFailureAfterQuietTicks(t *testing.T) {
	d := newDiagnostics()
	host := newFakeHost(pluginapi.Startup{})
	now := time.Unix(100, 0)
	d.emit(host, now)
	d.rejected(errors.New("bad packet"))
	d.emit(host, now.Add(time.Millisecond))
	if len(host.logs) != 1 || !strings.Contains(host.logs[0], "malformed packets=1") {
		t.Fatalf("logs = %q", host.logs)
	}
}
