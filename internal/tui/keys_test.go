package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/EliabParra/procmon/internal/monitor"
)

// rowPIDs lists the PIDs of the table in the order shown.
func rowPIDs(m *Model) []int {
	pids := make([]int, len(m.rows))
	for i, p := range m.rows {
		pids[i] = p.PID
	}
	return pids
}

// sortable has CPU order 20,30,10, memory order 10,30,20 and PID order 10,20,30.
func sortable() []monitor.Process {
	a, b, c := proc(10, "alpha", 5), proc(20, "beta", 50), proc(30, "gamma", 20)
	a.MemPercent, b.MemPercent, c.MemPercent = 30, 10, 20
	return []monitor.Process{a, b, c}
}

func TestSortKeys(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want []int
		pill string
	}{
		{"cpu is the default", nil, []int{20, 30, 10}, "sort CPU"},
		{"m sorts by memory", []string{"m"}, []int{10, 30, 20}, "sort MEM"},
		{"p sorts by pid", []string{"p"}, []int{10, 20, 30}, "sort PID"},
		{"c goes back to cpu", []string{"m", "c"}, []int{20, 30, 10}, "sort CPU"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loaded(120, 30, sortable()...)
			press(m, tt.keys...)
			if got := rowPIDs(m); fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("order = %v, want %v", got, tt.want)
			}
			if !strings.Contains(screenText(m), tt.pill) {
				t.Errorf("the header must show %q", tt.pill)
			}
		})
	}
}

func TestSortKeepsTheSelectionOnTheSameProcess(t *testing.T) {
	m := loaded(120, 30, sortable()...)
	press(m, "down") // gamma, second by CPU
	press(m, "m", "p", "c")
	if m.selected() != 30 {
		t.Errorf("selected PID = %d, want 30", m.selected())
	}
	if m.rows[m.cursor].PID != 30 {
		t.Errorf("the cursor is on PID %d, want 30", m.rows[m.cursor].PID)
	}
}

func TestTheSortedColumnShowsAnArrow(t *testing.T) {
	m := loaded(140, 30, sortable()...)
	for keys, want := range map[string]string{"c": "%CPU▼", "m": "%MEM▼", "p": "PID▲"} {
		press(m, keys)
		if !strings.Contains(screenText(m), want) {
			t.Errorf("after %q the header must show %q", keys, want)
		}
	}
}

func TestHelpLineDropsTheLastHintsWhenNarrow(t *testing.T) {
	m := loaded(30, 20, proc(1, "init", 0))
	out := screen(m)
	help := out[len(out)-1]
	if !strings.Contains(help, "q quit") {
		t.Errorf("quit must always be listed: %q", help)
	}
	if strings.Contains(help, "set interval") {
		t.Errorf("the last hints must be dropped to fit: %q", help)
	}
	assertFits(t, m, 30, 20)
}
