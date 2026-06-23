package service

import actdomain "github.com/dipu/atmos-core/internal/activity/domain"

// bestEcoAlternative returns the greenest readily-available substitute for a
// given transport mode, or nil when the mode is already zero-emission (no
// greener alternative to suggest).
func bestEcoAlternative(mode actdomain.TransportMode) *actdomain.TransportMode {
	alt := func(m actdomain.TransportMode) *actdomain.TransportMode { return &m }

	switch mode {
	case actdomain.ModeCar, actdomain.ModeCAB, actdomain.ModeFlight:
		return alt(actdomain.ModeMetro)
	case actdomain.ModeTwoWheeler, actdomain.ModeAutoRickshaw:
		return alt(actdomain.ModeBus)
	case actdomain.ModeBus:
		return alt(actdomain.ModeMetro)
	case actdomain.ModeTrain, actdomain.ModeMetro:
		return alt(actdomain.ModeCycling)
	default: // walking, walk, cycling, bicycle — already zero-emission
		return nil
	}
}
