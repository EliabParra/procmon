package tui

import (
	"testing"
	"time"
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
