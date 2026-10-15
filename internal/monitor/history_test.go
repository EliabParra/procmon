package monitor

import (
	"slices"
	"sync"
	"testing"

	"github.com/EliabParra/procmon/internal/procfs"
)

func TestRing(t *testing.T) {
	tests := []struct {
		name string
		size int
		add  []float64
		want []float64
	}{
		{"empty", 3, nil, []float64{}},
		{"partially filled keeps insertion order", 3, []float64{1, 2}, []float64{1, 2}},
		{"exactly full", 3, []float64{1, 2, 3}, []float64{1, 2, 3}},
		{"overwrites the oldest", 3, []float64{1, 2, 3, 4, 5}, []float64{3, 4, 5}},
		{"wraps more than once", 2, []float64{1, 2, 3, 4, 5, 6, 7}, []float64{6, 7}},
		{"size one", 1, []float64{8, 9}, []float64{9}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRing(tt.size)
			for _, v := range tt.add {
				r.add(v)
			}
			if got := r.values(); !slices.Equal(got, tt.want) {
				t.Fatalf("values() = %v, want %v", got, tt.want)
			}
		})
	}
}

// cpuRun refreshes once per entry of burns, burning that many ticks over an
// interval of 100 system jiffies, which makes the %CPU equal to the ticks. The
// first refresh has no baseline and always records 0.
func cpuRun(t *testing.T, m *Monitor, src *fakeSource, pid int, burns ...uint64) {
	t.Helper()
	for _, b := range burns {
		src.tick(100)
		src.burn(pid, b)
		mustRefresh(t, m)
	}
}

func TestHistoryRecordsOneSamplePerRefresh(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 10})
	m := newTestMonitor(t, src)
	mustRefresh(t, m)
	cpuRun(t, m, src, 10, 100, 50)

	want := []float64{0, 100, 50}
	if got := m.History(10); !slices.Equal(got, want) {
		t.Fatalf("History(10) = %v, want %v", got, want)
	}
}

func TestHistoryKeepsTheLastNSamples(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 10})
	m := newTestMonitor(t, src, WithHistory(3))
	mustRefresh(t, m)
	cpuRun(t, m, src, 10, 10, 20, 30, 40)

	want := []float64{20, 30, 40}
	if got := m.History(10); !slices.Equal(got, want) {
		t.Fatalf("History(10) = %v, want %v", got, want)
	}
}

func TestHistoryDefaultsToSixtySamples(t *testing.T) {
	for _, n := range []int{0, -5} {
		src := fourCoreMachine().add(proc{pid: 10})
		m := newTestMonitor(t, src, WithHistory(n))
		mustRefresh(t, m)
		cpuRun(t, m, src, 10, make([]uint64, 80)...)
		if got := len(m.History(10)); got != 60 {
			t.Fatalf("WithHistory(%d): %d samples kept, want 60", n, got)
		}
	}
}

func TestHistoryStartsOverForARecycledPID(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 10, start: 1000})
	m := newTestMonitor(t, src)
	mustRefresh(t, m)
	cpuRun(t, m, src, 10, 80, 90)

	src.stats[10] = procfs.ProcStat{PID: 10, State: 'S', StartTime: 2000}
	src.tick(400)
	mustRefresh(t, m)

	if got, want := m.History(10), []float64{0}; !slices.Equal(got, want) {
		t.Fatalf("History(10) after the pid was recycled = %v, want %v", got, want)
	}
}

func TestHistoryIsDroppedWhenTheProcessDisappears(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 10}).add(proc{pid: 20})
	m := newTestMonitor(t, src)
	mustRefresh(t, m)
	cpuRun(t, m, src, 10, 70)

	src.remove(10)
	src.tick(400)
	mustRefresh(t, m)
	if got := m.History(10); got != nil {
		t.Fatalf("History(10) after exit = %v, want nil", got)
	}
	if got := m.History(20); len(got) != 3 {
		t.Fatalf("History(20) = %v, want 3 samples", got)
	}

	// Even the same pid and start time, seen again later, is not the old series.
	src.add(proc{pid: 10})
	src.tick(400)
	mustRefresh(t, m)
	if got := m.History(10); len(got) != 1 {
		t.Fatalf("History(10) after coming back = %v, want a fresh series of 1", got)
	}
}

func TestHistoryOfAnUnknownPIDIsNil(t *testing.T) {
	m := newTestMonitor(t, fourCoreMachine().add(proc{pid: 10}))
	mustRefresh(t, m)
	if got := m.History(999); got != nil {
		t.Fatalf("History(999) = %v, want nil", got)
	}
}

func TestHistoryReturnsACopy(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 10})
	m := newTestMonitor(t, src)
	mustRefresh(t, m)
	cpuRun(t, m, src, 10, 40)

	h := m.History(10)
	h[0], h[1] = -1, -1
	if got, want := m.History(10), []float64{0, 40}; !slices.Equal(got, want) {
		t.Fatalf("History(10) = %v after the caller edited its copy, want %v", got, want)
	}
}

func TestFailedRefreshAddsNoSample(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 10})
	m := newTestMonitor(t, src)
	mustRefresh(t, m)

	src.loadErr = errBoom
	if _, err := m.Refresh(); err == nil {
		t.Fatal("expected the refresh to fail")
	}
	if got := len(m.History(10)); got != 1 {
		t.Fatalf("History(10) has %d samples after a failed refresh, want 1", got)
	}
}

func TestMonitorIsSafeForConcurrentUse(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 10}).add(proc{pid: 20})
	src.threads[10] = []procfs.ProcStat{{PID: 10, Comm: "t", State: 'S'}}
	m := newTestMonitor(t, src)

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 50 {
				if _, err := m.Refresh(); err != nil {
					t.Error(err)
					return
				}
				m.History(10)
				if _, err := m.Threads(10); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
}
