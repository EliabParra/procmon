package monitor

import (
	"slices"
	"testing"
	"time"
)

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
