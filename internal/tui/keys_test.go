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

func filtered(m *Model, text string) {
	press(m, "/")
	typeText(m, text)
	press(m, "enter")
}

func TestFilterByName(t *testing.T) {
	m := loaded(120, 30, proc(1, "systemd", 1), proc(2, "bash", 5), proc(3, "Bash-login", 9), proc(4, "kthreadd", 2))

	press(m, "/")
	if m.mode != modeFilter {
		t.Fatal("/ must start the filter input")
	}
	typeText(m, "bash")
	if len(m.rows) != 4 {
		t.Error("nothing is filtered until Enter")
	}
	press(m, "enter")

	if m.mode != modeNormal {
		t.Error("Enter must leave the input")
	}
	if got := rowPIDs(m); fmt.Sprint(got) != "[3 2]" {
		t.Errorf("rows = %v, want [3 2] (case-insensitive, by CPU)", got)
	}
	out := screenText(m)
	if !strings.Contains(out, "filter bash (2)") {
		t.Errorf("the header must show the active filter and its matches:\n%s", out)
	}
	if strings.Contains(out, "systemd") {
		t.Error("a process that does not match must not be listed")
	}
}

func TestFilterIgnoresSpacesAroundTheName(t *testing.T) {
	m := loaded(120, 30, proc(1, "systemd", 1), proc(2, "bash", 5))
	filtered(m, "  bash ")
	if got := rowPIDs(m); fmt.Sprint(got) != "[2]" {
		t.Errorf("rows = %v, want [2]", got)
	}
}

func TestFilterTextIsNotTreatedAsHotKeys(t *testing.T) {
	m := loaded(120, 30, proc(1, "queue", 1), proc(2, "mpd", 5))
	press(m, "/")
	cmd := press(m, "q")
	if isQuit(cmd) {
		t.Fatal("q must be text while typing a filter")
	}
	typeText(m, "ueue")
	press(m, "enter")
	if got := rowPIDs(m); fmt.Sprint(got) != "[1]" {
		t.Errorf("rows = %v, want [1]", got)
	}
}

func TestFilterEscapeCancels(t *testing.T) {
	m := loaded(120, 30, proc(1, "systemd", 1), proc(2, "bash", 5))
	filtered(m, "bash")

	press(m, "/")
	typeText(m, "zzz")
	press(m, "esc")

	if m.mode != modeNormal {
		t.Error("Esc must leave the input")
	}
	if got := rowPIDs(m); fmt.Sprint(got) != "[2]" {
		t.Errorf("rows = %v, want the previous filter kept", got)
	}
}

func TestFilterKeepsTheSelectionWhenItStillMatches(t *testing.T) {
	m := loaded(120, 30, proc(1, "bash-a", 1), proc(2, "zsh", 5), proc(3, "bash-b", 9))
	press(m, "end") // bash-a, the last by CPU
	filtered(m, "bash")
	if m.selected() != 1 {
		t.Errorf("selected PID = %d, want 1", m.selected())
	}
}

func TestCtrlCQuitsFromTheInputModes(t *testing.T) {
	for _, start := range []string{"/"} {
		m := loaded(120, 30, proc(1, "bash", 1))
		press(m, start)
		if !isQuit(press(m, "ctrl+c")) {
			t.Errorf("ctrl+c must quit while typing after %q", start)
		}
	}
}

func TestInputLineShowsThePromptAndPlacesTheCursor(t *testing.T) {
	m := loaded(100, 30, proc(1, "bash", 1))
	if m.View().Cursor != nil {
		t.Error("no cursor outside an input")
	}
	press(m, "/")
	typeText(m, "ba")

	lines := screen(m)
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "/ ba") {
		t.Errorf("the bottom line must be the filter prompt, got %q", last)
	}
	if !strings.Contains(last, "enter") || !strings.Contains(last, "esc") {
		t.Errorf("the prompt must say how to finish: %q", last)
	}
	cur := m.View().Cursor
	if cur == nil || cur.X != 4 || cur.Y != 29 {
		t.Errorf("cursor = %+v, want X=4 Y=29 (after \"/ ba\", on the last line)", cur)
	}

	press(m, "esc")
	if m.View().Cursor != nil {
		t.Error("the cursor must go away with the input")
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

func TestLongInputStaysOnOneLine(t *testing.T) {
	for _, size := range [][2]int{{120, 30}, {40, 12}, {10, 3}} {
		for _, start := range []string{"/"} {
			m := loaded(size[0], size[1], proc(1, "bash", 1))
			press(m, start)
			typeText(m, strings.Repeat("abc日", 100))
			assertFits(t, m, size[0], size[1])
			if cur := m.View().Cursor; cur == nil || cur.X >= size[0] || cur.Y != size[1]-1 {
				t.Errorf("%dx%d: cursor %+v is outside the last line", size[0], size[1], cur)
			}
		}
	}
}
