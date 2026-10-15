package monitor

import (
	"slices"
	"testing"
)

func pids(procs []Process) []int {
	out := make([]int, len(procs))
	for i, p := range procs {
		out[i] = p.PID
	}
	return out
}

func TestSort(t *testing.T) {
	input := func() []Process {
		return []Process{
			{PID: 30, CPUPercent: 5, MemPercent: 1},
			{PID: 10, CPUPercent: 50, MemPercent: 1},
			{PID: 20, CPUPercent: 5, MemPercent: 9},
			{PID: 5, CPUPercent: 0, MemPercent: 0},
		}
	}
	tests := []struct {
		name string
		key  SortKey
		want []int
	}{
		{"by pid ascending", ByPID, []int{5, 10, 20, 30}},
		{"by cpu highest first, ties by pid", ByCPU, []int{10, 20, 30, 5}},
		{"by mem highest first, ties by pid", ByMem, []int{20, 10, 30, 5}},
		{"unknown key behaves like by pid", SortKey(99), []int{5, 10, 20, 30}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			procs := input()
			Sort(procs, tt.key)
			if got := pids(procs); !slices.Equal(got, tt.want) {
				t.Fatalf("order = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSortIsStableWhateverTheInputOrder(t *testing.T) {
	base := []Process{
		{PID: 1, CPUPercent: 3}, {PID: 2, CPUPercent: 3}, {PID: 3, CPUPercent: 3},
		{PID: 4, CPUPercent: 9}, {PID: 5, CPUPercent: 3},
	}
	want := []int{4, 1, 2, 3, 5}
	for _, perm := range [][]int{{0, 1, 2, 3, 4}, {4, 3, 2, 1, 0}, {2, 4, 0, 3, 1}, {3, 0, 4, 2, 1}} {
		procs := make([]Process, len(base))
		for i, j := range perm {
			procs[i] = base[j]
		}
		Sort(procs, ByCPU)
		if got := pids(procs); !slices.Equal(got, want) {
			t.Fatalf("input order %v gave %v, want %v", perm, got, want)
		}
	}
}
