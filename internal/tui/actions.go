package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/EliabParra/procmon/internal/monitor"
)

// action is one of the signals the screen can send.
type action struct {
	signal string // the signal's name, as the status line shows it
	verb   string // what the user asked: "refusing to terminate procmon itself"
	head   string // how the question names its target, with a %s for it
	why    string // what the signal does; the first part of the question to go when the line is short
	done   string // what the status line adds once it was sent
	force  bool   // the stronger, separate question
	send   func(Control, int) error
}

// actions are the keys that send a signal. Every one asks first, each with its
// own question: the row under the cursor can change between frames, so a key
// press alone never reaches a process.
var actions = map[string]action{
	"x": {signal: "SIGTERM", verb: "terminate", head: "Terminate %s?", why: "It is asked to exit and can clean up.", done: "asked to exit", send: Control.Terminate},
	"X": {signal: "SIGKILL", verb: "force kill", head: "Force kill %s", why: "— the process cannot clean up.", done: "it cannot be caught or ignored", force: true, send: Control.Kill},
}

// target is the process an action is about. It is captured when the action
// starts, with the start time that tells it apart from a process that reuses
// the PID later: a refresh while a question is open cannot change who the
// answer goes to.
type target struct {
	pid       int
	startTime uint64
	name      string
}

func newTarget(p monitor.Process) target {
	name := clean(p.Comm)
	if name == "" {
		name = clean(p.Command)
	}
	return target{pid: p.PID, startTime: p.StartTime, name: name}
}

// String is how messages name the process: 4321 (firefox).
func (t target) String() string { return fmt.Sprintf("%d (%s)", t.pid, t.name) }

// pending is the action waiting for its answer: the y/n of a signal, or the
// value of a new nice (nice is then the one the process has now).
type pending struct {
	act  action
	who  target
	nice int
}

// actionTarget is the process the keys act on: the open one in the detail
// view, the selected row in the table. When there is none, it says so.
func (m *Model) actionTarget() (target, bool) {
	p, ok := m.selectedProc()
	if !ok {
		m.notice = notice{text: "no process selected"}
		return target{}, false
	}
	return newTarget(p), true
}

// startAction handles x, X, s and r: it captures the target and asks y/n.
func (m *Model) startAction(key string) {
	a := actions[key]
	t, ok := m.actionTarget()
	if !ok {
		return
	}
	m.mode, m.pending = modeConfirm, pending{act: a, who: t}
}

// perform sends the signal.
func (m *Model) perform(a action, t target) {
	failed := fmt.Sprintf("%s to %s failed", a.signal, t)
	if err := a.send(m.ctl, t.pid); err != nil {
		m.notice = notice{text: fmt.Sprintf("%s: %s", failed, clean(err.Error())), isErr: true}
		return
	}
	m.notice = notice{text: fmt.Sprintf("%s sent to %s — %s", a.signal, t, a.done)}
}

// confirmKey answers the question: y does it, n or esc cancels, and any other
// key is ignored so a stray press can neither confirm nor lose the question.
func (m *Model) confirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		p := m.pending
		m.mode, m.pending = modeNormal, pending{}
		m.perform(p.act, p.who)
	case "n", "N", "esc":
		m.mode, m.pending = modeNormal, pending{}
		m.notice = notice{text: "cancelled — nothing was sent"}
	}
	return m, nil
}

// confirmLine is the question in the bottom line. Force kill is worded
// differently and drawn in the alarm colour, so it cannot be mistaken for
// terminate.
func (m *Model) confirmLine(w int) string {
	a, t := m.pending.act, m.pending.who
	answer := "y/n"
	qStyle, aStyle := m.styles.text, m.styles.accent
	if a.force {
		qStyle = m.styles.err.Bold(true)
		aStyle = qStyle
		answer = "Press y to confirm, n to cancel"
	}
	question := fmt.Sprintf(a.head, t) + " " + a.why
	return qStyle.Render(question) + gap + aStyle.Render(answer)
}
