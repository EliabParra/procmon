package monitor

import (
	"slices"
	"testing"
	"time"
)

// forest is a hand-built process list:
//
//	1 init            2 kthreadd         500 orphan (parent 999 is not listed)
//	├─ 100 a          └─ 3 rcu
//	│  ├─ 200 c
//	│  └─ 201 d
//	└─ 101 b
//	   └─ 300 e
func forest() []Process {
	return []Process{
		{PID: 1, PPID: 0, Command: "init"},
		{PID: 2, PPID: 0, Command: "kthreadd"},
		{PID: 3, PPID: 2, Command: "rcu"},
		{PID: 100, PPID: 1, Command: "a"},
		{PID: 101, PPID: 1, Command: "b"},
		{PID: 200, PPID: 100, Command: "c"},
		{PID: 201, PPID: 100, Command: "d"},
		{PID: 300, PPID: 101, Command: "e"},
		{PID: 500, PPID: 999, Command: "orphan"},
	}
}

func render(rows []TreeRow) []string {
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = r.Prefix + r.Process.Command
	}
	return lines
}

func TestBuildTree(t *testing.T) {
	rows := BuildTree(forest())

	wantLines := []string{
		"init",
		"├─ a",
		"│  ├─ c",
		"│  └─ d",
		"└─ b",
		"   └─ e",
		"kthreadd",
		"└─ rcu",
		"orphan",
	}
	if got := render(rows); !slices.Equal(got, wantLines) {
		t.Fatalf("tree =\n%q\nwant\n%q", got, wantLines)
	}

	wantDepth := []int{0, 1, 2, 2, 1, 2, 0, 1, 0}
	var gotDepth []int
	for _, r := range rows {
		gotDepth = append(gotDepth, r.Depth)
	}
	if !slices.Equal(gotDepth, wantDepth) {
		t.Fatalf("depths = %v, want %v", gotDepth, wantDepth)
	}
}

func TestBuildTreeCarriesTheWholeProcess(t *testing.T) {
	procs := []Process{{PID: 1, PPID: 0, Command: "init", CPUPercent: 12.5, CPUTime: time.Second}}
	rows := BuildTree(procs)
	if len(rows) != 1 || rows[0].Process != procs[0] {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestBuildTreeDeepChain(t *testing.T) {
	procs := []Process{
		{PID: 1, PPID: 0, Command: "a"},
		{PID: 2, PPID: 1, Command: "b"},
		{PID: 3, PPID: 2, Command: "c"},
		{PID: 4, PPID: 3, Command: "d"},
	}
	want := []string{"a", "└─ b", "   └─ c", "      └─ d"}
	if got := render(BuildTree(procs)); !slices.Equal(got, want) {
		t.Fatalf("tree = %q, want %q", got, want)
	}
}

func TestBuildTreeKeepsTheOrderOfTheInput(t *testing.T) {
	procs := forest()
	// The caller sorted by CPU: b is busier than a, rcu than kthreadd's other children.
	procs[3].CPUPercent, procs[4].CPUPercent = 1, 9
	Sort(procs, ByCPU)

	got := render(BuildTree(procs))
	// Roots and siblings follow the sorted input: b before a under init.
	wantStart := []string{"init", "├─ b", "│  └─ e", "└─ a"}
	if len(got) < len(wantStart) || !slices.Equal(got[:len(wantStart)], wantStart) {
		t.Fatalf("tree = %q, want it to start with %q", got, wantStart)
	}
}

func TestBuildTreeShowsEveryProcessOnce(t *testing.T) {
	tests := []struct {
		name  string
		procs []Process
	}{
		{"forest", forest()},
		{"process that is its own parent", []Process{
			{PID: 20, PPID: 20, Command: "self"},
			{PID: 21, PPID: 20, Command: "child"},
		}},
		{"two-process cycle with a descendant", []Process{
			{PID: 1, PPID: 0, Command: "init"},
			{PID: 10, PPID: 11, Command: "x"},
			{PID: 11, PPID: 10, Command: "y"},
			{PID: 12, PPID: 11, Command: "z"},
		}},
		{"three-process cycle", []Process{
			{PID: 7, PPID: 9, Command: "p"},
			{PID: 8, PPID: 7, Command: "q"},
			{PID: 9, PPID: 8, Command: "r"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan []TreeRow, 1)
			go func() { done <- BuildTree(tt.procs) }()

			var rows []TreeRow
			select {
			case rows = <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("BuildTree did not finish: it loops on a cycle")
			}

			got := make([]int, len(rows))
			for i, r := range rows {
				got[i] = r.Process.PID
			}
			slices.Sort(got)
			want := pids(tt.procs)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("tree shows pids %v, want each of %v exactly once", got, want)
			}
		})
	}
}

func TestBuildTreeEmpty(t *testing.T) {
	if rows := BuildTree(nil); len(rows) != 0 {
		t.Fatalf("rows = %+v, want none", rows)
	}
}

func TestWithAncestors(t *testing.T) {
	all := forest()
	byCommand := func(names ...string) []Process {
		var out []Process
		for _, p := range all {
			if slices.Contains(names, p.Command) {
				out = append(out, p)
			}
		}
		return out
	}

	tests := []struct {
		name   string
		subset []Process
		want   []int // PIDs, in the order of the full list
	}{
		{"a deep match brings every ancestor", byCommand("e"), []int{1, 101, 300}},
		{"two matches share their ancestors", byCommand("c", "d"), []int{1, 100, 200, 201}},
		{"a root has no ancestors", byCommand("kthreadd"), []int{2}},
		{"an orphan keeps only itself", byCommand("orphan"), []int{500}},
		{"a match above another adds nothing twice", byCommand("a", "c"), []int{1, 100, 200}},
		{"nothing matches", nil, []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pids(WithAncestors(all, tt.subset))
			if !slices.Equal(got, tt.want) {
				t.Errorf("pids = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWithAncestorsSurvivesAParentCycle(t *testing.T) {
	cycle := []Process{
		{PID: 1, PPID: 2, Command: "x"},
		{PID: 2, PPID: 3, Command: "y"},
		{PID: 3, PPID: 1, Command: "z"},
	}
	got := pids(WithAncestors(cycle, cycle[:1]))
	if !slices.Equal(got, []int{1, 2, 3}) {
		t.Errorf("pids = %v, want [1 2 3]", got)
	}
}

func TestWithAncestorsDoesNotTouchItsInputs(t *testing.T) {
	all := forest()
	subset := []Process{all[7]}
	WithAncestors(all, subset)
	if len(subset) != 1 || len(all) != 9 {
		t.Error("inputs must not change")
	}
}
