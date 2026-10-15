package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/EliabParra/procmon/internal/monitor"
)

// actionSetup is a model whose selected process is PID 200, firefox.
func actionSetup(opts ...Option) (*Model, *fakeMonitor, *fakeControl) {
	firefox := proc(200, "/usr/bin/firefox --new-window", 5)
	firefox.Comm = "firefox"
	firefox.StartTime = 777
	mon, ctl := &fakeMonitor{}, &fakeControl{}
	m := loadedOpts(mon, ctl, 140, 35, []monitor.Process{proc(100, "shell", 1), firefox, proc(300, "other", 2)}, opts...)
	press(m, "p", "down") // by PID: 100, 200, 300
	return m, mon, ctl
}

// footerText is the last line of the screen.
func footerText(m *Model) string {
	lines := screen(m)
	return strings.TrimRight(lines[len(lines)-1], " ")
}

// rawFooter is the last line of the screen with its styling.
func rawFooter(m *Model) string {
	lines := strings.Split(m.View().Content, "\n")
	return lines[len(lines)-1]
}

func TestTerminateAsksBeforeSending(t *testing.T) {
	m, _, ctl := actionSetup()
	press(m, "x")

	if len(ctl.calls) != 0 {
		t.Fatalf("nothing may be sent before the answer, got %v", ctl.calls)
	}
	line := footerText(m)
	for _, want := range []string{"Terminate", "200", "firefox", "y/n"} {
		if !strings.Contains(line, want) {
			t.Errorf("the question is missing %q: %q", want, line)
		}
	}
	if m.View().Cursor != nil {
		t.Error("a y/n question has no text cursor")
	}
}

func TestYesSendsSIGTERMAndTheStatusLineConfirmsIt(t *testing.T) {
	m, _, ctl := actionSetup()
	press(m, "x", "y")

	if !slices.Equal(ctl.calls, []string{"terminate 200"}) {
		t.Fatalf("calls = %v, want one terminate of 200", ctl.calls)
	}
	if got := footerText(m); !strings.Contains(got, "SIGTERM sent to 200 (firefox)") {
		t.Errorf("the status line must say what happened: %q", got)
	}
	if m.mode != modeNormal {
		t.Error("the question must close")
	}
	if m.notice.isErr {
		t.Error("a success is not an error")
	}
}

func TestAnythingButYesCancels(t *testing.T) {
	for _, ask := range []string{"x", "X"} {
		for _, answer := range []string{"n", "N", "esc"} {
			t.Run(ask+"/"+answer, func(t *testing.T) {
				m, _, ctl := actionSetup()
				press(m, ask, answer)

				if len(ctl.calls) != 0 {
					t.Errorf("calls = %v, want none", ctl.calls)
				}
				if m.mode != modeNormal {
					t.Error("the question must close")
				}
				if got := footerText(m); !strings.Contains(got, "cancelled") {
					t.Errorf("the status line must say it was cancelled: %q", got)
				}
			})
		}
	}
}

func TestOtherKeysLeaveTheQuestionOpen(t *testing.T) {
	for _, ask := range []string{"x", "X"} {
		t.Run(ask, func(t *testing.T) {
			m, _, ctl := actionSetup()
			press(m, ask)
			for _, k := range []string{"j", "q", "x", "X", "s", "r", "enter", "t", "/"} {
				cmd := press(m, k)
				if isQuit(cmd) {
					t.Fatalf("%s must not quit while a question is open", k)
				}
				if m.mode != modeConfirm {
					t.Fatalf("%s closed the question", k)
				}
			}
			if len(ctl.calls) != 0 {
				t.Errorf("calls = %v, want none", ctl.calls)
			}
			if !isQuit(press(m, "ctrl+c")) {
				t.Error("ctrl+c must always quit")
			}
		})
	}
}

func TestForceKillHasItsOwnStrongerConfirmation(t *testing.T) {
	m, _, ctl := actionSetup()
	press(m, "X")

	line := footerText(m)
	for _, want := range []string{"Force kill", "200", "firefox", "cannot clean up", "Press y to confirm"} {
		if !strings.Contains(line, want) {
			t.Errorf("the force kill question is missing %q: %q", want, line)
		}
	}
	if strings.Contains(line, "Terminate") || strings.Contains(line, "y/n") {
		t.Errorf("it must not look like the terminate question: %q", line)
	}
	if !strings.Contains(rawFooter(m), sgrFg(colError)) {
		t.Error("the force kill question is drawn in the alarm colour")
	}
	if len(ctl.calls) != 0 {
		t.Fatalf("nothing may be sent before the answer, got %v", ctl.calls)
	}

	press(m, "y")
	if !slices.Equal(ctl.calls, []string{"kill 200"}) {
		t.Errorf("calls = %v, want one kill of 200", ctl.calls)
	}
	if got := footerText(m); !strings.Contains(got, "SIGKILL sent to 200 (firefox)") {
		t.Errorf("the status line must say what happened: %q", got)
	}
}

func TestActionsWithNothingSelected(t *testing.T) {
	m := loaded(100, 30)
	for _, key := range []string{"x", "X"} {
		press(m, key)
		if got := footerText(m); !strings.Contains(got, "no process selected") {
			t.Errorf("%s: message = %q", key, got)
		}
		if m.mode != modeNormal {
			t.Errorf("%s: no question without a process", key)
		}
	}
}

func TestHostileNamesAreCleanedInMessagesAndQuestions(t *testing.T) {
	evil := proc(200, "evil", 5)
	evil.Comm = "evil\x1b[2J\tname"
	evil.StartTime = 1
	mon, ctl := &fakeMonitor{}, &fakeControl{}
	m := loadedOpts(mon, ctl, 140, 35, []monitor.Process{evil})

	press(m, "x")
	if line := footerText(m); strings.ContainsRune(line, '\x1b') || !strings.Contains(line, "evil?[2J?name") {
		t.Errorf("question = %q", line)
	}
	assertFits(t, m, 140, 35)
	press(m, "y")
	if got := m.View().Content; strings.Contains(got, "evil\x1b") {
		t.Errorf("an escape sequence reached the screen: %q", got)
	}
	assertFits(t, m, 140, 35)
}
