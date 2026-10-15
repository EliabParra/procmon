package tui

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
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

// parseInterval reads what the user typed: seconds, optionally decimal with a
// point or a comma, optionally followed by "s" ("2", "0.5", "1,5", "3s").
func parseInterval(text string) (time.Duration, error) {
	s := strings.TrimSpace(text)
	s = strings.TrimSpace(strings.TrimSuffix(s, "s"))
	s = strings.Replace(s, ",", ".", 1)
	secs, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(secs) || math.IsInf(secs, 0) {
		return 0, errors.New("not a number of seconds")
	}
	if secs < MinInterval.Seconds() || secs > MaxInterval.Seconds() {
		return 0, fmt.Errorf("must be between %s and %s", formatInterval(MinInterval), formatInterval(MaxInterval))
	}
	return time.Duration(math.Round(secs*1000)) * time.Millisecond, nil
}

// setInterval changes the refresh interval and starts a new tick chain; the
// tick already waiting with the old interval is dropped when it arrives.
func (m *Model) setInterval(d time.Duration) tea.Cmd {
	m.interval = d
	m.gen++
	return tickAfter(d, m.gen)
}
