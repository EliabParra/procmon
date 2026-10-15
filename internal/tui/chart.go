package tui

import "math"

// sparkBlocks are the eight heights of a one-line bar chart.
var sparkBlocks = []rune("▁▂▃▄▅▆▇█")

// sparkMin is the smallest scale of the chart, in %. A series that stays below
// it (an idle process, or counter noise) is drawn against 1%, so it does not
// stretch to fill the chart.
const sparkMin = 1.0

// sparkline draws values, oldest first, as width block characters against a
// scale where top is the highest block. It always fills the width: with fewer
// samples than columns each sample is stretched over several columns, with
// more only the newest ones are drawn. NaN, infinite and negative samples
// count as zero.
func sparkline(values []float64, width int, top float64) string {
	if len(values) == 0 || width <= 0 {
		return ""
	}
	if !(top >= sparkMin) { // zero, negative or NaN: never divide by it
		top = sparkMin
	}

	if len(values) > width {
		values = values[len(values)-width:]
	}
	out := make([]rune, width)
	for col := range out {
		v := finite(values[col*len(values)/width])
		level := int(math.Round(v / top * float64(len(sparkBlocks)-1)))
		out[col] = sparkBlocks[min(max(level, 0), len(sparkBlocks)-1)]
	}
	return string(out)
}

// seriesStats are the smallest, mean and largest sample (zeros for none).
func seriesStats(values []float64) (lo, avg, hi float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	lo = math.Inf(1)
	sum := 0.0
	for _, v := range values {
		v = finite(v)
		lo, hi = math.Min(lo, v), math.Max(hi, v)
		sum += v
	}
	return lo, sum / float64(len(values)), hi
}

// finite turns anything that is not a usable %CPU into zero.
func finite(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return v
}
