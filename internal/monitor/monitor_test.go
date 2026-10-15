package monitor

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/EliabParra/procmon/internal/oserr"
	"github.com/EliabParra/procmon/internal/procfs"
)

func near(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

func mustRefresh(t *testing.T, m *Monitor) Snapshot {
	t.Helper()
	snap, err := m.Refresh()
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	return snap
}

func TestRefreshFillsTheProcessRow(t *testing.T) {
	src := fourCoreMachine().add(proc{
		pid: 100, ppid: 1, comm: "fish", state: 'S',
		utime: 250, stime: 50, start: 5000,
		uid: 1000, rssKB: 250, cmdline: "/usr/bin/fish --login",
		threads: 3, priority: 25, nice: 5,
	})
	snap := mustRefresh(t, newTestMonitor(t, src))

	want := Process{
		PID: 100, PPID: 1, User: "eliab", State: 'S',
		CPUPercent: 0, MemPercent: 25, RSSKB: 250,
		Threads: 3, Priority: 25, Nice: 5,
		CPUTime: 3 * time.Second, // (250+50) ticks at 100 Hz
		Command: "/usr/bin/fish --login", Comm: "fish", StartTime: 5000,
	}
	if got := byPID(t, snap, 100); got != want {
		t.Fatalf("row = %+v\nwant  %+v", got, want)
	}
	if snap.Cores != 4 {
		t.Fatalf("Cores = %d, want 4", snap.Cores)
	}
	if want := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC); !snap.TakenAt.Equal(want) {
		t.Fatalf("TakenAt = %v, want %v", snap.TakenAt, want)
	}
}

func TestRefreshProcessesAreSortedByPID(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 300}).add(proc{pid: 7}).add(proc{pid: 42})
	snap := mustRefresh(t, newTestMonitor(t, src))
	if got, want := pids(snap.Processes), []int{7, 42, 300}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestRefreshCommandFallsBackToComm(t *testing.T) {
	src := fourCoreMachine().
		add(proc{pid: 2, comm: "kthreadd", cmdline: ""}).
		add(proc{pid: 3, comm: "bash", cmdline: "bash -l"})
	snap := mustRefresh(t, newTestMonitor(t, src))
	if got := byPID(t, snap, 2).Command; got != "[kthreadd]" {
		t.Fatalf("kernel thread Command = %q, want [kthreadd]", got)
	}
	if got := byPID(t, snap, 3).Command; got != "bash -l" {
		t.Fatalf("Command = %q, want bash -l", got)
	}
}

func TestRefreshFirstSampleHasNoCPUPercent(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 10, utime: 9000}).add(proc{pid: 20, utime: 1})
	src.tick(123456)
	snap := mustRefresh(t, newTestMonitor(t, src))
	if len(snap.Processes) != 2 {
		t.Fatalf("processes = %v, want 2", pids(snap.Processes))
	}
	for _, p := range snap.Processes {
		if p.CPUPercent != 0 {
			t.Fatalf("pid %d CPUPercent = %v on the first refresh, want 0", p.PID, p.CPUPercent)
		}
	}
}

func TestRefreshComputesCPUPercentFromDeltas(t *testing.T) {
	src := fourCoreMachine().
		add(proc{pid: 10, utime: 1000}).
		add(proc{pid: 20, utime: 500}).
		add(proc{pid: 30, utime: 70})
	src.tick(5000)
	m := newTestMonitor(t, src)
	mustRefresh(t, m)

	// 400 jiffies pass over 4 cores = 100 ticks of wall time.
	src.tick(400)
	src.burn(10, 100) // a whole core for the whole interval
	src.burn(20, 50)  // half of one core
	snap := mustRefresh(t, m)

	// The spec formula: the process delta over the system delta, so one core of
	// four is a quarter of the machine.
	near(t, "pid 10 %CPU", byPID(t, snap, 10).CPUPercent, 25)
	near(t, "pid 20 %CPU", byPID(t, snap, 20).CPUPercent, 12.5)
	near(t, "pid 30 %CPU", byPID(t, snap, 30).CPUPercent, 0)

	// Coherence with the system: the processes cannot use more than the whole machine.
	total := 0.0
	for _, p := range snap.Processes {
		total += p.CPUPercent
	}
	if total > 100 {
		t.Fatalf("sum of %%CPU = %v exceeds 100", total)
	}
}

func TestRefreshNewProcessStartsAtZero(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 10, utime: 100})
	src.tick(1000)
	m := newTestMonitor(t, src)
	mustRefresh(t, m)

	src.add(proc{pid: 40, utime: 999, start: 7000}) // appeared since the last refresh
	src.tick(400)
	snap := mustRefresh(t, m)
	near(t, "new process %CPU", byPID(t, snap, 40).CPUPercent, 0)
}

func TestRefreshRecycledPIDIsNeverSubtracted(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 10, utime: 1000, start: 1000})
	src.tick(1000)
	m := newTestMonitor(t, src)
	mustRefresh(t, m)

	tests := []struct {
		name  string
		start uint64
		utime uint64
		want  float64
	}{
		{"new process with more ticks than the old one", 2000, 5000, 0},
		{"new process with fewer ticks than the old one", 3000, 10, 0},
		{"same process keeps being measured", 3000, 110, 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src.stats[10] = procfs.ProcStat{PID: 10, State: 'S', StartTime: tt.start, UTime: tt.utime}
			src.tick(400)
			snap := mustRefresh(t, m)
			near(t, "%CPU", byPID(t, snap, 10).CPUPercent, tt.want)
		})
	}
}

func TestRefreshCountersGoingBackwardsGiveZero(t *testing.T) {
	t.Run("process counter", func(t *testing.T) {
		src := fourCoreMachine().add(proc{pid: 10, utime: 500})
		src.tick(1000)
		m := newTestMonitor(t, src)
		mustRefresh(t, m)

		src.stats[10] = procfs.ProcStat{PID: 10, State: 'S', StartTime: 1000, UTime: 100}
		src.tick(400)
		near(t, "%CPU", byPID(t, mustRefresh(t, m), 10).CPUPercent, 0)
	})
	t.Run("system counter", func(t *testing.T) {
		src := fourCoreMachine().add(proc{pid: 10, utime: 500})
		src.tick(1000)
		m := newTestMonitor(t, src)
		mustRefresh(t, m)

		src.burn(10, 50)
		src.sys.CPU.User = 10
		near(t, "%CPU", byPID(t, mustRefresh(t, m), 10).CPUPercent, 0)
	})
}

func TestRefreshUsesOneCoreWhenNoneAreListed(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 10, utime: 0})
	src.sys.Cores = nil
	src.tick(1000)
	m := newTestMonitor(t, src)
	mustRefresh(t, m)

	src.tick(100)
	src.burn(10, 50)
	snap := mustRefresh(t, m)
	if snap.Cores != 1 {
		t.Fatalf("Cores = %d, want the fallback 1", snap.Cores)
	}
	near(t, "%CPU", byPID(t, snap, 10).CPUPercent, 50)
}

func TestRefreshSkipsProcessesThatExit(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeSource)
	}{
		{"gone before stat", func(f *fakeSource) { f.statErr[10] = oserr.ErrProcessGone }},
		{"gone before status", func(f *fakeSource) { f.statusErr[10] = oserr.ErrProcessGone }},
		{"gone before cmdline", func(f *fakeSource) { f.cmdlineErr[10] = oserr.ErrProcessGone }},
		{"listed but already removed", func(f *fakeSource) {
			delete(f.stats, 10)
			delete(f.statuses, 10)
			delete(f.cmdlines, 10)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := fourCoreMachine().add(proc{pid: 10}).add(proc{pid: 20})
			tt.mutate(src)
			snap, err := newTestMonitor(t, src).Refresh()
			if err != nil {
				t.Fatalf("Refresh() error = %v, want none", err)
			}
			if got, want := pids(snap.Processes), []int{20}; !slices.Equal(got, want) {
				t.Fatalf("processes = %v, want %v", got, want)
			}
		})
	}
}

func TestRefreshSkipsAProcessWhoseStatIsUnreadable(t *testing.T) {
	for name, cause := range map[string]error{
		"permission denied": oserr.ErrPermission,
		"malformed":         fmt.Errorf("procfs: pid 10: malformed stat"),
	} {
		t.Run(name, func(t *testing.T) {
			src := fourCoreMachine().add(proc{pid: 10}).add(proc{pid: 20})
			src.statErr[10] = cause
			snap, err := newTestMonitor(t, src).Refresh()
			if err != nil {
				t.Fatalf("Refresh() error = %v, want none", err)
			}
			if got, want := pids(snap.Processes), []int{20}; !slices.Equal(got, want) {
				t.Fatalf("processes = %v, want %v", got, want)
			}
		})
	}
}

func TestRefreshKeepsTheRowWhenDetailsCannotBeRead(t *testing.T) {
	denied := oserr.ErrPermission
	broken := errors.New("procfs: malformed status")
	tests := []struct {
		name        string
		setup       func(*fakeSource)
		wantUser    string
		wantRSS     uint64
		wantCommand string
	}{
		{"status permission denied", func(f *fakeSource) { f.statusErr[10] = denied }, "?", 0, "sleep 100"},
		{"status malformed", func(f *fakeSource) { f.statusErr[10] = broken }, "?", 0, "sleep 100"},
		{"cmdline permission denied", func(f *fakeSource) { f.cmdlineErr[10] = denied }, "root", 500, "[sleep]"},
		{"cmdline malformed", func(f *fakeSource) { f.cmdlineErr[10] = broken }, "root", 500, "[sleep]"},
		{"both denied", func(f *fakeSource) { f.statusErr[10], f.cmdlineErr[10] = denied, denied }, "?", 0, "[sleep]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := fourCoreMachine().add(proc{pid: 10, comm: "sleep", uid: 0, rssKB: 500, cmdline: "sleep 100", utime: 30})
			tt.setup(src)
			snap, err := newTestMonitor(t, src).Refresh()
			if err != nil {
				t.Fatalf("Refresh() error = %v, want none", err)
			}
			got := byPID(t, snap, 10)
			if got.User != tt.wantUser || got.RSSKB != tt.wantRSS || got.Command != tt.wantCommand {
				t.Fatalf("user=%q rss=%d command=%q, want user=%q rss=%d command=%q",
					got.User, got.RSSKB, got.Command, tt.wantUser, tt.wantRSS, tt.wantCommand)
			}
			if got.CPUTime != 300*time.Millisecond || got.State != 'S' {
				t.Fatalf("what Stat gave was lost: CPUTime=%v State=%c", got.CPUTime, got.State)
			}
			if got.MemPercent != float64(tt.wantRSS)/10 { // total memory is 1000 kB
				t.Fatalf("MemPercent = %v for RSS %d", got.MemPercent, tt.wantRSS)
			}
		})
	}
}
