package processing

import "time"

type DropoutPolicy struct {
	HoldDuration  time.Duration
	DecayDuration time.Duration
	StaleAfter    time.Duration
}

func (state *channelState) recordFresh(value float32, atNS int64, policy DropoutPolicy) {
	state.seen = true
	state.lastFreshAtNS = atNS
	state.lastFreshValue = value
	state.dropoutStartNS = addDurationSaturated(atNS, policy.StaleAfter)
}

func (state *channelState) recordUnavailable(atNS int64) {
	if state.seen {
		state.dropoutStartNS = atNS
	}
}

func (state channelState) dropoutValue(policy DropoutPolicy, nowNS int64) (float32, bool) {
	if !state.seen {
		return 0, false
	}
	if nowNS <= state.dropoutStartNS {
		return state.lastFreshValue, true
	}

	elapsed := time.Duration(nowNS - state.dropoutStartNS)
	if elapsed <= policy.HoldDuration {
		return state.lastFreshValue, true
	}
	afterHold := elapsed - policy.HoldDuration
	if policy.DecayDuration == 0 || afterHold >= policy.DecayDuration {
		return 0, true
	}
	fractionRemaining := 1 - float64(afterHold)/float64(policy.DecayDuration)
	return state.lastFreshValue * float32(fractionRemaining), true
}

func addDurationSaturated(atNS int64, duration time.Duration) int64 {
	const maxInt64 = int64(^uint64(0) >> 1)
	delta := int64(duration)
	if atNS > maxInt64-delta {
		return maxInt64
	}
	return atNS + delta
}
