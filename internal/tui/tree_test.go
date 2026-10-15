package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/EliabParra/procmon/internal/monitor"
)

// family is a small hierarchy, with the CPU use of each process:
//
//	1 init (0)
//	├─ 100 shell (1)
//	│  ├─ 200 editor (5)
//	│  └─ 201 sleeper (2)
//	└─ 101 daemon (50)
//	2 kthreadd (3)
//	└─ 3 worker (9)
func family() []monitor.Process {
	child := func(pid, ppid int, name string, cpu float64) monitor.Process {
		p := proc(pid, name, cpu)
		p.PPID = ppid
		return p
	}
	return []monitor.Process{
		child(1, 0, "init", 0),
		child(2, 0, "kthreadd", 3),
		child(3, 2, "worker", 9),
		child(100, 1, "shell", 1),
		child(101, 1, "daemon", 50),
		child(200, 100, "editor", 5),
		child(201, 100, "sleeper", 2),
	}
}

// treeLines are the COMMAND cells of the table rows on screen, top to bottom:
// "├─ shell". The title line tells where that column starts.
func treeLines(m *Model) []string {
	var out []string
	start := -1
	for _, l := range screen(m) {
		cells := []rune(l) // columns are counted in characters, not bytes
		if start < 0 {
			if before, _, found := strings.Cut(l, "COMMAND"); found {
				start = len([]rune(before))
			}
			continue
		}
		if strings.HasPrefix(l, "╰") {
			break
		}
		if cmd := strings.TrimRight(string(cells[min(start, len(cells)):]), " │"); cmd != "" {
			out = append(out, cmd)
		}
	}
	return out
}

func TestTKeyTogglesTheTreeView(t *testing.T) {
	m := loaded(120, 30, family()...)
	if strings.Contains(screenText(m), "├─") {
		t.Fatal("the table starts flat")
	}
	if !strings.Contains(screenText(m), "view table") {
		t.Errorf("the header must say which view is shown:\n%s", screenText(m))
	}

	press(m, "t")
	out := screenText(m)
	if !strings.Contains(out, "├─ ") || !strings.Contains(out, "└─ ") {
		t.Errorf("the tree view must draw connectors:\n%s", out)
	}
	if !strings.Contains(out, "view tree") {
		t.Errorf("the header must say the view is the tree:\n%s", out)
	}

	press(m, "t")
	if strings.Contains(screenText(m), "├─") || !strings.Contains(screenText(m), "view table") {
		t.Errorf("t again must go back to the flat table:\n%s", screenText(m))
	}
}

func TestTreeShowsParentsBeforeChildren(t *testing.T) {
	m := loaded(120, 30, family()...)
	press(m, "p") // by PID, so the order is easy to read
	press(m, "t")

	want := []string{
		"init",
		"├─ shell",
		"│  ├─ editor",
		"│  └─ sleeper",
		"└─ daemon",
		"kthreadd",
		"└─ worker",
	}
	if got := treeLines(m); !slices.Equal(got, want) {
		t.Errorf("tree =\n%q\nwant\n%q", got, want)
	}
}

func TestTreeFollowsTheActiveSort(t *testing.T) {
	m := loaded(120, 30, family()...)
	press(m, "t") // sorted by CPU, the default

	// Siblings go by CPU, highest first: daemon (50) before shell (1), editor
	// (5) before sleeper (2); and so do the roots: kthreadd (3) before init (0).
	want := []string{
		"kthreadd",
		"└─ worker",
		"init",
		"├─ daemon",
		"└─ shell",
		"   ├─ editor",
		"   └─ sleeper",
	}
	if got := treeLines(m); !slices.Equal(got, want) {
		t.Errorf("tree by CPU =\n%q\nwant\n%q", got, want)
	}

	press(m, "p")
	if got := treeLines(m); got[0] != "init" {
		t.Errorf("sorting by PID must reorder the tree: %q", got)
	}
}

func TestTreeFilterKeepsTheAncestorsOfEveryMatch(t *testing.T) {
	m := loaded(120, 30, family()...)
	press(m, "t", "/")
	typeText(m, "sleeper")
	press(m, "enter")

	want := []string{"init", "└─ shell", "   └─ sleeper"}
	if got := treeLines(m); !slices.Equal(got, want) {
		t.Errorf("filtered tree =\n%q\nwant\n%q", got, want)
	}
	if out := screenText(m); !strings.Contains(out, "sleeper (1)") {
		t.Errorf("the filter pill counts the matches, not their ancestors:\n%s", out)
	}

	// Ancestors are context: dimmed. The match itself is not.
	dim := sgrFg(colDim)
	if line := rawLine(t, m, "└─ shell"); !strings.Contains(line, dim) {
		t.Errorf("an ancestor that does not match must be dimmed: %q", line)
	}
	if line := rawLine(t, m, "└─ sleeper"); strings.Contains(line, dim) {
		t.Errorf("the match must not be dimmed: %q", line)
	}

	press(m, "t") // the flat table lists only the match
	if got := treeLines(m); !slices.Equal(got, []string{"sleeper"}) {
		t.Errorf("filtered table = %q, want only the match", got)
	}
}

func TestTreeSelectionAndScrolling(t *testing.T) {
	m := loaded(100, 14, family()...) // 5 process rows: the tree has 7
	press(m, "p", "t")

	press(m, "down", "down")
	if m.selected() != 200 {
		t.Errorf("selected PID = %d, want 200 (editor, third in tree order)", m.selected())
	}
	press(m, "end")
	if m.selected() != 3 {
		t.Errorf("selected PID = %d, want 3 (worker, the last row of the tree)", m.selected())
	}
	if !strings.Contains(screenText(m), "└─ worker") {
		t.Errorf("the last row must have scrolled into view:\n%s", screenText(m))
	}
}

func TestToggleKeepsThePickedProcessSelected(t *testing.T) {
	m := loaded(120, 30, family()...)
	press(m, "p", "down", "down", "down") // 100: shell
	if m.selected() != 100 {
		t.Fatalf("selected PID = %d, want 100", m.selected())
	}

	press(m, "t")
	if m.selected() != 100 {
		t.Errorf("selected PID = %d after the tree opened, want 100", m.selected())
	}
	press(m, "t")
	if m.selected() != 100 {
		t.Errorf("selected PID = %d after going back, want 100", m.selected())
	}
}

func TestTreeUpdatesWithEveryRefresh(t *testing.T) {
	m := loaded(120, 30, family()...)
	press(m, "p", "t")

	grown := family()
	extra := proc(300, "newcomer", 0)
	extra.PPID = 201
	m.Update(refreshedMsg{snap: snapshotOf(append(grown, extra)...)})

	// Under sleeper, the last child of shell, which is not the last child of init.
	if got := treeLines(m); !slices.Contains(got, "│     └─ newcomer") {
		t.Errorf("a new child must appear under its parent:\n%q", got)
	}
}

func TestTreeTextIsCleanedAndFits(t *testing.T) {
	procs := family()
	procs[5].Command = "evil\x1b[2J\tcommand " + strings.Repeat("x", 200)
	for _, size := range [][2]int{{120, 35}, {80, 24}, {40, 12}, {30, 8}, {20, 5}, {10, 3}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := loaded(size[0], size[1], procs...)
			press(m, "t", "end", "down")
			assertFits(t, m, size[0], size[1])
			if strings.ContainsRune(screenText(m), '\x1b') {
				t.Error("an escape byte reached the screen")
			}
		})
	}
}

func TestDeepTreeStaysInsideTheCommandColumn(t *testing.T) {
	var procs []monitor.Process
	for i := 1; i <= 30; i++ { // a chain: every process is the child of the one before
		p := proc(i, fmt.Sprintf("level-%d", i), 0)
		p.PPID = i - 1
		procs = append(procs, p)
	}
	m := loaded(60, 20, procs...)
	press(m, "t", "end")
	assertFits(t, m, 60, 20)
}
