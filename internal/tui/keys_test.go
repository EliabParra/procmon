package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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

func TestFilterEmptyClears(t *testing.T) {
	m := loaded(120, 30, proc(1, "systemd", 1), proc(2, "bash", 5))
	filtered(m, "bash")

	press(m, "/")
	press(m, "backspace", "backspace", "backspace", "backspace")
	press(m, "enter")

	if len(m.rows) != 2 {
		t.Errorf("an empty filter must show everything, got %v", rowPIDs(m))
	}
	if strings.Contains(screenText(m), "filter bash") {
		t.Error("the filter pill must disappear when the filter is empty")
	}
}

func TestFilterInputStartsFromTheCurrentFilter(t *testing.T) {
	m := loaded(120, 30, proc(1, "systemd", 1), proc(2, "bash", 5), proc(3, "basic", 3))
	filtered(m, "bash")

	press(m, "/", "backspace", "backspace") // bash -> ba
	press(m, "enter")

	if got := rowPIDs(m); fmt.Sprint(got) != "[2 3]" {
		t.Errorf("rows = %v, want [2 3]", got)
	}
}

func TestFilterWithoutMatches(t *testing.T) {
	m := loaded(120, 30, proc(1, "systemd", 1), proc(2, "bash", 5))
	filtered(m, "nothing")

	if !strings.Contains(screenText(m), `no processes match "nothing"`) {
		t.Errorf("the table must explain why it is empty:\n%s", screenText(m))
	}
	if m.selected() != 0 {
		t.Errorf("selected PID = %d, want 0", m.selected())
	}
	press(m, "down", "end") // must not panic
	press(m, "/", "ctrl+u", "enter")
	if m.selected() == 0 {
		t.Error("clearing the filter must select a row again")
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

func TestFilterBackspaceAndClearLine(t *testing.T) {
	m := loaded(120, 30, proc(1, "abc", 1))
	press(m, "/")
	typeText(m, "abcd")
	press(m, "backspace")
	if m.input.String() != "abc" {
		t.Errorf("input = %q, want abc", m.input.String())
	}
	press(m, "ctrl+u")
	if m.input.String() != "" {
		t.Errorf("ctrl+u must clear the line, got %q", m.input.String())
	}
	press(m, "backspace") // on an empty line: harmless
}

func TestFilterTypingMultibyteText(t *testing.T) {
	m := loaded(120, 30, proc(1, "日本語", 1), proc(2, "bash", 1))
	press(m, "/")
	typeText(m, "日本語")
	press(m, "backspace")
	if m.input.String() != "日本" {
		t.Errorf("backspace must remove a whole rune, got %q", m.input.String())
	}
}

func TestFilterPasteIsSingleLineAndClean(t *testing.T) {
	m := loaded(120, 30, proc(1, "bash", 1))
	press(m, "/")
	m.Update(tea.PasteMsg{Content: "ba\x1b[2Jsh\n"})
	if got := m.input.String(); got != "ba?[2Jsh" {
		t.Errorf("pasted text = %q", got)
	}
	m2 := loaded(120, 30, proc(1, "bash", 1))
	m2.Update(tea.PasteMsg{Content: "x"}) // outside an input: ignored
	if m2.input.String() != "" {
		t.Error("paste outside an input must be ignored")
	}
}

func TestCtrlCQuitsFromTheInputModes(t *testing.T) {
	for _, start := range []string{"/", "i"} {
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

func TestIntervalSteps(t *testing.T) {
	m := loaded(120, 30, proc(1, "bash", 1))
	steps := []struct {
		key  string
		want time.Duration
	}{
		{"+", 2 * time.Second}, {"+", 3 * time.Second}, {"+", 5 * time.Second}, {"+", 10 * time.Second},
		{"+", 10 * time.Second}, // already the longest step
		{"-", 5 * time.Second}, {"-", 3 * time.Second}, {"-", 2 * time.Second}, {"-", time.Second},
		{"-", 500 * time.Millisecond},
		{"-", 500 * time.Millisecond}, // already the shortest step
	}
	for _, s := range steps {
		press(m, s.key)
		if m.interval != s.want {
			t.Fatalf("after %q the interval is %v, want %v", s.key, m.interval, s.want)
		}
	}
	if !strings.Contains(screenText(m), "refresh 0.5s") {
		t.Error("the header must show the interval")
	}
}

func TestIntervalStepsFromACustomValue(t *testing.T) {
	m := loaded(120, 30, proc(1, "bash", 1))
	m.interval = 2500 * time.Millisecond
	press(m, "+")
	if m.interval != 3*time.Second {
		t.Errorf("+ from 2.5s = %v, want 3s", m.interval)
	}
	m.interval = 2500 * time.Millisecond
	press(m, "-")
	if m.interval != 2*time.Second {
		t.Errorf("- from 2.5s = %v, want 2s", m.interval)
	}
	m.interval = 30 * time.Second
	press(m, "-")
	if m.interval != 10*time.Second {
		t.Errorf("- from 30s = %v, want 10s", m.interval)
	}
}

func TestChangingTheIntervalRestartsTheTickChain(t *testing.T) {
	mon := &fakeMonitor{snap: snapshotOf(proc(1, "bash", 1))}
	m := New(mon, &fakeControl{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	old := m.gen

	cmd := press(m, "+")

	if m.gen == old {
		t.Error("a new interval needs a new tick generation")
	}
	if cmd == nil {
		t.Error("a new interval must schedule its own tick")
	}
	if _, next := m.Update(tickMsg{gen: old}); next != nil {
		t.Error("the tick of the old interval must be ignored")
	}
	if mon.count() != 0 {
		t.Error("an old tick must not refresh")
	}
	if _, next := m.Update(tickMsg{gen: m.gen}); next == nil {
		t.Error("the new tick must keep the loop going")
	}
}

func TestTypedIntervalTakesEffectAndTicks(t *testing.T) {
	m := loaded(120, 30, proc(1, "bash", 1))
	press(m, "i")
	if m.mode != modeInterval {
		t.Fatal("i must start the interval input")
	}
	typeText(m, "0.2")
	cmd := press(m, "enter")

	if m.interval != 200*time.Millisecond {
		t.Errorf("interval = %v, want 200ms", m.interval)
	}
	if m.mode != modeNormal {
		t.Error("Enter must leave the input")
	}
	var ticked bool
	for _, msg := range run(cmd) {
		if tk, ok := msg.(tickMsg); ok && tk.gen == m.gen {
			ticked = true
		}
	}
	if !ticked {
		t.Error("the typed interval must schedule a tick of the new generation")
	}
}

func TestTypedIntervalValues(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration // 0 when invalid
		message string
	}{
		{in: "2", want: 2 * time.Second},
		{in: "0.5", want: 500 * time.Millisecond},
		{in: "1.5s", want: 1500 * time.Millisecond},
		{in: "0,5", want: 500 * time.Millisecond},
		{in: " 3 ", want: 3 * time.Second},
		{in: "60", want: 60 * time.Second},
		{in: "0.2", want: 200 * time.Millisecond},
		{in: "abc", message: "not a number"},
		{in: "", message: "not a number"},
		{in: "0", message: "between"},
		{in: "-1", message: "between"},
		{in: "0.05", message: "between"},
		{in: "61", message: "between"},
		{in: "1e9", message: "between"},
		{in: "NaN", message: "not a number"},
		{in: "Inf", message: "not a number"},
		{in: "1.2.3", message: "not a number"},
		{in: "2 3", message: "not a number"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.in), func(t *testing.T) {
			m := loaded(120, 30, proc(1, "bash", 1))
			press(m, "i")
			typeText(m, tt.in)
			cmd := press(m, "enter") // not run: a tick command waits for the interval

			if m.mode != modeNormal {
				t.Error("Enter must leave the input")
			}
			if tt.want > 0 {
				if m.interval != tt.want {
					t.Errorf("interval = %v, want %v", m.interval, tt.want)
				}
				if cmd == nil {
					t.Error("a new interval must schedule its tick")
				}
				return
			}
			if cmd != nil {
				t.Error("an invalid interval must not start anything, least of all quit")
			}
			if m.interval != DefaultInterval {
				t.Errorf("an invalid interval changed it to %v", m.interval)
			}
			out := screenText(m)
			if !strings.Contains(out, "invalid interval") || !strings.Contains(out, tt.message) {
				t.Errorf("the status line must explain the problem (%q):\n%s", tt.message, out)
			}
			assertFits(t, m, 120, 30)

			press(m, "j") // the next key dismisses the message
			if strings.Contains(screenText(m), "invalid interval") {
				t.Error("the message must go away on the next key")
			}
		})
	}
}

func TestIntervalInputEscapeCancels(t *testing.T) {
	m := loaded(120, 30, proc(1, "bash", 1))
	press(m, "i")
	typeText(m, "5")
	press(m, "esc")
	if m.interval != DefaultInterval || m.mode != modeNormal {
		t.Errorf("Esc must cancel: interval %v, mode %v", m.interval, m.mode)
	}
}

func TestIntervalInputShowsPromptAndCurrentValue(t *testing.T) {
	m := loaded(100, 30, proc(1, "bash", 1))
	press(m, "i")
	typeText(m, "2")
	lines := screen(m)
	last := lines[len(lines)-1]
	if !strings.Contains(last, "seconds") || !strings.Contains(last, "2") {
		t.Errorf("the prompt must ask for seconds: %q", last)
	}
}

func TestNewHonoursTheIntervalOption(t *testing.T) {
	tests := []struct {
		name string
		opt  time.Duration
		want time.Duration
	}{
		{"as given", 2 * time.Second, 2 * time.Second},
		{"too fast is raised", time.Millisecond, MinInterval},
		{"too slow is lowered", time.Hour, MaxInterval},
		{"zero is raised", 0, MinInterval},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(&fakeMonitor{}, &fakeControl{}, WithInterval(tt.opt))
			if m.interval != tt.want {
				t.Errorf("interval = %v, want %v", m.interval, tt.want)
			}
		})
	}
	if New(&fakeMonitor{}, &fakeControl{}).interval != DefaultInterval {
		t.Error("the default interval is 1s")
	}
}

func TestHelpLineListsTheKeysThatWork(t *testing.T) {
	m := loaded(200, 30, proc(1, "init", 0))
	out := screen(m)
	help := out[len(out)-1]
	for _, want := range []string{"q quit", "↑/↓ move", "pgup/pgdn page", "home/end jump", "c cpu", "m mem", "p pid", "/ filter", "+/- interval", "i set interval"} {
		if !strings.Contains(help, want) {
			t.Errorf("help line is missing %q: %q", want, help)
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

func TestLongInputStaysOnOneLine(t *testing.T) {
	for _, size := range [][2]int{{120, 30}, {40, 12}, {10, 3}} {
		for _, start := range []string{"/", "i"} {
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
