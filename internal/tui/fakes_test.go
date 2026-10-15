package tui

import (
	"fmt"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/EliabParra/procmon/internal/monitor"
	"github.com/EliabParra/procmon/internal/oserr"
)

// fakeMonitor serves a canned snapshot and counts how often it was asked.
type fakeMonitor struct {
	mu        sync.Mutex
	snap      monitor.Snapshot
	err       error
	refreshes int

	history     map[int][]float64
	threads     map[int][]monitor.Thread
	threadsErr  error
	threadReads int

	verifyErr error // when set, Verify returns it
	verified  []string
}

func (f *fakeMonitor) Refresh() (monitor.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshes++
	return f.snap, f.err
}

func (f *fakeMonitor) History(pid int) []float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.history[pid]
}

func (f *fakeMonitor) Threads(pid int) ([]monitor.Thread, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.threadReads++
	return f.threads[pid], f.threadsErr
}

// Verify answers like the real one against the snapshot the fake serves: the
// process must be listed with the same start time.
func (f *fakeMonitor) Verify(pid int, startTime uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verified = append(f.verified, fmt.Sprintf("%d@%d", pid, startTime))
	if f.verifyErr != nil {
		return f.verifyErr
	}
	for _, p := range f.snap.Processes {
		if p.PID == pid && p.StartTime == startTime {
			return nil
		}
	}
	return fmt.Errorf("monitor: verify %d: %w", pid, oserr.ErrProcessGone)
}

func (f *fakeMonitor) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.refreshes
}

func (f *fakeMonitor) reads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.threadReads
}

// fakeControl records the calls it receives and answers with canned results.
type fakeControl struct {
	calls   []string
	err     error // returned by the signal calls and SetPriority
	nice    int   // what Priority reports
	niceErr error // returned by Priority
}

func (f *fakeControl) record(what string, pid int) error {
	f.calls = append(f.calls, fmt.Sprintf("%s %d", what, pid))
	return f.err
}
func (f *fakeControl) Terminate(pid int) error { return f.record("terminate", pid) }
func (f *fakeControl) Kill(pid int) error      { return f.record("kill", pid) }
func (f *fakeControl) Suspend(pid int) error   { return f.record("suspend", pid) }
func (f *fakeControl) Resume(pid int) error    { return f.record("resume", pid) }
func (f *fakeControl) Priority(pid int) (int, error) {
	f.calls = append(f.calls, fmt.Sprintf("priority %d", pid))
	return f.nice, f.niceErr
}
func (f *fakeControl) SetPriority(pid, nice int) error {
	return f.record(fmt.Sprintf("setpriority %d", nice), pid)
}

// proc builds a process row; the other fields get plausible defaults.
func proc(pid int, command string, cpu float64) monitor.Process {
	return monitor.Process{
		PID: pid, PPID: 1, User: "eliab", State: 'S',
		CPUPercent: cpu, MemPercent: 1.5, RSSKB: 10 * 1024,
		Threads: 1, Priority: 20, Nice: 0,
		CPUTime: time.Second, Command: command, Comm: command,
	}
}

// snapshotOf wraps rows (sorted by PID, as Refresh returns them) in a snapshot.
func snapshotOf(procs ...monitor.Process) monitor.Snapshot {
	monitor.Sort(procs, monitor.ByPID)
	return monitor.Snapshot{
		Processes: procs,
		Summary:   monitor.Summary{Total: len(procs)},
		Cores:     4,
		TakenAt:   time.Date(2026, 10, 14, 15, 4, 5, 0, time.Local),
	}
}

// loaded returns a model of the given terminal size that already holds procs.
func loaded(w, h int, procs ...monitor.Process) *Model {
	m := New(&fakeMonitor{}, &fakeControl{})
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m.Update(refreshedMsg{snap: snapshotOf(procs...)})
	return m
}

// run executes cmd and every command batched inside it, returning the messages.
// Only use it with short intervals: a tick command really waits.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, run(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}
