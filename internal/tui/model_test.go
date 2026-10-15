package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestInitRefreshesAndSchedulesATick(t *testing.T) {
	mon := &fakeMonitor{snap: snapshotOf(proc(1, "init", 0))}
	m := New(mon, &fakeControl{})
	m.interval = time.Millisecond // the first tick really waits

	msgs := run(m.Init())

	var refreshed, ticked bool
	for _, msg := range msgs {
		switch msg := msg.(type) {
		case refreshedMsg:
			refreshed = true
			if len(msg.snap.Processes) != 1 {
				t.Errorf("refreshed snapshot has %d processes, want 1", len(msg.snap.Processes))
			}
		case tickMsg:
			ticked = true
			if msg.gen != m.gen {
				t.Errorf("tick generation = %d, want %d", msg.gen, m.gen)
			}
		}
	}
	if !refreshed || !ticked {
		t.Errorf("Init produced refreshed=%v ticked=%v, want both", refreshed, ticked)
	}
	if mon.count() != 1 {
		t.Errorf("Refresh called %d times, want 1", mon.count())
	}
}

func TestTickRefreshesAndReschedules(t *testing.T) {
	mon := &fakeMonitor{snap: snapshotOf(proc(1, "init", 0))}
	m := New(mon, &fakeControl{})
	m.interval = time.Millisecond

	_, cmd := m.Update(tickMsg{gen: m.gen})
	msgs := run(cmd)

	if mon.count() != 1 {
		t.Errorf("Refresh called %d times, want 1", mon.count())
	}
	var next bool
	for _, msg := range msgs {
		if tk, ok := msg.(tickMsg); ok && tk.gen == m.gen {
			next = true
		}
	}
	if !next {
		t.Error("a tick must schedule the next tick")
	}
}

func TestTickDoesNotOverlapRefreshes(t *testing.T) {
	mon := &fakeMonitor{snap: snapshotOf(proc(1, "init", 0))}
	m := New(mon, &fakeControl{})
	m.interval = time.Millisecond

	_, first := m.Update(tickMsg{gen: m.gen})
	_, second := m.Update(tickMsg{gen: m.gen}) // the first refresh is still running
	run(first)
	run(second)

	if mon.count() != 1 {
		t.Errorf("Refresh called %d times while one was in flight, want 1", mon.count())
	}
	m.Update(refreshedMsg{snap: mon.snap}) // it finished
	_, third := m.Update(tickMsg{gen: m.gen})
	run(third)
	if mon.count() != 2 {
		t.Errorf("Refresh called %d times after the first finished, want 2", mon.count())
	}
}

func TestStaleTickIsIgnored(t *testing.T) {
	mon := &fakeMonitor{}
	m := New(mon, &fakeControl{})
	m.gen = 3

	_, cmd := m.Update(tickMsg{gen: 2})

	if cmd != nil {
		t.Error("a tick from an older generation must do nothing")
	}
	if mon.count() != 0 {
		t.Errorf("Refresh called %d times, want 0", mon.count())
	}
}

func TestRefreshErrorShowsInTheStatusLineAndKeepsRunning(t *testing.T) {
	m := loaded(120, 30, proc(1, "init", 0), proc(2, "bash", 0))

	_, cmd := m.Update(refreshedMsg{err: errors.New("monitor: read memory: boom")})

	if isQuit(cmd) {
		t.Error("a refresh error must not quit")
	}
	out := screenText(m)
	if !strings.Contains(out, "monitor: read memory: boom") {
		t.Errorf("the error is not in the status line:\n%s", out)
	}
	if !strings.Contains(out, "bash") {
		t.Error("the last good snapshot must stay on screen")
	}

	m.Update(refreshedMsg{snap: snapshotOf(proc(1, "init", 0))})
	if strings.Contains(screenText(m), "boom") {
		t.Error("the error must clear after a good refresh")
	}
}

func TestQuitKeys(t *testing.T) {
	for _, name := range []string{"q", "ctrl+c"} {
		t.Run(name, func(t *testing.T) {
			m := loaded(120, 30, proc(1, "init", 0))
			if !isQuit(press(m, name)) {
				t.Errorf("%s must return tea.Quit", name)
			}
		})
	}
}

func TestNavigationKeys(t *testing.T) {
	// ByCPU order is PID 1..20 because the CPU falls as the PID grows.
	tests := []struct {
		name string
		keys []string
		want int // PID of the selected row
	}{
		{"starts on the first row", nil, 1},
		{"down", []string{"down"}, 2},
		{"j", []string{"j", "j", "j"}, 4},
		{"up from the first row stays", []string{"up"}, 1},
		{"k", []string{"down", "down", "k"}, 2},
		{"end", []string{"end"}, 20},
		{"down at the end stays", []string{"end", "down"}, 20},
		{"home", []string{"end", "home"}, 1},
		{"pgdown moves a page", []string{"pgdown"}, 1 + 6},
		{"pgup moves a page back", []string{"end", "pgup"}, 20 - 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loaded(100, 15, manyProcs(20)...) // 15 rows tall: 6 process rows
			press(m, tt.keys...)
			if got := m.selected(); got != tt.want {
				t.Errorf("selected PID = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSelectionFollowsThePIDWhenTheOrderChanges(t *testing.T) {
	m := loaded(100, 30, proc(10, "alpha", 50), proc(20, "beta", 30), proc(30, "gamma", 10))
	press(m, "down") // beta
	if m.selected() != 20 {
		t.Fatalf("selected PID = %d, want 20", m.selected())
	}

	// gamma overtakes everybody: beta moves from the second to the third row.
	m.Update(refreshedMsg{snap: snapshotOf(proc(10, "alpha", 50), proc(20, "beta", 30), proc(30, "gamma", 90))})

	if m.selected() != 20 {
		t.Errorf("selected PID = %d after reordering, want 20", m.selected())
	}
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (beta is now third)", m.cursor)
	}
}

func TestSelectionStaysOnTheTopRowUntilTheUserPicksAnother(t *testing.T) {
	// The first refresh has no %CPU yet, so the table starts in PID order.
	m := loaded(100, 30, proc(10, "alpha", 0), proc(20, "beta", 0), proc(30, "gamma", 0))
	if m.selected() != 10 {
		t.Fatalf("selected PID = %d, want 10", m.selected())
	}

	// Real figures arrive and gamma is the busiest: the top row is still the selected one.
	m.Update(refreshedMsg{snap: snapshotOf(proc(10, "alpha", 1), proc(20, "beta", 2), proc(30, "gamma", 90))})
	if m.selected() != 30 || m.cursor != 0 {
		t.Errorf("selected PID = %d at row %d, want PID 30 at row 0", m.selected(), m.cursor)
	}

	press(m, "down") // beta
	m.Update(refreshedMsg{snap: snapshotOf(proc(10, "alpha", 99), proc(20, "beta", 2), proc(30, "gamma", 90))})
	if m.selected() != 20 {
		t.Errorf("selected PID = %d, want 20: a picked process is followed", m.selected())
	}
}

func TestHomeGoesBackToTheTopRow(t *testing.T) {
	m := loaded(100, 30, proc(10, "alpha", 50), proc(20, "beta", 30), proc(30, "gamma", 10))
	press(m, "down", "home")

	m.Update(refreshedMsg{snap: snapshotOf(proc(10, "alpha", 5), proc(20, "beta", 30), proc(30, "gamma", 10))})

	if m.selected() != 20 || m.cursor != 0 {
		t.Errorf("selected PID = %d at row %d, want the new top row (PID 20)", m.selected(), m.cursor)
	}
}

func TestSelectionMovesToANeighbourWhenTheProcessExits(t *testing.T) {
	m := loaded(100, 30, proc(10, "alpha", 50), proc(20, "beta", 30), proc(30, "gamma", 10))
	press(m, "down")

	m.Update(refreshedMsg{snap: snapshotOf(proc(10, "alpha", 50), proc(30, "gamma", 10))})

	if m.selected() != 30 {
		t.Errorf("selected PID = %d, want 30 (the row that took its place)", m.selected())
	}
}

func TestSelectionOfAnEmptyListIsHarmless(t *testing.T) {
	m := loaded(100, 30)
	press(m, "down", "end", "pgdown", "home", "up")
	if m.selected() != 0 {
		t.Errorf("selected PID = %d, want 0 for an empty list", m.selected())
	}
}

func TestScrollKeepsTheSelectedRowVisible(t *testing.T) {
	m := loaded(100, 15, manyProcs(50)...) // 6 process rows

	press(m, "end")
	out := screenText(m)
	if !strings.Contains(out, "cmd-50 ") {
		t.Errorf("the last process is selected but not on screen:\n%s", out)
	}
	if strings.Contains(out, "cmd-1 ") {
		t.Error("the first process must have scrolled out")
	}

	press(m, "home")
	if !strings.Contains(screenText(m), "cmd-1 ") {
		t.Error("the first process must be back on screen after home")
	}
}

func TestResizeRecomputesTheLayout(t *testing.T) {
	m := loaded(120, 35, manyProcs(100)...)
	press(m, "end")

	for _, size := range [][2]int{{80, 24}, {60, 12}, {150, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		lines := screen(m)
		if len(lines) != size[1] {
			t.Errorf("after resize to %v the view has %d lines", size, len(lines))
		}
		if !strings.Contains(screenText(m), "cmd-100 ") {
			t.Errorf("after resize to %v the selected row scrolled out", size)
		}
	}
}
