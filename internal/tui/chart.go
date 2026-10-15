package tui

import "math"

// sparkBlocks are the eight heights of a one-line bar chart.
var sparkBlocks = []rune("▁▂▃▄▅▆▇█")

// sparkMin is the smallest scale of the chart, in %. A series that stays below
// it (an idle process, or counter noise) is drawn against 1%, so it does not
// stretch to fill the chart.
const sparkMin = 1.0

// niceCeil is the smallest step of 1, 2, 5, 10, 20, 50, 100, 200, 500, ... (1,
// 2 and 5 times a power of ten) that is at least v. Anything up to sparkMin
// gives sparkMin.
func niceCeil(v float64) float64 {
	if !(v > sparkMin) { // also catches NaN
		return sparkMin
	}
	for decade := 1.0; ; decade *= 10 {
		for _, step := range [...]float64{1, 2, 5} {
			if step*decade >= v {
				return step * decade
			}
		}
	}
}

// sparkScale is the value a full-height block stands for: the largest sample
// rounded up to a nice step, never below sparkMin and never above limit, the
// whole machine in the active %CPU mode (a sample past it is clock skew
// between the two /proc reads and is drawn clipped, the figures above the
// chart still show it).
func sparkScale(values []float64, limit float64) float64 {
	_, _, peak := seriesStats(values)
	return math.Min(niceCeil(peak), math.Max(limit, sparkMin))
}

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
