package plugins

import (
	"math"
	"sync/atomic"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
)

type observedPluginLog struct {
	Entry   pluginapi.LogEntry
	Dropped uint64
}

func saturatingAddUint64(left, right uint64) uint64 {
	if right > math.MaxUint64-left {
		return math.MaxUint64
	}
	return left + right
}

func accumulateDropped(counter *atomic.Uint64, dropped uint64) {
	for {
		current := counter.Load()
		if counter.CompareAndSwap(current, saturatingAddUint64(current, dropped)) {
			return
		}
	}
}
