package processing

import "math"

type channelCandidate struct {
	value float32
	valid bool
}

func projectMutualExclusion(candidates *[channelCount]channelCandidate, groups [][]ChannelID) {
	for _, group := range groups {
		var winner ChannelID
		var winnerMagnitude float64
		for _, id := range group {
			candidate := candidates[id-1]
			if !candidate.valid {
				continue
			}
			magnitude := math.Abs(float64(candidate.value))
			if winner == 0 || magnitude > winnerMagnitude || magnitude == winnerMagnitude && id < winner {
				winner = id
				winnerMagnitude = magnitude
			}
		}
		if winner == 0 {
			continue
		}
		for _, id := range group {
			if id != winner && candidates[id-1].valid {
				candidates[id-1].value = 0
			}
		}
	}
}
