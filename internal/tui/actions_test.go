package tui

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/EliabParra/procmon/internal/monitor"
	"github.com/EliabParra/procmon/internal/oserr"
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

func TestOwnPIDDefaultsToTheRunningProcess(t *testing.T) {
	if got := New(&fakeMonitor{}, &fakeControl{}).selfPID; got != os.Getpid() {
		t.Errorf("selfPID = %d, want %d", got, os.Getpid())
	}
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
	for _, ask := range []string{"x", "X", "s", "r"} {
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
	for _, ask := range []string{"x", "X", "s", "r"} {
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

func TestEveryActionNeedsItsOwnAnswer(t *testing.T) {
	m, _, ctl := actionSetup()
	press(m, "x", "n", "X", "y") // declining one must not confirm the next
	press(m, "X", "n", "x", "y")
	press(m, "s", "n", "r", "y")
	press(m, "r", "n", "s", "y")
	want := []string{"kill 200", "terminate 200", "resume 200", "suspend 200"}
	if !slices.Equal(ctl.calls, want) {
		t.Errorf("calls = %v, want %v, each after its own yes", ctl.calls, want)
	}
}

func TestSuspendAndResumeAskBeforeSending(t *testing.T) {
	tests := []struct {
		key, verb, signal, done, call string
	}{
		{"s", "Suspend", "SIGSTOP", "stopped", "suspend 200"},
		{"r", "Resume", "SIGCONT", "resumed", "resume 200"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			m, _, ctl := actionSetup()
			press(m, tt.key)

			if len(ctl.calls) != 0 {
				t.Fatalf("nothing may be sent before the answer, got %v", ctl.calls)
			}
			if m.mode != modeConfirm {
				t.Fatal("the key must open a y/n question")
			}
			line := footerText(m)
			for _, want := range []string{tt.verb + " 200 (firefox)?", "y/n"} {
				if !strings.Contains(line, want) {
					t.Errorf("the question is missing %q: %q", want, line)
				}
			}
			if strings.Contains(line, "Press y") {
				t.Errorf("only force kill spells out its answer: %q", line)
			}

			press(m, "y")
			if !slices.Equal(ctl.calls, []string{tt.call}) {
				t.Fatalf("calls = %v, want %q", ctl.calls, tt.call)
			}
			got := footerText(m)
			if !strings.Contains(got, tt.signal+" sent to 200 (firefox)") || !strings.Contains(got, tt.done) {
				t.Errorf("the status line must say what happened: %q", got)
			}
			if m.mode != modeNormal || m.notice.isErr {
				t.Errorf("mode = %v, notice = %+v", m.mode, m.notice)
			}
		})
	}
}

func TestTheMessageLastsUntilTheNextKey(t *testing.T) {
	m, _, _ := actionSetup()
	press(m, "s", "y")
	if !strings.Contains(footerText(m), "SIGSTOP") {
		t.Fatal("test setup: the message should be showing")
	}
	press(m, "down")
	if strings.Contains(footerText(m), "SIGSTOP") {
		t.Error("the message must go away on the next key")
	}
}

func TestFailuresAreExplainedNotFatal(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want []string
	}{
		{"the process is gone", fmt.Errorf("control: %w", oserr.ErrProcessGone), []string{"process 200 no longer exists"}},
		{"permission denied", fmt.Errorf("control: %w", oserr.ErrPermission), []string{"permission denied", "root", "owner", "SIGSTOP", "200 (firefox)"}},
		{"anything else", errors.New("boom"), []string{"SIGSTOP to 200 (firefox) failed", "boom"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _, ctl := actionSetup()
			ctl.err = tt.err
			if isQuit(press(m, "s", "y")) {
				t.Fatal("an error must not quit")
			}
			got := footerText(m)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("the message is missing %q: %q", want, got)
				}
			}
			if !m.notice.isErr {
				t.Errorf("a failure must be flagged as an error: %+v", m.notice)
			}
			if !strings.Contains(rawFooter(m), sgrFg(colError)) {
				t.Errorf("errors are drawn in the error colour: %q", rawFooter(m))
			}
		})
	}
}

func TestAStaleRowNeverReachesTheKernel(t *testing.T) {
	tests := []struct {
		name  string
		after func(mon *fakeMonitor)
	}{
		{"the process exited", func(mon *fakeMonitor) {
			mon.snap = snapshotOf(proc(100, "shell", 1), proc(300, "other", 2))
		}},
		{"its PID went to another process", func(mon *fakeMonitor) {
			stranger := proc(200, "stranger", 0)
			stranger.StartTime = 999
			mon.snap = snapshotOf(proc(100, "shell", 1), stranger, proc(300, "other", 2))
		}},
	}
	for _, tt := range tests {
		for _, keys := range [][]string{{"s", "y"}, {"r", "y"}, {"x", "y"}, {"X", "y"}} {
			t.Run(fmt.Sprintf("%s/%s", tt.name, strings.Join(keys, "")), func(t *testing.T) {
				m, mon, ctl := actionSetup()
				// The screen still shows the old row: no refresh has come yet.
				tt.after(mon)

				press(m, keys...)

				if len(ctl.calls) != 0 {
					t.Fatalf("a signal was sent to a process that is not the one listed: %v", ctl.calls)
				}
				if got := footerText(m); !strings.Contains(got, "process 200 no longer exists") {
					t.Errorf("the message must say the process is gone: %q", got)
				}
				if !slices.Contains(mon.verified, "200@777") {
					t.Errorf("the PID and start time of the row must be verified, got %v", mon.verified)
				}
			})
		}
	}
}

func TestVerifyIsCalledRightBeforeEverySignal(t *testing.T) {
	m, mon, ctl := actionSetup()
	press(m, "s", "y", "r", "y", "x", "y", "X", "y")
	if len(ctl.calls) != 4 || len(mon.verified) != 4 {
		t.Errorf("verified %v before sending %v", mon.verified, ctl.calls)
	}
}

func TestUnverifiableProcessIsNotSignalled(t *testing.T) {
	m, mon, ctl := actionSetup()
	mon.verifyErr = fmt.Errorf("monitor: verify 200: %w", oserr.ErrPermission)
	press(m, "s", "y")
	if len(ctl.calls) != 0 {
		t.Errorf("calls = %v, want none: the process could not be verified", ctl.calls)
	}
	if got := footerText(m); !strings.Contains(got, "permission denied") {
		t.Errorf("message = %q", got)
	}

	mon.verifyErr = errors.New("boom")
	press(m, "s", "y")
	if len(ctl.calls) != 0 || !strings.Contains(footerText(m), "boom") {
		t.Errorf("calls = %v, message = %q", ctl.calls, footerText(m))
	}
}

func TestTheQuestionIsAboutTheProcessThatWasAsked(t *testing.T) {
	for _, ask := range []string{"x", "X", "s", "r"} {
		t.Run(ask, func(t *testing.T) {
			m, mon, ctl := actionSetup()
			press(m, ask) // about 200

			// A refresh arrives while the question is open: 200 is gone and the
			// selection moves to its neighbour.
			mon.snap = snapshotOf(proc(100, "shell", 1), proc(300, "other", 2))
			m.Update(refreshedMsg{snap: mon.snap})
			if m.selected() == 200 {
				t.Fatal("test setup: the selection should have moved")
			}
			press(m, "y")

			if len(ctl.calls) != 0 {
				t.Errorf("calls = %v: the answer must not go to the neighbour", ctl.calls)
			}
			if got := footerText(m); !strings.Contains(got, "process 200 no longer exists") {
				t.Errorf("message = %q", got)
			}
		})
	}
}

// The question belongs to the row that was selected when the key was pressed,
// whatever else is pressed while it is open.
func TestTheAnswerGoesToTheRowThatWasAsked(t *testing.T) {
	for ask, call := range map[string]string{"s": "suspend 200", "r": "resume 200"} {
		t.Run(ask, func(t *testing.T) {
			m, _, ctl := actionSetup()
			press(m, ask)
			press(m, "down", "down") // ignored while the question is open
			press(m, "y")
			if !slices.Equal(ctl.calls, []string{call}) {
				t.Errorf("calls = %v, want only %q", ctl.calls, call)
			}
		})
	}
}

func TestTheMonitorNeverActsOnItself(t *testing.T) {
	for _, key := range []string{"x", "X", "s", "r"} {
		t.Run(key, func(t *testing.T) {
			m, _, ctl := actionSetup(WithSelfPID(200))
			press(m, key)

			if m.mode != modeNormal {
				t.Error("no question or prompt may open for procmon itself")
			}
			press(m, "y") // in case a question did open
			if len(ctl.calls) != 0 {
				t.Fatalf("calls = %v: procmon must not touch itself", ctl.calls)
			}
		})
	}
}

func TestSelfRefusalMessage(t *testing.T) {
	tests := map[string]string{
		"x": "refusing to terminate procmon itself (PID 200)",
		"X": "refusing to force kill procmon itself (PID 200)",
		"s": "refusing to suspend procmon itself (PID 200)",
		"r": "refusing to resume procmon itself (PID 200)",
	}
	for key, want := range tests {
		m, _, _ := actionSetup(WithSelfPID(200))
		press(m, key)
		if got := footerText(m); !strings.Contains(got, want) {
			t.Errorf("%s: message = %q, want %q", key, got, want)
		}
		if !m.notice.isErr {
			t.Errorf("%s: the refusal is an error message", key)
		}
	}
}

// perform is the one place that reaches the kernel, so it checks for itself
// again, whatever the caller did before.
func TestTheGuardAlsoStandsAtTheSignalItself(t *testing.T) {
	m, mon, ctl := actionSetup(WithSelfPID(200))
	for key, a := range actions {
		m.perform(a, target{pid: 200, startTime: 777, name: "firefox"})
		if len(ctl.calls) != 0 || len(mon.verified) != 0 {
			t.Fatalf("%s: procmon reached the kernel for itself: %v", key, ctl.calls)
		}
	}
}

func TestOtherProcessesStillWorkWhenOneIsOurOwn(t *testing.T) {
	m, _, ctl := actionSetup(WithSelfPID(100))
	press(m, "s", "y")
	if !slices.Equal(ctl.calls, []string{"suspend 200"}) {
		t.Errorf("calls = %v", ctl.calls)
	}
}

func TestActionsWithNothingSelected(t *testing.T) {
	m := loaded(100, 30)
	for _, key := range []string{"x", "X", "s", "r"} {
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

func TestTheNameFallsBackToTheCommand(t *testing.T) {
	p := proc(200, "/bin/thing --flag", 5)
	p.Comm = ""
	p.StartTime = 1
	mon, ctl := &fakeMonitor{}, &fakeControl{}
	m := loadedOpts(mon, ctl, 140, 35, []monitor.Process{p})
	press(m, "s")
	if got := footerText(m); !strings.Contains(got, "Suspend 200 (/bin/thing --flag)?") {
		t.Errorf("question = %q", got)
	}
	press(m, "y")
	if got := footerText(m); !strings.Contains(got, "200 (/bin/thing --flag)") {
		t.Errorf("message = %q", got)
	}
}

func TestHelpLineListsTheActionKeys(t *testing.T) {
	for _, build := range map[string]func() *Model{
		"table": func() *Model { m, _, _ := actionSetup(); m.Update(windowSize(200, 35)); return m },
		"detail": func() *Model {
			m, _, _ := actionSetup()
			m.Update(windowSize(200, 35))
			deliver(m, press(m, "enter"))
			return m
		},
	} {
		help := footerText(build())
		for _, want := range []string{"x terminate", "X kill", "s suspend", "r resume"} {
			if !strings.Contains(help, want) {
				t.Errorf("help line is missing %q: %q", want, help)
			}
		}
	}
}
