package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/EliabParra/procmon/internal/oserr"
)

func TestNicePromptShowsTheCurrentValueAndTheRange(t *testing.T) {
	m, _, ctl := actionSetup()
	ctl.nice = 3
	press(m, "n")

	if m.mode != modeNice {
		t.Fatal("n must open the nice prompt")
	}
	line := footerText(m)
	for _, want := range []string{"nice of 200 (firefox) is 3", "-20..19"} {
		if !strings.Contains(line, want) {
			t.Errorf("the prompt is missing %q: %q", want, line)
		}
	}
	if !slices.Equal(ctl.calls, []string{"priority 200"}) {
		t.Errorf("calls = %v: the current nice comes from Priority and nothing is set yet", ctl.calls)
	}
	if cur := m.View().Cursor; cur == nil || cur.Y != m.height-1 {
		t.Errorf("the text cursor belongs on the prompt line: %+v", cur)
	}
}

func TestTypingANiceValueSetsIt(t *testing.T) {
	m, mon, ctl := actionSetup()
	ctl.nice = 3
	press(m, "n")
	typeText(m, "5")
	press(m, "enter")

	want := []string{"priority 200", "setpriority 5 200"}
	if !slices.Equal(ctl.calls, want) {
		t.Fatalf("calls = %v, want %v", ctl.calls, want)
	}
	if got := footerText(m); !strings.Contains(got, "nice of 200 (firefox) set from 3 to 5") {
		t.Errorf("the status line must say what happened: %q", got)
	}
	if m.mode != modeNormal || m.notice.isErr {
		t.Errorf("mode = %v, notice = %+v", m.mode, m.notice)
	}
	if !slices.Contains(mon.verified, "200@777") {
		t.Errorf("the process must be verified before the change, got %v", mon.verified)
	}
}

func TestNiceAcceptsEveryValueOfTheRange(t *testing.T) {
	for typed, want := range map[string]int{"-20": -20, "19": 19, "0": 0, "+7": 7, " 7 ": 7, "-1": -1, "007": 7} {
		t.Run(typed, func(t *testing.T) {
			m, _, ctl := actionSetup()
			press(m, "n")
			typeText(m, typed)
			press(m, "enter")
			if got := ctl.calls[len(ctl.calls)-1]; got != fmt.Sprintf("setpriority %d 200", want) {
				t.Errorf("typed %q: last call %q, want nice %d", typed, got, want)
			}
		})
	}
}

func TestNiceRejectsAnythingElse(t *testing.T) {
	for _, typed := range []string{"", "   ", "abc", "20", "-21", "100", "5.5", "5,5", "1e1", "0x5", "- 3", "--3", "99999999999999999999", "５"} {
		t.Run(fmt.Sprintf("%q", typed), func(t *testing.T) {
			m, _, ctl := actionSetup()
			press(m, "n")
			typeText(m, typed)
			press(m, "enter")

			for _, c := range ctl.calls {
				if strings.HasPrefix(c, "setpriority") {
					t.Fatalf("nice %q reached the kernel: %v", typed, ctl.calls)
				}
			}
			got := footerText(m)
			if !strings.Contains(got, "invalid nice") {
				t.Errorf("the message must say the input is invalid: %q", got)
			}
			if !m.notice.isErr || m.mode != modeNormal {
				t.Errorf("notice = %+v, mode = %v", m.notice, m.mode)
			}
		})
	}
}

func TestInvalidNiceExplainsWhy(t *testing.T) {
	m, _, _ := actionSetup()
	press(m, "n")
	typeText(m, "99")
	press(m, "enter")
	if got := footerText(m); !strings.Contains(got, "-20") || !strings.Contains(got, "19") {
		t.Errorf("an out-of-range value must give the range: %q", got)
	}

	press(m, "n")
	typeText(m, "fast")
	press(m, "enter")
	if got := footerText(m); !strings.Contains(got, `"fast"`) || !strings.Contains(got, "whole number") {
		t.Errorf("a non-number must say what is expected: %q", got)
	}
}

func TestEscapeCancelsTheNicePrompt(t *testing.T) {
	m, _, ctl := actionSetup()
	press(m, "n")
	typeText(m, "5")
	press(m, "esc")

	if m.mode != modeNormal || len(ctl.calls) != 1 {
		t.Errorf("mode = %v, calls = %v: only the read of the current value may have happened", m.mode, ctl.calls)
	}
	if m.pending.who.pid != 0 {
		t.Error("a cancelled prompt must not leave its target behind")
	}
}

func TestNiceInputIsTextNotHotKeys(t *testing.T) {
	m, _, ctl := actionSetup()
	press(m, "n")
	typeText(m, "xXsrq/n")
	if m.mode != modeNice || len(ctl.calls) != 1 {
		t.Errorf("mode = %v, calls = %v: typed letters are text", m.mode, ctl.calls)
	}
	press(m, "backspace", "ctrl+u")
	typeText(m, "4")
	press(m, "enter")
	if got := ctl.calls[len(ctl.calls)-1]; got != "setpriority 4 200" {
		t.Errorf("last call = %q", got)
	}
}

func TestNicePasteIsOneCleanLine(t *testing.T) {
	m, _, ctl := actionSetup()
	press(m, "n")
	m.Update(tea.PasteMsg{Content: "6\n"})
	press(m, "enter")
	if got := ctl.calls[len(ctl.calls)-1]; got != "setpriority 6 200" {
		t.Errorf("last call = %q", got)
	}
}

func TestCannotReadTheCurrentNice(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"gone", fmt.Errorf("control: %w", oserr.ErrProcessGone), "process 200 no longer exists"},
		{"other", errors.New("boom"), "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _, ctl := actionSetup()
			ctl.niceErr = tt.err
			press(m, "n")
			if m.mode != modeNormal {
				t.Error("no prompt without a current value")
			}
			if got := footerText(m); !strings.Contains(got, tt.want) || !m.notice.isErr {
				t.Errorf("message = %q (error %v)", got, m.notice.isErr)
			}
		})
	}
}

func TestNiceFailuresAreExplained(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want []string
	}{
		{"gone", fmt.Errorf("control: %w", oserr.ErrProcessGone), []string{"process 200 no longer exists"}},
		{"permission", fmt.Errorf("control: %w", oserr.ErrPermission), []string{"permission denied", "lowering nice needs root", "CAP_SYS_NICE", "owner"}},
		{"other", errors.New("boom"), []string{"nice 5", "200 (firefox)", "failed", "boom"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _, ctl := actionSetup()
			press(m, "n")
			typeText(m, "5")
			ctl.err = tt.err
			if isQuit(press(m, "enter")) {
				t.Fatal("an error must not quit")
			}
			got := footerText(m)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("the message is missing %q: %q", want, got)
				}
			}
			if !m.notice.isErr || !strings.Contains(rawFooter(m), sgrFg(colError)) {
				t.Errorf("failures are drawn as errors: %+v", m.notice)
			}
		})
	}
}

func TestAStaleRowNeverGetsANewNice(t *testing.T) {
	m, mon, ctl := actionSetup()
	press(m, "n")
	typeText(m, "5")
	// While the prompt was open the process died and its PID was reused.
	stranger := proc(200, "stranger", 0)
	stranger.StartTime = 999
	mon.snap = snapshotOf(proc(100, "shell", 1), stranger)
	press(m, "enter")

	for _, c := range ctl.calls {
		if strings.HasPrefix(c, "setpriority") {
			t.Fatalf("a stranger got a new nice: %v", ctl.calls)
		}
	}
	if got := footerText(m); !strings.Contains(got, "process 200 no longer exists") {
		t.Errorf("message = %q", got)
	}
}

func TestTheNicePromptIsAboutTheProcessThatWasAsked(t *testing.T) {
	m, mon, ctl := actionSetup()
	press(m, "n")
	typeText(m, "5")

	mon.snap = snapshotOf(proc(100, "shell", 1), proc(300, "other", 2))
	m.Update(refreshedMsg{snap: mon.snap}) // 200 is gone, the selection moves on
	press(m, "enter")

	for _, c := range ctl.calls {
		if strings.HasPrefix(c, "setpriority") {
			t.Errorf("the value must not go to the neighbour: %v", ctl.calls)
		}
	}
}

func TestTheMonitorNeverChangesItsOwnPriority(t *testing.T) {
	m, _, ctl := actionSetup(WithSelfPID(200))
	press(m, "n")
	if m.mode != modeNormal || len(ctl.calls) != 0 {
		t.Errorf("mode = %v, calls = %v: nothing may happen for procmon itself", m.mode, ctl.calls)
	}
	if got := footerText(m); !strings.Contains(got, "refusing to change the priority of procmon itself (PID 200)") {
		t.Errorf("message = %q", got)
	}
}

// Like perform for the signals, the commit is the one place that reaches the
// kernel, so it checks for itself again.
func TestTheGuardAlsoStandsAtTheNiceCommit(t *testing.T) {
	m, mon, ctl := actionSetup(WithSelfPID(200))
	m.commitNice(pending{who: target{pid: 200, startTime: 777, name: "firefox"}, nice: 0}, "5")
	if len(ctl.calls) != 0 || len(mon.verified) != 0 {
		t.Errorf("procmon reached the kernel for itself: %v", ctl.calls)
	}
}

func TestNiceWithNothingSelected(t *testing.T) {
	m := loaded(100, 30)
	press(m, "n")
	if got := footerText(m); !strings.Contains(got, "no process selected") || m.mode != modeNormal {
		t.Errorf("message = %q, mode = %v", got, m.mode)
	}
}

func TestHelpLineListsTheNiceKey(t *testing.T) {
	m, _, _ := actionSetup()
	m.Update(windowSize(200, 35))
	if help := footerText(m); !strings.Contains(help, "n nice") {
		t.Errorf("help line is missing the nice key: %q", help)
	}
}
