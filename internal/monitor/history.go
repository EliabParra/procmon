package monitor

import "slices"

// defaultHistory is how many %CPU samples are kept per process.
const defaultHistory = 60

// ring keeps the last few CPU samples of one process: once full, each new
// sample overwrites the oldest one.
type ring struct {
	buf  []float64
	next int // where the next sample is written
	full bool
}

func newRing(size int) *ring { return &ring{buf: make([]float64, max(size, 1))} }

func (r *ring) add(v float64) {
	r.buf[r.next] = v
	r.next++
	if r.next == len(r.buf) {
		r.next, r.full = 0, true
	}
}

// values returns the samples oldest first, in a new slice.
func (r *ring) values() []float64 {
	if !r.full {
		return slices.Clone(r.buf[:r.next])
	}
	return append(slices.Clone(r.buf[r.next:]), r.buf[:r.next]...)
}
