package tui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// sgrFg is the part of an escape sequence that switches the foreground to c,
// as the renderer writes it: 38;2;R;G;B. The renderer may join it with other
// attributes, so tests look for it inside the line, not for a whole sequence.
func sgrFg(c color.Color) string { return "38;2;" + rgb(c) }

// sgrBg is the same for a background colour: 48;2;R;G;B.
func sgrBg(c color.Color) string { return "48;2;" + rgb(c) }

func rgb(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("%d;%d;%d", r>>8, g>>8, b>>8)
}

// rawLine returns the styled line of the screen that holds marker.
func rawLine(t *testing.T, m *Model, marker string) string {
	t.Helper()
	for l := range strings.SplitSeq(m.View().Content, "\n") {
		if strings.Contains(l, marker) {
			return l
		}
	}
	t.Fatalf("no line contains %q:\n%s", marker, screenText(m))
	return ""
}

func inState(pid int, command string, state byte) func() (int, string, byte) {
	return func() (int, string, byte) { return pid, command, state }
}

func TestRowsAreHighlightedByState(t *testing.T) {
	zombie := proc(20, "zombie-row", 5)
	zombie.State = 'Z'
	blocked := proc(30, "blocked-row", 4)
	blocked.State = 'D'
	stopped := proc(40, "stopped-row", 3)
	stopped.State = 'T'
	tracing := proc(50, "tracing-row", 2)
	tracing.State = 't'
	calm := proc(60, "calm-row", 1)

	// The first row is the selected one, so every other row shows its own style.
	m := loaded(120, 30, proc(10, "first-row", 90), zombie, blocked, stopped, tracing, calm)

	tests := []struct {
		marker string
		fg     color.Color
	}{
		{"zombie-row", colZombie},
		{"blocked-row", colDiskSleep},
		{"stopped-row", colStopped},
		{"tracing-row", colStopped},
	}
	for _, tt := range tests {
		line := rawLine(t, m, tt.marker)
		if !strings.Contains(line, sgrFg(tt.fg)) {
			t.Errorf("%s must use its state colour: %q", tt.marker, line)
		}
	}

	// The three states look different from each other and from a plain row.
	if colZombie == colDiskSleep || colDiskSleep == colStopped || colZombie == colStopped {
		t.Error("zombie, disk-sleep and stopped need three distinct colours")
	}
	calmLine := rawLine(t, m, "calm-row")
	for _, c := range []color.Color{colZombie, colDiskSleep, colStopped} {
		if strings.Contains(calmLine, sgrFg(c)) || strings.Contains(calmLine, sgrBg(c)) {
			t.Errorf("a running or sleeping row carries no state colour: %q", calmLine)
		}
	}
}

func TestTheStateCellOfAHighlightedRowIsEmphasised(t *testing.T) {
	zombie := proc(20, "zombie-row", 5)
	zombie.State = 'Z'
	m := loaded(120, 30, proc(10, "first-row", 90), zombie)

	line := rawLine(t, m, "zombie-row")
	if !strings.Contains(line, sgrBg(colZombie)) {
		t.Errorf("the state letter must sit on a coloured chip: %q", line)
	}
	// Only that one cell: the rest of the row has no background.
	if n := strings.Count(line, sgrBg(colZombie)); n != 1 {
		t.Errorf("the chip must be a single cell, found %d background runs: %q", n, line)
	}
}

func TestSelectedHighlightedRowKeepsBothLooks(t *testing.T) {
	zombie := proc(20, "zombie-row", 95)
	zombie.State = 'Z'
	m := loaded(120, 30, zombie, proc(10, "other-row", 1))

	line := rawLine(t, m, "zombie-row")
	if !strings.Contains(line, sgrBg(colSelect)) {
		t.Errorf("the selected zombie keeps the selection background: %q", line)
	}
	if !strings.Contains(line, sgrFg(colZombie)) {
		t.Errorf("the selected zombie keeps its colour: %q", line)
	}
}

func TestHighlightDoesNotChangeTheLayout(t *testing.T) {
	zombie := proc(20, "zombie-row --with args", 5)
	zombie.State = 'Z'
	plain := proc(20, "zombie-row --with args", 5)
	m := loaded(120, 30, proc(10, "first-row", 90), zombie)
	n := loaded(120, 30, proc(10, "first-row", 90), plain)

	got := rawLine(t, m, "zombie-row")
	want := rawLine(t, n, "zombie-row")
	if lipgloss.Width(got) != lipgloss.Width(want) {
		t.Errorf("highlighted row is %d columns, plain row %d", lipgloss.Width(got), lipgloss.Width(want))
	}
	assertFits(t, m, 120, 30)
}

func TestHeaderCountsUseTheStateColours(t *testing.T) {
	m := loaded(120, 35, proc(1, "init", 0))
	m.Update(refreshedMsg{snap: headerSnapshot()}) // 1 disk-sleep, 3 stopped, 4 zombie
	line := rawLine(t, m, "zombie")

	for name, c := range map[string]color.Color{"zombie": colZombie, "disk-sleep": colDiskSleep, "stopped": colStopped} {
		if !strings.Contains(line, sgrFg(c)) {
			t.Errorf("the %s count must use its state colour: %q", name, line)
		}
	}

	snap := headerSnapshot()
	snap.Summary.Zombie, snap.Summary.DiskSleep, snap.Summary.Stopped = 0, 0, 0
	m.Update(refreshedMsg{snap: snap})
	line = rawLine(t, m, "zombie")
	for _, c := range []color.Color{colZombie, colDiskSleep, colStopped} {
		if strings.Contains(line, sgrFg(c)) {
			t.Errorf("a zero count carries no alarm colour: %q", line)
		}
	}
}
