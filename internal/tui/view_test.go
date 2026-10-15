package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EliabParra/procmon/internal/monitor"
)

func TestViewUsesTheAlternateScreen(t *testing.T) {
	m := loaded(100, 30, proc(1, "init", 0))
	if !m.View().AltScreen {
		t.Error("the view must ask for the alternate screen")
	}
}

// assertFits checks the invariants that keep redraws clean: exactly the height
// of the terminal, no line wider than its width.
func assertFits(t *testing.T, m *Model, w, h int) {
	t.Helper()
	content := m.View().Content
	lines := strings.Split(content, "\n")
	if len(lines) != h {
		t.Errorf("%dx%d: view has %d lines, want %d", w, h, len(lines), h)
	}
	for i, l := range lines {
		if got := lipgloss.Width(l); got > w {
			t.Errorf("%dx%d: line %d is %d columns wide:\n%q", w, h, i, got, l)
		}
	}
}

func TestViewFillsTheTerminalExactly(t *testing.T) {
	odd := proc(3, "wide \x1b[31m日本語日本語日本語 with\ttabs\nand a newline "+strings.Repeat("x", 300), 5)
	odd.User = "very-long-user-name-here"
	states := map[string]func(w, h int) *Model{
		"before the first refresh": func(w, h int) *Model {
			m := New(&fakeMonitor{}, &fakeControl{})
			m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			return m
		},
		"empty list": func(w, h int) *Model { return loaded(w, h) },
		"few processes": func(w, h int) *Model {
			return loaded(w, h, proc(1, "init", 0), proc(2, "kthreadd", 0))
		},
		"many processes": func(w, h int) *Model { return loaded(w, h, manyProcs(500)...) },
		"hostile text":   func(w, h int) *Model { return loaded(w, h, odd, proc(4, "ok", 1)) },
		"refresh error": func(w, h int) *Model {
			m := loaded(w, h, manyProcs(5)...)
			m.Update(refreshedMsg{err: fmt.Errorf("monitor: %s", strings.Repeat("boom ", 40))})
			return m
		},
	}
	sizes := [][2]int{{120, 35}, {80, 24}, {200, 60}, {60, 15}, {40, 12}, {30, 8}, {20, 5}, {10, 3}, {5, 2}, {1, 1}}
	for name, build := range states {
		for _, size := range sizes {
			t.Run(fmt.Sprintf("%s %dx%d", name, size[0], size[1]), func(t *testing.T) {
				m := build(size[0], size[1])
				press(m, "end", "pgdown", "down")
				assertFits(t, m, size[0], size[1])
			})
		}
	}
}

func TestViewIsDeterministic(t *testing.T) {
	m := loaded(100, 30, manyProcs(30)...)
	if a, b := m.View().Content, m.View().Content; a != b {
		t.Error("rendering twice without an update must give the same output")
	}
}

func TestViewBeforeTheFirstWindowSizeIsEmpty(t *testing.T) {
	m := New(&fakeMonitor{}, &fakeControl{})
	if got := m.View().Content; got != "" {
		t.Errorf("view without a size = %q, want empty", got)
	}
}

func headerSnapshot() monitor.Snapshot {
	snap := snapshotOf(proc(1, "init", 0))
	snap.Summary = monitor.Summary{
		Total: 312, Running: 2, Sleeping: 290, Idle: 12, DiskSleep: 1, Stopped: 3, Zombie: 4,
		Load1: 0.52, Load5: 0.48, Load15: 0.4,
		MemTotalKB: 16 * 1024 * 1024, MemAvailableKB: 12 * 1024 * 1024, MemUsedKB: 4 * 1024 * 1024,
		SwapTotalKB: 8 * 1024 * 1024, SwapUsedKB: 0,
		Uptime: 3*24*time.Hour + 4*time.Hour + 5*time.Minute + 12*time.Second,
	}
	snap.TakenAt = time.Date(2026, 10, 14, 15, 4, 5, 0, time.Local)
	return snap
}

func TestHeaderShowsTheSummary(t *testing.T) {
	m := loaded(120, 35, proc(1, "init", 0))
	m.Update(refreshedMsg{snap: headerSnapshot()})
	out := screenText(m)

	for _, want := range []string{
		"Tasks 312",
		"running 2",
		"sleeping 302", // sleeping + idle kernel threads, like top
		"disk-sleep 1",
		"stopped 3",
		"zombie 4",
		"Load 0.52 0.48 0.40",
		"up 3d 4:05",
		"Mem",
		"4.0G / 16.0G",
		"avail 12.0G",
		"Swap",
		"0K / 8.0G",
		"refresh 1s",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("header is missing %q:\n%s", want, out)
		}
	}
}

func TestClockUsesTwelveHours(t *testing.T) {
	m := loaded(120, 35, proc(1, "init", 0))
	m.Update(refreshedMsg{snap: headerSnapshot()})
	out := screenText(m)

	if !strings.Contains(out, "3:04:05 PM") {
		t.Errorf("the refresh time must read 3:04:05 PM:\n%s", out)
	}
	if strings.Contains(out, "15:04") {
		t.Error("24-hour time must never be shown")
	}
}

func TestHeaderBeforeTheFirstRefresh(t *testing.T) {
	m := New(&fakeMonitor{}, &fakeControl{})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(screenText(m), "waiting for the first refresh") {
		t.Error("the header must say it is waiting for data")
	}
}
