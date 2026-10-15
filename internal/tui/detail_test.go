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

func TestEnterOpensTheDetailOfTheSelectedProcess(t *testing.T) {
	m, _ := editorSetup(120, 35)
	openDetail(m)
	out := screenText(m)

	for _, want := range []string{
		"PID 200", "state S", "user eliab", "nice 5",
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
