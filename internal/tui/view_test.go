package tui

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

func TestTableShowsEveryColumnWhenThereIsRoom(t *testing.T) {
	m := loaded(140, 30, proc(1234, "/usr/bin/foo --bar", 12.34))
	header := columnHeaderLine(t, m)
	for _, title := range []string{"PID", "PPID", "USER", "S", "%CPU", "%MEM", "RES", "THR", "PRI", "NI", "TIME", "COMMAND"} {
		if !hasColumn(header, title) {
			t.Errorf("column %q is missing: %q", title, header)
		}
	}
}

func TestTableRowFormatsEachField(t *testing.T) {
	p := monitor.Process{
		PID: 1234, PPID: 1, User: "eliab", State: 'R', CPUPercent: 12.34, MemPercent: 5.67,
		RSSKB: 1536 * 1024, Threads: 12, Priority: 20, Nice: 0,
		CPUTime: 83*time.Second + 450*time.Millisecond, Command: "/usr/bin/foo --bar", Comm: "foo",
	}
	m := loaded(140, 30, p)
	want := regexp.MustCompile(`1234\s+1\s+eliab\s+R\s+12\.3\s+5\.7\s+1\.5G\s+12\s+20\s+0\s+1:23\.45\s+/usr/bin/foo --bar`)
	if out := screenText(m); !want.MatchString(out) {
		t.Errorf("the row does not read pid ppid user state cpu mem res thr pri ni time command:\n%s", out)
	}
}

func TestNarrowTerminalDropsColumnsButKeepsTheCommand(t *testing.T) {
	cmd := "/usr/bin/some-long-command --with --arguments"
	tests := []struct {
		width   int
		dropped []string
		kept    []string
	}{
		{width: 140, dropped: nil, kept: []string{"PPID", "USER", "THR", "PRI", "TIME", "RES"}},
		{width: 80, dropped: []string{"PPID"}, kept: []string{"PID", "%CPU", "%MEM", "NI", "COMMAND"}},
		{width: 50, dropped: []string{"PPID", "THR", "PRI", "USER"}, kept: []string{"PID", "S", "%CPU", "NI", "COMMAND"}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.width), func(t *testing.T) {
			m := loaded(tt.width, 24, proc(1234, cmd, 3))
			assertFits(t, m, tt.width, 24)
			header := columnHeaderLine(t, m)
			for _, d := range tt.dropped {
				if hasColumn(header, d) {
					t.Errorf("column %s should be dropped at width %d: %q", d, tt.width, header)
				}
			}
			for _, k := range tt.kept {
				if !hasColumn(header, k) {
					t.Errorf("column %s should stay at width %d: %q", k, tt.width, header)
				}
			}
		})
	}
}

// hasColumn reports whether line, a table header, has a column called title.
func hasColumn(line, title string) bool {
	clear := strings.NewReplacer("│", " ", "▲", " ", "▼", " ").Replace(line)
	return slices.Contains(strings.Fields(clear), title)
}

// columnHeaderLine returns the line of the table that holds the column titles.
func columnHeaderLine(t *testing.T, m *Model) string {
	t.Helper()
	for _, l := range screen(m) {
		if strings.Contains(l, "COMMAND") {
			return l
		}
	}
	t.Fatalf("no column header on screen:\n%s", screenText(m))
	return ""
}

func TestCommandIsTruncatedWithAnEllipsis(t *testing.T) {
	m := loaded(80, 24, proc(1, "/usr/bin/"+strings.Repeat("very-long-", 20)+"tail-marker", 3))
	out := screenText(m)
	if !strings.Contains(out, "…") {
		t.Errorf("a long command must end in an ellipsis:\n%s", out)
	}
	if strings.Contains(out, "tail-marker") {
		t.Error("the end of the command must be cut")
	}
}

func TestRealTimePriorityIsShownAsRT(t *testing.T) {
	p := proc(7, "migration/0", 0)
	p.Priority = -100
	out := screenText(loaded(140, 30, p))
	if !regexp.MustCompile(`\brt\b`).MatchString(out) {
		t.Errorf("a real-time priority must read rt:\n%s", out)
	}
}

func TestSelectedRowIsHighlighted(t *testing.T) {
	m := loaded(120, 30, proc(10, "selected-one", 50), proc(20, "other-one", 10))
	raw := strings.Split(m.View().Content, "\n")

	var selected, other string
	for _, l := range raw {
		switch {
		case strings.Contains(l, "selected-one"):
			selected = l
		case strings.Contains(l, "other-one"):
			other = l
		}
	}
	if selected == "" || other == "" {
		t.Fatal("rows not found on screen")
	}
	if !strings.Contains(selected, "\x1b[") {
		t.Error("the selected row must be styled")
	}
	if strings.Contains(other, "48;2;") {
		t.Error("only the selected row carries a background")
	}
	press(m, "down")
	for _, l := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(l, "other-one") && !strings.Contains(l, "48;2;") {
			t.Error("the highlight must move with the selection")
		}
	}
}

func TestEmptyTableSaysSo(t *testing.T) {
	m := loaded(100, 30)
	if !strings.Contains(screenText(m), "no processes") {
		t.Error("an empty table must say there is nothing to show")
	}
}

// Any process can put anything in its command line. What reaches the terminal
// must be exactly what a harmless command with '?' in place of each control
// character would produce, so no escape sequence is ever interpreted.
func TestHostileTextCannotReachTheTerminal(t *testing.T) {
	hostile := proc(2, "evil\x1b[31m\tred\x1b[2J\x07bell\n\xff", 1)
	hostile.User = "u\x1b[2Jser"
	harmless := proc(2, "evil?[31m?red?[2J?bell??", 1)
	harmless.User = "u?[2Jser"

	first := proc(1, "first", 50) // the selected row, so the hostile one is plain
	got := loaded(120, 30, first, hostile)
	want := loaded(120, 30, first, harmless)

	if got.View().Content != want.View().Content {
		t.Errorf("hostile text changed the output:\n%s\nwant\n%s", got.View().Content, want.View().Content)
	}
	for _, l := range strings.Split(got.View().Content, "\n") {
		if strings.Contains(l, "evil") {
			plain := ansi.Strip(l)
			if strings.ContainsRune(plain, '\x1b') {
				t.Errorf("an ESC byte reached the row: %q", l)
			}
			if !strings.Contains(plain, "evil?[31m?red?[2J?bell??") {
				t.Errorf("the control characters must show as '?': %q", plain)
			}
			if w := lipgloss.Width(l); w != 120 {
				t.Errorf("the row is %d columns wide, want 120", w)
			}
		}
	}
	assertFits(t, got, 120, 30)
}
