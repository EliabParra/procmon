package tui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/EliabParra/procmon/internal/monitor"
	"github.com/EliabParra/procmon/internal/oserr"
)

// WithSelfPID tells the screen which PID is the monitor itself, so it can
// refuse to act on it (default: the PID of the running process).
func WithSelfPID(pid int) Option {
	return func(m *Model) { m.selfPID = pid }
}

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
	"s": {signal: "SIGSTOP", verb: "suspend", head: "Suspend %s?", why: "It stays frozen until it is resumed.", done: "stopped", send: Control.Suspend},
	"r": {signal: "SIGCONT", verb: "resume", head: "Resume %s?", why: "It continues from where it stopped.", done: "resumed", send: Control.Resume},
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
	if m.view == viewDetail {
		p, alive := m.detailProc()
		if !alive {
			m.notice = notice{text: fmt.Sprintf("process %d no longer exists", m.detail.pid), isErr: true}
			return target{}, false
		}
		return newTarget(p), true
	}
	p, ok := m.selectedProc()
	if !ok {
		m.notice = notice{text: "no process selected"}
		return target{}, false
	}
	return newTarget(p), true
}

// refuseSelf reports whether t is the monitor itself, and says why nothing
// will happen: ending, freezing or reprioritising the monitor from its own
// screen is never what the user wants.
func (m *Model) refuseSelf(t target, verb string) bool {
	if t.pid != m.selfPID {
		return false
	}
	m.notice = notice{text: fmt.Sprintf("refusing to %s procmon itself (PID %d)", verb, t.pid), isErr: true}
	return true
}

// startAction handles x, X, s and r: it captures the target and asks y/n.
func (m *Model) startAction(key string) {
	a := actions[key]
	t, ok := m.actionTarget()
	if !ok || m.refuseSelf(t, a.verb) {
		return
	}
	m.mode, m.pending = modeConfirm, pending{act: a, who: t}
}

// perform sends the signal. The snapshot on screen may be a second old and PIDs
// are reused, so first it reads the process again and checks that the PID
// still belongs to the process the user saw.
func (m *Model) perform(a action, t target) {
	if m.refuseSelf(t, a.verb) {
		return
	}
	can, failed := "send "+a.signal, fmt.Sprintf("%s to %s failed", a.signal, t)
	if err := m.mon.Verify(t.pid, t.startTime); err != nil {
		m.notice = failure(err, t, can, "cannot verify "+t.String())
		return
	}
	if err := a.send(m.ctl, t.pid); err != nil {
		m.notice = failure(err, t, can, failed)
		return
	}
	m.notice = notice{text: fmt.Sprintf("%s sent to %s — %s", a.signal, t, a.done)}
}

// failure turns an error of the act or read doors into a message for the
// status line. can is what was not allowed ("send SIGSTOP") and failed how
// to say it did not work.
func failure(err error, t target, can, failed string) notice {
	switch {
	case errors.Is(err, oserr.ErrProcessGone):
		return notice{text: fmt.Sprintf("process %d no longer exists", t.pid), isErr: true}
	case errors.Is(err, oserr.ErrPermission):
		return notice{text: fmt.Sprintf("permission denied: only root or the owner of %s can %s", t, can), isErr: true}
	}
	return notice{text: fmt.Sprintf("%s: %s", failed, clean(err.Error())), isErr: true}
}

// startNice opens the prompt for a new nice value, showing the current one.
func (m *Model) startNice() {
	t, ok := m.actionTarget()
	if !ok || m.refuseSelf(t, "change the priority of") {
		return
	}
	cur, err := m.ctl.Priority(t.pid)
	if err != nil {
		m.notice = failure(err, t, "read the nice of "+t.String(), fmt.Sprintf("reading the nice of %s failed", t))
		return
	}
	m.pending = pending{who: t, nice: cur}
	m.startInput(modeNice, "")
}

// parseNice reads what the user typed: a whole number from -20 to 19.
func parseNice(text string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0, errors.New("not a whole number")
	}
	if n < minNice || n > maxNice {
		return 0, fmt.Errorf("must be between %d and %d", minNice, maxNice)
	}
	return n, nil
}

// Limits of the nice value, the ones setpriority(2) accepts.
const (
	minNice = -20 // highest priority
	maxNice = 19  // lowest priority
)

// commitNice applies what was typed in the nice prompt. An invalid value is
// reported in the status line and changes nothing.
func (m *Model) commitNice(p pending, text string) {
	n, err := parseNice(text)
	if err != nil {
		m.notice = notice{text: fmt.Sprintf("invalid nice %q: %v", clean(text), err), isErr: true}
		return
	}
	t := p.who
	if m.refuseSelf(t, "change the priority of") {
		return
	}
	if err := m.mon.Verify(t.pid, t.startTime); err != nil {
		m.notice = failure(err, t, "change the nice of "+t.String(), "cannot verify "+t.String())
		return
	}
	if err := m.ctl.SetPriority(t.pid, n); err != nil {
		if errors.Is(err, oserr.ErrPermission) {
			m.notice = notice{text: "permission denied: lowering nice needs root (CAP_SYS_NICE); only the owner can raise it", isErr: true}
			return
		}
		m.notice = failure(err, t, "", fmt.Sprintf("setting nice %d on %s failed", n, t))
		return
	}
	m.notice = notice{text: fmt.Sprintf("nice of %s set from %d to %d", t, p.nice, n)}
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
