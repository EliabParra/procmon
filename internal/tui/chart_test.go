package tui

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSparklineLevels(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		top    float64
		want   string
	}{
		{"zero is the lowest block, the top of the scale the highest", []float64{0, 100}, 100, "▁█"},
		{"every level on the way up", []float64{0, 25, 50, 75, 100}, 100, "▁▃▅▆█"},
		{"a sample above the top is clipped to the highest block", []float64{0, 100, 200}, 100, "▁██"},
		{"a smaller top stretches the same series", []float64{0, 5, 10}, 10, "▁▅█"},
		{"a flat idle series", []float64{0, 0, 0, 0}, 1, "▁▁▁▁"},
		{"the largest sample is the tallest column, below the top it stops short", []float64{1, 6, 3}, 10, "▂▅▃"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sparkline(tt.values, len(tt.values), tt.top); got != tt.want {
				t.Errorf("sparkline(%v, top %v) = %q, want %q", tt.values, tt.top, got, tt.want)
			}
		})
	}
}

func TestSparkScale(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		limit  float64
		want   float64
	}{
		{"all zeros keep the floor", []float64{0, 0, 0}, 100, 1},
		{"no samples keep the floor", nil, 100, 1},
		{"noise below the floor does not stretch the chart", []float64{0, 0.4, 0.1}, 100, 1},
		{"exactly the floor", []float64{1}, 100, 1},
		{"just above the floor", []float64{1.01}, 100, 2},
		{"1 6 3 round up to 10", []float64{1, 6, 3}, 100, 10},
		{"2 is a step", []float64{2}, 100, 2},
		{"3 rounds up to 5", []float64{3}, 100, 5},
		{"5 is a step", []float64{5}, 100, 5},
		{"5.1 rounds up to 10", []float64{5.1}, 100, 10},
		{"11 rounds up to 20", []float64{4, 11, 7}, 100, 20},
		{"21 rounds up to 50", []float64{21}, 100, 50},
		{"60 rounds up to 100", []float64{0, 60}, 100, 100},
		{"100 is a step", []float64{100}, 100, 100},
		{"per-core above one core goes past 100", []float64{80, 160, 240}, 1600, 500},
		{"per-core 1000 is a step", []float64{1000}, 1600, 1000},
		{"never above the whole machine", []float64{50, 105}, 100, 100},
		{"per-core never above all the cores", []float64{150, 99999}, 400, 400},
		{"NaN, infinite and negative samples count as zero", []float64{math.NaN(), math.Inf(1), -5}, 100, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sparkScale(tt.values, tt.limit); got != tt.want {
				t.Errorf("sparkScale(%v, limit %v) = %v, want %v", tt.values, tt.limit, got, tt.want)
			}
		})
	}
}

func TestNiceCeilSteps(t *testing.T) {
	steps := []float64{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000}
	for i, step := range steps {
		if got := niceCeil(step); got != step {
			t.Errorf("niceCeil(%v) = %v, a step is its own ceiling", step, got)
		}
		if i > 0 {
			if got := niceCeil(steps[i-1] + 0.5); got != step {
				t.Errorf("niceCeil(%v) = %v, want the next step %v", steps[i-1]+0.5, got, step)
			}
		}
	}
	if got := niceCeil(0.2); got != 1 {
		t.Errorf("niceCeil(0.2) = %v, want the floor 1", got)
	}
}

func TestSparklineFillsTheWidthByStretching(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		width  int
		want   string
	}{
		{"twice as wide", []float64{0, 100}, 4, "▁▁██"},
		{"an uneven split spreads the remainder", []float64{0, 50, 100}, 7, "▁▁▁▅▅██"},
		{"one sample fills the whole line", []float64{100}, 5, "█████"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sparkline(tt.values, tt.width, 100)
			if got != tt.want {
				t.Errorf("sparkline(%v, %d) = %q, want %q", tt.values, tt.width, got, tt.want)
			}
		})
	}
}

func TestSparklineKeepsTheNewestSamplesWhenThereAreTooMany(t *testing.T) {
	values := []float64{100, 100, 100, 0, 0, 100} // oldest first
	if got := sparkline(values, 3, 100); got != "▁▁█" {
		t.Errorf("sparkline = %q, want the last three samples: ▁▁█", got)
	}
}

func TestSparklineIsAlwaysExactlyAsWideAsAsked(t *testing.T) {
	for n := 1; n <= 12; n++ {
		values := make([]float64, n)
		for i := range values {
			values[i] = float64(i * 9)
		}
		for width := 1; width <= 30; width++ {
			if got := utf8.RuneCountInString(sparkline(values, width, 100)); got != width {
				t.Fatalf("%d samples in %d columns gave %d characters", n, width, got)
			}
		}
	}
}

func TestSparklineWithNothingToDraw(t *testing.T) {
	for name, tt := range map[string]struct {
		values []float64
		width  int
	}{
		"no samples":  {nil, 10},
		"zero width":  {[]float64{1, 2}, 0},
		"negative":    {[]float64{1, 2}, -3},
		"empty slice": {[]float64{}, 4},
	} {
		if got := sparkline(tt.values, tt.width, 100); got != "" {
			t.Errorf("%s: sparkline = %q, want empty", name, got)
		}
	}
}

func TestSparklineSurvivesBadValues(t *testing.T) {
	got := sparkline([]float64{math.NaN(), -5, math.Inf(1), 50}, 4, 100)
	if utf8.RuneCountInString(got) != 4 || !strings.ContainsAny(got, "▁▂▃▄▅▆▇█") {
		t.Errorf("sparkline = %q, want 4 block characters", got)
	}
	if !strings.HasPrefix(got, "▁▁") {
		t.Errorf("NaN and negative samples draw as the lowest block: %q", got)
	}
}

func TestSparklineSurvivesABadTop(t *testing.T) {
	for _, top := range []float64{0, -3, math.NaN()} {
		if got := sparkline([]float64{5, 0}, 2, top); got != "█▁" {
			t.Errorf("sparkline with top %v = %q, want the floor of 1 to apply: █▁", top, got)
		}
	}
}

func TestSeriesStats(t *testing.T) {
	min, avg, max := seriesStats([]float64{2, 4, 12})
	if min != 2 || avg != 6 || max != 12 {
		t.Errorf("stats = %v %v %v, want 2 6 12", min, avg, max)
	}
	if min, avg, max := seriesStats(nil); min != 0 || avg != 0 || max != 0 {
		t.Errorf("stats of nothing = %v %v %v, want zeros", min, avg, max)
	}
}
