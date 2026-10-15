package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/EliabParra/procmon/internal/monitor"
	"github.com/EliabParra/procmon/internal/oserr"
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

func TestDetailListsTheThreads(t *testing.T) {
	m, _ := editorSetup(120, 35)
	openDetail(m)
	out := screenText(m)

	for _, title := range []string{"TID", "TIME", "NAME"} {
		if !strings.Contains(out, title) {
			t.Errorf("the thread list has no %s column:\n%s", title, out)
		}
	}
	for _, row := range []string{
		`200\s+S\s+0:01\.50\s+editor`,
		`201\s+R\s+1:30\.00\s+worker`,
		`202\s+D\s+0:02\.00\s+io-wait`,
	} {
		if !regexp.MustCompile(row).MatchString(out) {
			t.Errorf("no thread row matches %q:\n%s", row, out)
		}
	}
}

func TestThreadStatesUseTheSameColoursAsTheTable(t *testing.T) {
	m, mon := editorSetup(120, 35)
	mon.threads[200] = append(mon.threads[200], thread(203, "stuck", 'Z', 0), thread(204, "paused", 'T', 0))
	openDetail(m)

	for marker, c := range map[string]string{
		"io-wait": sgrFg(colDiskSleep),
		"stuck":   sgrFg(colZombie),
		"paused":  sgrFg(colStopped),
	} {
		if line := rawLine(t, m, marker); !strings.Contains(line, c) {
			t.Errorf("thread %s must be tinted: %q", marker, line)
		}
	}
	if line := rawLine(t, m, "worker"); strings.Contains(line, sgrFg(colDiskSleep)) || strings.Contains(line, sgrBg(colStopped)) {
		t.Errorf("a running thread keeps the plain look: %q", line)
	}
}

func TestThreadNamesAreCleaned(t *testing.T) {
	m, mon := editorSetup(120, 35)
	mon.threads[200] = []monitor.Thread{thread(200, "evil\x1b[2J\tname\n", 'S', 0)}
	openDetail(m)

	out := screenText(m)
	if strings.ContainsRune(out, '\x1b') || !strings.Contains(out, "evil?[2J?name?") {
		t.Errorf("hostile thread names must show as '?':\n%q", out)
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

func TestDetailRefreshesOnEveryTick(t *testing.T) {
	m, mon := editorSetup(120, 35)
	openDetail(m)
	before := mon.reads()

	mon.threads[200] = []monitor.Thread{thread(200, "editor", 'S', 0), thread(209, "fresh-thread", 'S', 0)}
	mon.history[200] = []float64{0, 0, 0, 0, 0, 100}
	refreshNow(m)

	if mon.reads() <= before {
		t.Error("a tick with the detail open must read the threads again")
	}
	out := screenText(m)
	if !strings.Contains(out, "fresh-thread") {
		t.Errorf("the new thread must show after the tick:\n%s", out)
	}
	if !strings.Contains(out, "6 samples") {
		t.Errorf("the chart must follow the history after the tick:\n%s", out)
	}
}

func TestTheTableDoesNotReadThreads(t *testing.T) {
	m, mon := editorSetup(120, 35)
	refreshNow(m)
	refreshNow(m)
	if mon.reads() != 0 {
		t.Errorf("threads were read %d times with no detail open", mon.reads())
	}

	openDetail(m)
	press(m, "esc")
	reads := mon.reads()
	refreshNow(m)
	if mon.reads() != reads {
		t.Error("closing the detail must stop reading threads")
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

func TestProcessThatDiesBetweenRefreshesIsReportedByTheThreadRead(t *testing.T) {
	m, mon := editorSetup(120, 35)
	openDetail(m)
	mon.threadsErr = fmt.Errorf("monitor: threads of 200: %w", oserr.ErrProcessGone)

	// The snapshot still lists it, but reading its threads says it is gone.
	refreshNow(m)

	if out := screenText(m); !strings.Contains(out, "process 200 no longer exists") {
		t.Errorf("a clear message is expected:\n%s", out)
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

func TestPermissionDeniedOnTheThreadsIsReportedAndTheRestStays(t *testing.T) {
	m, mon := editorSetup(120, 35)
	openDetail(m)
	mon.threadsErr = fmt.Errorf("monitor: threads of 200: %w", oserr.ErrPermission)
	refreshNow(m)

	out := screenText(m)
	if !strings.Contains(out, "permission denied") {
		t.Errorf("the threads section must say permission was denied:\n%s", out)
	}
	if !strings.Contains(out, "PID 200") || !strings.Contains(out, "CPU history") {
		t.Errorf("the header and the chart must stay:\n%s", out)
	}
}

func TestOtherThreadErrorsAreShownToo(t *testing.T) {
	m, mon := editorSetup(120, 35)
	openDetail(m)
	mon.threadsErr = fmt.Errorf("monitor: threads of 200: boom")
	refreshNow(m)
	if !strings.Contains(screenText(m), "boom") {
		t.Errorf("the reason must be on screen:\n%s", screenText(m))
	}
}

func TestDetailBeforeTheThreadsArrive(t *testing.T) {
	m, _ := editorSetup(120, 35)
	press(m, "p", "down", "enter") // the command that loads the detail has not run yet
	out := screenText(m)
	if !strings.Contains(out, "PID 200") || !strings.Contains(out, "reading threads") {
		t.Errorf("the detail must open at once and say it is reading:\n%s", out)
	}
}

func manyThreads(n int) []monitor.Thread {
	threads := make([]monitor.Thread, n)
	for i := range threads {
		threads[i] = thread(1000+i, fmt.Sprintf("t-%d", i), 'S', time.Duration(i)*time.Second)
	}
	return threads
}

// With no room for thread rows the title must not claim a range of zero rows.
func TestThreadTitleWithoutRoomForRowsSaysSo(t *testing.T) {
	m, _ := editorSetup(100, 16) // 8 lines in the box: the thread title is the last but one
	openDetail(m)

	out := screenText(m)
	if !strings.Contains(out, "Threads (3, enlarge the window)") {
		t.Errorf("the title must give the count and say why there are no rows:\n%s", out)
	}
	if strings.Contains(out, "1-0") {
		t.Errorf("no empty range:\n%s", out)
	}
	assertFits(t, m, 100, 16)

	press(m, "down", "end") // scrolling with no rows keeps the same title
	if !strings.Contains(screenText(m), "Threads (3, enlarge the window)") {
		t.Errorf("scrolling must not change it:\n%s", screenText(m))
	}
}

func TestThreadTitleNeverShowsAnEmptyRange(t *testing.T) {
	rangeOf := regexp.MustCompile(`\b(\d+)-(\d+) of`)
	for _, h := range []int{16, 17, 18, 19, 20, 25, 35} {
		t.Run(fmt.Sprintf("height %d", h), func(t *testing.T) {
			m, mon := editorSetup(100, h)
			mon.threads[200] = manyThreads(40)
			openDetail(m)

			got := rangeOf.FindStringSubmatch(screenText(m))
			if got == nil {
				return // no room for rows: the title says so instead
			}
			var first, last int
			fmt.Sscan(got[1], &first)
			fmt.Sscan(got[2], &last)
			if last < first {
				t.Errorf("the range %s-%s lists no rows", got[1], got[2])
			}
		})
	}
}

func TestDetailFitsEverySize(t *testing.T) {
	states := map[string]func(m *Model, mon *fakeMonitor){
		"loaded":  func(m *Model, mon *fakeMonitor) { openDetail(m) },
		"not yet": func(m *Model, mon *fakeMonitor) { press(m, "p", "down", "enter") },
		"gone": func(m *Model, mon *fakeMonitor) {
			openDetail(m)
			m.Update(refreshedMsg{snap: snapshotOf(proc(100, "shell", 1))})
		},
		"forbidden": func(m *Model, mon *fakeMonitor) { mon.threadsErr = oserr.ErrPermission; openDetail(m) },
		"many":      func(m *Model, mon *fakeMonitor) { mon.threads[200] = manyThreads(500); openDetail(m); press(m, "end") },
		"hostile": func(m *Model, mon *fakeMonitor) {
			mon.threads[200] = []monitor.Thread{thread(1, "日本語\x1b[31m"+strings.Repeat("x", 300), 'S', 0)}
			openDetail(m)
		},
	}
	sizes := [][2]int{{120, 35}, {80, 24}, {200, 60}, {60, 15}, {40, 12}, {30, 8}, {20, 5}, {10, 3}, {5, 2}, {1, 1}}
	for name, setup := range states {
		for _, size := range sizes {
			t.Run(fmt.Sprintf("%s %dx%d", name, size[0], size[1]), func(t *testing.T) {
				m, mon := editorSetup(size[0], size[1])
				setup(m, mon)
				assertFits(t, m, size[0], size[1])
			})
		}
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
