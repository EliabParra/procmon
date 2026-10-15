package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/EliabParra/procmon/internal/monitor"
)

// thread builds a thread row.
func thread(tid int, comm string, state byte, cpu time.Duration) monitor.Thread {
	return monitor.Thread{TID: tid, Comm: comm, State: state, CPUTime: cpu}
}

// editorSetup is a monitor with a few processes; PID 200 is the editor, with
// a CPU history and three threads.
func editorSetup(w, h int) (*Model, *fakeMonitor) {
	editor := proc(200, "/usr/bin/editor --flag", 5)
	editor.PPID = 100
	editor.Nice = 5
	editor.Threads = 3
	editor.StartTime = 777
	mon := &fakeMonitor{
		history: map[int][]float64{200: {0, 10, 50, 100, 20}},
		threads: map[int][]monitor.Thread{200: {
			thread(200, "editor", 'S', 1500*time.Millisecond),
			thread(201, "worker", 'R', 90*time.Second),
			thread(202, "io-wait", 'D', 2*time.Second),
		}},
	}
	m := loadedWith(mon, w, h, proc(100, "shell", 1), editor, proc(300, "other", 2))
	return m, mon
}

// openDetail selects the editor and opens its detail view the way the user does.
func openDetail(m *Model) {
	press(m, "p", "down") // by PID: 100, 200, 300
	deliver(m, press(m, "enter"))
}

// sparkRow is the line of the screen that holds only block characters.
func sparkRow(m *Model) string {
	for _, l := range screen(m) {
		inner := strings.Trim(l, "│ ")
		if inner != "" && strings.Trim(inner, string(sparkBlocks)) == "" {
			return inner
		}
	}
	return ""
}

func TestEnterOpensTheDetailOfTheSelectedProcess(t *testing.T) {
	m, _ := editorSetup(120, 35)
	openDetail(m)
	out := screenText(m)

	for _, want := range []string{
		"PID 200", "state S sleeping", "user eliab", "nice 5",
		"/usr/bin/editor --flag", "view detail", "threads 3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the detail is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "COMMAND") || strings.Contains(out, "%MEM") {
		t.Errorf("the table must give way to the detail:\n%s", out)
	}
	assertFits(t, m, 120, 35)
}

func TestEnterOnAnEmptyTableSaysThereIsNothingToOpen(t *testing.T) {
	m := loaded(100, 30)
	press(m, "enter")
	if m.view != viewTable {
		t.Error("there is nothing to open")
	}
	if !strings.Contains(screenText(m), "no process selected") {
		t.Errorf("the status line must say so:\n%s", screenText(m))
	}
}

func TestDetailDrawsTheCPUHistory(t *testing.T) {
	m, mon := editorSetup(120, 35)
	openDetail(m)
	out := screenText(m)

	for _, want := range []string{"CPU history", "min 0.0%", "avg 36.0%", "max 100.0%", "5 samples"} {
		if !strings.Contains(out, want) {
			t.Errorf("the chart is missing %q:\n%s", want, out)
		}
	}
	const inner = 120 - 4
	if got, want := sparkRow(m), sparkline(mon.history[200], inner, 100); got != want {
		t.Errorf("sparkline =\n%q\nwant it to fill the %d columns:\n%q", got, inner, want)
	}
	if !strings.Contains(sparkRow(m), "█") {
		t.Error("the 100% sample must reach the top block")
	}
}

func TestDetailChartWithoutSamples(t *testing.T) {
	m, mon := editorSetup(120, 35)
	mon.history = nil
	openDetail(m)
	if !strings.Contains(screenText(m), "no samples yet") {
		t.Errorf("an empty history must say so:\n%s", screenText(m))
	}
	assertFits(t, m, 120, 35)
}

func TestDetailHeaderHighlightsTheState(t *testing.T) {
	m, mon := editorSetup(120, 35)
	zombie := proc(200, "/usr/bin/editor", 0)
	zombie.State = 'Z'
	zombie.StartTime = 777
	mon.snap = snapshotOf(proc(100, "shell", 1), zombie, proc(300, "other", 2))
	m.Update(refreshedMsg{snap: mon.snap})
	openDetail(m)

	if !strings.Contains(screenText(m), "state Z zombie") {
		t.Errorf("the state must be named:\n%s", screenText(m))
	}
	if line := rawLine(t, m, "state"); !strings.Contains(line, sgrFg(colZombie)) {
		t.Errorf("a zombie state is drawn in the zombie colour: %q", line)
	}
}

func TestEscGoesBackToTheTableOnTheSameProcess(t *testing.T) {
	for _, closer := range []string{"esc", "enter"} {
		t.Run(closer, func(t *testing.T) {
			m, _ := editorSetup(120, 35)
			openDetail(m)
			press(m, closer)

			if m.view != viewTable {
				t.Fatal("the table must be back")
			}
			if m.selected() != 200 {
				t.Errorf("selected PID = %d, want 200", m.selected())
			}
			if out := screenText(m); !strings.Contains(out, "COMMAND") || strings.Contains(out, "CPU history") {
				t.Errorf("the table must be on screen again:\n%s", out)
			}
		})
	}
}

func TestSelectionStaysOnTheOpenedProcessWhileTheOrderChanges(t *testing.T) {
	m := loaded(120, 35, proc(10, "alpha", 50), proc(20, "beta", 30))
	deliver(m, press(m, "enter")) // alpha, the top row, never explicitly picked

	m.Update(refreshedMsg{snap: snapshotOf(proc(10, "alpha", 1), proc(20, "beta", 90))})
	press(m, "esc")

	if m.selected() != 10 {
		t.Errorf("selected PID = %d, want 10: the process whose detail was open", m.selected())
	}
}

func TestQuitStillWorksInTheDetail(t *testing.T) {
	for _, name := range []string{"q", "ctrl+c"} {
		m, _ := editorSetup(120, 35)
		openDetail(m)
		if !isQuit(press(m, name)) {
			t.Errorf("%s must quit from the detail view", name)
		}
	}
}

func TestTableKeysDoNothingInTheDetail(t *testing.T) {
	m, _ := editorSetup(120, 35)
	openDetail(m)
	sortKey := m.sortKey

	press(m, "c", "m", "p", "t")
	if m.sortKey != sortKey || m.tree || m.view != viewDetail {
		t.Errorf("sort/tree keys must be ignored in the detail (sort %v, tree %v, view %v)", m.sortKey, m.tree, m.view)
	}
	press(m, "/")
	if m.mode != modeNormal {
		t.Error("the filter cannot be edited from the detail view")
	}
}

func TestProcessThatExitsWhileTheDetailIsOpen(t *testing.T) {
	m, _ := editorSetup(120, 35)
	openDetail(m)

	// The next refresh no longer lists PID 200.
	m.Update(refreshedMsg{snap: snapshotOf(proc(100, "shell", 1), proc(300, "other", 2))})

	out := screenText(m)
	if !strings.Contains(out, "process 200 no longer exists") {
		t.Errorf("a clear message is expected:\n%s", out)
	}
	if strings.Contains(out, "state S sleeping") {
		t.Error("stale details of the dead process must not stay on screen")
	}
	assertFits(t, m, 120, 35)

	press(m, "esc")
	if m.view != viewTable {
		t.Error("esc must still go back")
	}
}

func TestRecycledPIDIsNotTheSameProcess(t *testing.T) {
	m, mon := editorSetup(120, 35)
	openDetail(m)

	other := proc(200, "a-stranger", 0)
	other.StartTime = 999 // same PID, another start time: another process
	m.Update(refreshedMsg{snap: snapshotOf(proc(100, "shell", 1), other)})
	_ = mon

	out := screenText(m)
	if !strings.Contains(out, "no longer exists") || strings.Contains(out, "a-stranger") {
		t.Errorf("a reused PID must not be shown as the old process:\n%s", out)
	}
}

func TestStateNames(t *testing.T) {
	for state, want := range map[byte]string{
		'R': "running", 'S': "sleeping", 'D': "disk sleep", 'Z': "zombie", 'T': "stopped",
		't': "tracing stop", 'I': "idle", 'X': "dead", 'x': "dead", '?': "unknown",
	} {
		if got := stateName(state); got != want {
			t.Errorf("stateName(%q) = %q, want %q", state, got, want)
		}
	}
}

// scaleSetup opens the detail of a process with the given CPU history, in the
// units of the monitor, on a machine with that many cores.
func scaleSetup(history []float64, cores int, opts ...Option) (*Model, *fakeMonitor) {
	mon := &fakeMonitor{history: map[int][]float64{200: history}}
	snap := snapshotOf(proc(200, "burner", 25))
	snap.Cores = cores
	mon.snap = snap
	m := New(mon, &fakeControl{}, opts...)
	m.Update(windowSize(120, 35))
	m.Update(refreshedMsg{snap: snap})
	deliver(m, press(m, "enter"))
	return m, mon
}

func TestDetailChartOfAnIdleProcessIsFlat(t *testing.T) {
	m, _ := scaleSetup([]float64{0, 0, 0}, 4)
	if row := sparkRow(m); row == "" || strings.Trim(row, "▁") != "" {
		t.Errorf("an idle series must draw only the lowest block: %q", row)
	}
}

func TestDetailChartShowsTheLargestSampleAsTheTallestColumn(t *testing.T) {
	m, _ := scaleSetup([]float64{1, 6, 3}, 4)
	row := sparkRow(m)
	if !strings.Contains(row, "▅") || strings.ContainsAny(row, "▆▇█") {
		t.Errorf("6 on a scale of 10 draws ▅ at most: %q", row)
	}
	if !strings.Contains(row, "▂") || !strings.Contains(row, "▃") {
		t.Errorf("1 and 3 draw ▂ and ▃, below the 6: %q", row)
	}
}
