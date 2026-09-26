package steamlink

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wzhqwq/vrcft-go/pkg/osc"
	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

const diagnosticInterval = 5 * time.Second

// diagnostics keeps only aggregate protocol facts. In particular it never
// retains packet bytes or received tracking values.
type diagnostics struct {
	malformed       uint64
	unsupported     uint64
	invalid         uint64
	queueDrops      uint64
	unknownMessages uint64
	unknownNames    map[string]uint64
	unknownOverflow uint64
	lastEmission    time.Time
	hasEmission     bool
}

func newDiagnostics() *diagnostics { return &diagnostics{unknownNames: make(map[string]uint64)} }

func (d *diagnostics) rejected(err error) {
	if errors.Is(err, osc.ErrUnsupportedType) {
		d.unsupported++
		return
	}
	d.malformed++
}

func (d *diagnostics) report(report inputReport) {
	d.invalid += uint64(report.InvalidMessages)
	d.unknownMessages += uint64(report.UnknownMessages)
	for _, address := range report.UnknownAddresses {
		d.unknown(address)
	}
	if remaining := report.UnknownMessages - len(report.UnknownAddresses); remaining > 0 {
		d.unknownOverflow += uint64(remaining)
	}
}

func (d *diagnostics) unknown(address string) {
	if _, exists := d.unknownNames[address]; exists {
		d.unknownNames[address]++
		return
	}
	if len(d.unknownNames) == maxUnknownAddresses {
		d.unknownOverflow++
		return
	}
	d.unknownNames[address] = 1
}

func (d *diagnostics) ready(now time.Time) bool {
	return !d.hasEmission || now.Sub(d.lastEmission) >= diagnosticInterval
}

func (d *diagnostics) emitted(now time.Time) { d.lastEmission, d.hasEmission = now, true }

func (d *diagnostics) emit(host pluginapi.Host, now time.Time) {
	if !d.ready(now) {
		return
	}
	emitted := false
	if d.malformed > 0 {
		host.Log(pluginapi.LogWarn, fmt.Sprintf("steamlink diagnostics: malformed packets=%d", d.malformed))
		d.malformed = 0
		emitted = true
	}
	if d.unsupported > 0 {
		host.Log(pluginapi.LogWarn, fmt.Sprintf("steamlink diagnostics: unsupported packets=%d", d.unsupported))
		d.unsupported = 0
		emitted = true
	}
	if d.invalid > 0 {
		host.Log(pluginapi.LogWarn, fmt.Sprintf("steamlink diagnostics: invalid messages=%d", d.invalid))
		d.invalid = 0
		emitted = true
	}
	if d.queueDrops > 0 {
		host.Log(pluginapi.LogWarn, fmt.Sprintf("steamlink diagnostics: queue drops=%d", d.queueDrops))
		d.queueDrops = 0
		emitted = true
	}
	if d.unknownMessages > 0 {
		names := make([]string, 0, len(d.unknownNames))
		for name := range d.unknownNames {
			names = append(names, name)
		}
		sort.Strings(names)
		message := fmt.Sprintf("steamlink diagnostics: unknown messages=%d names=%s", d.unknownMessages, strings.Join(names, ","))
		if d.unknownOverflow > 0 {
			message += fmt.Sprintf(" overflow=%d", d.unknownOverflow)
		}
		host.Log(pluginapi.LogWarn, message)
		d.unknownMessages, d.unknownOverflow = 0, 0
		clear(d.unknownNames)
		emitted = true
	}
	if emitted {
		d.emitted(now)
	}
}
