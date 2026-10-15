package monitor

import (
	"cmp"
	"slices"
	"strings"
)

// SortKey chooses the order of the process table.
type SortKey int

const (
	ByPID SortKey = iota // ascending PID (the default)
	ByCPU                // highest %CPU first
	ByMem                // highest %MEM first
)

// Sort orders procs in place by key. ByCPU and ByMem put the highest first;
// ties (and ByPID itself) are broken by ascending PID so the order does not
// jump around between refreshes.
func Sort(procs []Process, key SortKey) {
	slices.SortFunc(procs, func(a, b Process) int {
		switch key {
		case ByCPU:
			if c := cmp.Compare(b.CPUPercent, a.CPUPercent); c != 0 {
				return c
			}
		case ByMem:
			if c := cmp.Compare(b.MemPercent, a.MemPercent); c != 0 {
				return c
			}
		}
		return cmp.Compare(a.PID, b.PID)
	})
}

// Filter returns the processes whose command line or comm contains query,
// ignoring case. An empty query matches everything. The result is a new slice.
func Filter(procs []Process, query string) []Process {
	query = strings.ToLower(query)
	out := make([]Process, 0, len(procs))
	for _, p := range procs {
		if strings.Contains(strings.ToLower(p.Command), query) ||
			strings.Contains(strings.ToLower(p.Comm), query) {
			out = append(out, p)
		}
	}
	return out
}
