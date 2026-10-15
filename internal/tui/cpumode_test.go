package tui

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/EliabParra/procmon/internal/monitor"
)

// cpuSetup is a machine of four cores (the one snapshotOf describes). The
// selected process, 200, keeps one core busy: 25 in the units of the course
// formula, a share of the whole machine, and 100 in the per-core view.
func cpuSetup(opts ...Option) (*Model, *fakeMonitor) {
	mon := &fakeMonitor{history: map[int][]float64{200: {0, 20, 40, 60}}}
	procs := []monitor.Process{proc(100, "shell", 1), proc(200, "burner", 25), proc(300, "other", 5)}
	return loadedOpts(mon, &fakeControl{}, 120, 35, procs, opts...), mon
}

// cpuCellOf is the %CPU cell of the row that shows command: the text between
// the state letter and the %MEM of the test rows (always 1.5).
func cpuCellOf(t *testing.T, m *Model, command string) string {
	t.Helper()
	re := regexp.MustCompile(`\sS\s+(\S+)\s+1\.5\s`)
	for _, l := range screen(m) {
		if strings.Contains(l, command) {
			if sub := re.FindStringSubmatch(l); sub != nil {
				return sub[1]
			}
		}
	}
	t.Fatalf("no row of %q with a %%CPU cell on:\n%s", command, screenText(m))
	return ""
}

func TestCPUColumnIsTheShareOfTheMachineByDefault(t *testing.T) {
	m, _ := cpuSetup()
	if got := cpuCellOf(t, m, "burner"); got != "25.0" {
		t.Errorf("%%CPU = %q, want the machine share 25.0", got)
	}
	if out := screenText(m); !strings.Contains(out, "cpu machine") {
		t.Errorf("the header must name the mode:\n%s", out)
	}
}

func TestShiftIToggleBetweenMachineAndPerCore(t *testing.T) {
	m, _ := cpuSetup()

	press(m, "I")
	if got := cpuCellOf(t, m, "burner"); got != "100.0" {
		t.Errorf("per-core %%CPU = %q, want one flat-out core of four to read 100.0", got)
	}
	if got := cpuCellOf(t, m, "shell"); got != "4.0" {
		t.Errorf("per-core %%CPU of shell = %q, want 4.0", got)
	}
	if out := screenText(m); !strings.Contains(out, "cpu per-core") || strings.Contains(out, "cpu machine") {
		t.Errorf("the header must say per-core:\n%s", out)
	}

	press(m, "I")
	if got := cpuCellOf(t, m, "burner"); got != "25.0" {
		t.Errorf("%%CPU = %q after the second press, want 25.0 again", got)
	}
	if out := screenText(m); !strings.Contains(out, "cpu machine") {
		t.Errorf("the header must say machine again:\n%s", out)
	}
}

func TestCPUModeDoesNotChangeTheOrderOfTheRows(t *testing.T) {
	for _, tree := range []bool{false, true} {
		m, _ := cpuSetup()
		if tree {
			press(m, "t")
		}
		before := rowPIDs(m)
		press(m, "I")
		if got := rowPIDs(m); !slices.Equal(got, before) {
			t.Errorf("tree=%v: order %v after I, want %v: the factor is the same for every row", tree, got, before)
		}
		if before[0] != 200 {
			t.Fatalf("tree=%v: the sort by CPU must start with the burner, got %v", tree, before)
		}
	}
}

func TestCPUModeAppliesToTheTreeView(t *testing.T) {
	m, _ := cpuSetup()
	press(m, "t", "I")
	if got := cpuCellOf(t, m, "burner"); got != "100.0" {
		t.Errorf("tree %%CPU = %q in per-core mode, want 100.0", got)
	}
}

func TestCPUModeAppliesToTheDetailView(t *testing.T) {
	m, mon := cpuSetup()
	deliver(m, press(m, "enter")) // the burner is the top row

	out := screenText(m)
	for _, want := range []string{"cpu 25.0%", "min 0.0%", "avg 30.0%", "max 60.0%", "scale 0-100%", "cpu machine"} {
		if !strings.Contains(out, want) {
			t.Errorf("machine mode: the detail is missing %q:\n%s", want, out)
		}
	}
	const inner = 120 - 4
	if got, want := sparkRow(m), sparkline(mon.history[200], inner, 100); got != want {
		t.Errorf("machine sparkline =\n%q\nwant\n%q", got, want)
	}

	press(m, "I") // works inside the detail view and keeps it open
	if m.view != viewDetail {
		t.Fatal("I must not leave the detail view")
	}
	out = screenText(m)
	for _, want := range []string{"cpu 100.0%", "min 0.0%", "avg 120.0%", "max 240.0%", "scale 0-400%", "cpu per-core"} {
		if !strings.Contains(out, want) {
			t.Errorf("per-core mode: the detail is missing %q:\n%s", want, out)
		}
	}
	scaled := []float64{0, 80, 160, 240}
	if got, want := sparkRow(m), sparkline(scaled, inner, 400); got != want {
		t.Errorf("per-core sparkline =\n%q\nwant\n%q", got, want)
	}
	assertFits(t, m, 120, 35)

	press(m, "I")
	if out := screenText(m); !strings.Contains(out, "max 60.0%") {
		t.Errorf("back to machine mode the chart must read the spec units again:\n%s", out)
	}
}

func TestCPUModeDoesNotTouchTheStoredHistory(t *testing.T) {
	m, mon := cpuSetup()
	deliver(m, press(m, "enter"))
	press(m, "I", "I", "I")
	if got, want := mon.history[200], []float64{0, 20, 40, 60}; !slices.Equal(got, want) {
		t.Errorf("history = %v, want %v: scaling is for drawing only", got, want)
	}
}

func TestShiftIIsTextWhileTyping(t *testing.T) {
	tests := []struct {
		name  string
		start string
	}{
		{"filter", "/"},
		{"interval", "i"},
		{"nice", "n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _, _ := actionSetup()
			press(m, tt.start)
			if !m.typing() {
				t.Fatalf("%q must open an input", tt.start)
			}
			press(m, "I")
			if m.cpu != cpuMachine {
				t.Error("I changed the cpu mode while text was being typed")
			}
			if got := m.input.String(); got != "I" {
				t.Errorf("typed text = %q, want I", got)
			}
		})
	}
}

func TestShiftIIsIgnoredWhileAQuestionIsOpen(t *testing.T) {
	m, _, ctl := actionSetup()
	press(m, "x")
	press(m, "I")
	if m.mode != modeConfirm {
		t.Error("the question must stay open")
	}
	if m.cpu != cpuMachine {
		t.Error("I changed the cpu mode while a y/n question was open")
	}
	if len(ctl.calls) != 0 {
		t.Errorf("nothing may be sent, got %v", ctl.calls)
	}
}

func TestTogglingTheCPUModeSaysWhatTheNumbersMean(t *testing.T) {
	m, _ := cpuSetup()
	press(m, "I")
	if line := footerText(m); !strings.Contains(line, "one core") {
		t.Errorf("per-core notice = %q, want it to say that 100 is one core", line)
	}
	press(m, "I")
	if line := footerText(m); !strings.Contains(line, "whole machine") {
		t.Errorf("machine notice = %q, want it to say that 100 is the whole machine", line)
	}
}

func TestHelpLineListsTheCPUModeKey(t *testing.T) {
	m, _ := cpuSetup()
	m.Update(windowSize(200, 35))
	if help := footerText(m); !strings.Contains(help, "I cpu mode") {
		t.Errorf("the table help line is missing the key: %q", help)
	}
	deliver(m, press(m, "enter"))
	if help := footerText(m); !strings.Contains(help, "I cpu mode") {
		t.Errorf("the detail help line is missing the key: %q", help)
	}
}

func TestPerCoreCPUOptionStartsInPerCoreMode(t *testing.T) {
	m, _ := cpuSetup(WithPerCoreCPU())
	if got := cpuCellOf(t, m, "burner"); got != "100.0" {
		t.Errorf("%%CPU = %q, want 100.0 from the start", got)
	}
	if out := screenText(m); !strings.Contains(out, "cpu per-core") {
		t.Errorf("the header must say per-core:\n%s", out)
	}
	press(m, "I")
	if got := cpuCellOf(t, m, "burner"); got != "25.0" {
		t.Errorf("%%CPU = %q after I, want to go back to 25.0", got)
	}
}

func TestPerCoreWithoutACoreCountChangesNothing(t *testing.T) {
	snap := snapshotOf(proc(200, "burner", 25))
	snap.Cores = 0
	m := New(&fakeMonitor{}, &fakeControl{})
	m.Update(windowSize(120, 35))
	m.Update(refreshedMsg{snap: snap})
	press(m, "I")
	if got := cpuCellOf(t, m, "burner"); got != "25.0" {
		t.Errorf("%%CPU = %q, want 25.0: no core count means one core", got)
	}
}

func TestCPUFigureStaysInItsColumn(t *testing.T) {
	tests := []struct {
		v    float64
		want string
	}{
		{0, "0.0"},
		{25, "25.0"},
		{100, "100.0"},
		{999.94, "999.9"},
		{1600, "1600"},
		{12800, "12800"},
	}
	for _, tt := range tests {
		if got := cpuText(tt.v); got != tt.want {
			t.Errorf("cpuText(%v) = %q, want %q", tt.v, got, tt.want)
		}
	}

	// A 128-core machine flat out in per-core mode: 12800, and nothing breaks.
	snap := snapshotOf(proc(200, "burner", 100))
	snap.Cores = 128
	m := New(&fakeMonitor{}, &fakeControl{}, WithPerCoreCPU())
	m.Update(windowSize(120, 35))
	m.Update(refreshedMsg{snap: snap})
	if got := cpuCellOf(t, m, "burner"); got != "12800" {
		t.Errorf("%%CPU = %q, want 12800", got)
	}
	assertFits(t, m, 120, 35)
}
