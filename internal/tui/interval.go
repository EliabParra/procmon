package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// Limits of the refresh interval. Below the minimum, /proc would be read more
// often than the kernel updates the counters %CPU is computed from.
const (
	MinInterval = 200 * time.Millisecond
	MaxInterval = 60 * time.Second
)

// intervalSteps are the values the + and - keys move between.
var intervalSteps = []time.Duration{
	500 * time.Millisecond, time.Second, 2 * time.Second, 3 * time.Second, 5 * time.Second, 10 * time.Second,
}

// WithInterval sets the first refresh interval, raised or lowered into the
// allowed range.
func WithInterval(d time.Duration) Option {
	return func(m *Model) { m.interval = min(max(d, MinInterval), MaxInterval) }
}

// nextInterval is the step after cur (dir > 0) or before it (dir < 0). It
// reports false when there is none, so a custom 30s still steps down to 10s
// but cannot step up.
func nextInterval(cur time.Duration, dir int) (time.Duration, bool) {
	if dir > 0 {
		for _, s := range intervalSteps {
			if s > cur {
				return s, true
			}
		}
		return cur, false
	}
	for i := len(intervalSteps) - 1; i >= 0; i-- {
		if intervalSteps[i] < cur {
			return intervalSteps[i], true
		}
	}
	return cur, false
}

// setInterval changes the refresh interval and starts a new tick chain; the
// tick already waiting with the old interval is dropped when it arrives.
func (m *Model) setInterval(d time.Duration) tea.Cmd {
	m.interval = d
	m.gen++
	return tickAfter(d, m.gen)
}
