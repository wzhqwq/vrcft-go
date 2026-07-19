package processing

import "time"

type DropoutPolicy struct {
	HoldDuration  time.Duration
	DecayDuration time.Duration
	StaleAfter    time.Duration
}
