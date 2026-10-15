// Package tui is the screen of the process monitor: a Bubble Tea model that
// lays out the summary header, the process table and the help line, and turns
// key presses into calls on the monitor and control ports. It never reads
// /proc or sends signals itself.
package tui

import "github.com/EliabParra/procmon/internal/monitor"

// Monitor is what the screen needs from the logic layer. *monitor.Monitor
// satisfies it; tests plug in a fake.
type Monitor interface {
	Refresh() (monitor.Snapshot, error)
	History(pid int) []float64
	Threads(pid int) ([]monitor.Thread, error)
	// Verify re-reads pid and fails with oserr.ErrProcessGone unless it is
	// still the process that started at startTime.
	Verify(pid int, startTime uint64) error
}

// Control is what the screen needs from the act door: signals and priority.
// The functions of package control satisfy it through a thin adapter.
type Control interface {
	Terminate(pid int) error
	Kill(pid int) error
	Suspend(pid int) error
	Resume(pid int) error
	Priority(pid int) (int, error)
	SetPriority(pid, nice int) error
}

var _ Monitor = (*monitor.Monitor)(nil)
