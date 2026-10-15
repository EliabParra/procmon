package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/EliabParra/procmon/internal/monitor"
)

// handleKey routes a key press by mode. Ctrl+C quits from any mode, so there
// is always a way out through tea.Quit, which restores the terminal.
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	m.notice = notice{} // a message lasts until the next key
	switch m.mode {
	case modeFilter, modeInterval:
		return m.inputKey(msg)
	case modeConfirm:
		return m.confirmKey(msg)
	default:
		return m.normalKey(msg)
	}
}

// normalKey handles the hot keys: one press, no Enter. The ones that mean the
// same on every screen come first; the rest depend on the screen.
func (m *Model) normalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "i":
		m.startInput(modeInterval, "")
	case "+":
		return m, m.stepInterval(+1)
	case "-":
		return m, m.stepInterval(-1)
	case "x", "X", "s", "r":
		m.startAction(msg.String())
	default:
		if m.view == viewDetail {
			return m.detailKey(msg)
		}
		return m.tableKey(msg)
	}
	return m, nil
}

// tableKey handles the keys of the process table.
func (m *Model) tableKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	page := max(1, m.layout().tableRows)
	switch msg.String() {
	case "up", "k":
		m.pick(m.cursor - 1)
	case "down", "j":
		m.pick(m.cursor + 1)
	case "pgup":
		m.pick(m.cursor - page)
	case "pgdown":
		m.pick(m.cursor + page)
	case "home":
		m.moveTo(0)
		m.picked = false // the top row again, whatever the order becomes
	case "end":
		m.pick(len(m.rows) - 1)
	case "enter":
		return m, m.openDetail()
	case "c":
		m.setSort(monitor.ByCPU)
	case "m":
		m.setSort(monitor.ByMem)
	case "p":
		m.setSort(monitor.ByPID)
	case "t":
		m.tree = !m.tree
		m.rebuild()
	case "/":
		m.startInput(modeFilter, m.filter)
	}
	return m, nil
}

func (m *Model) setSort(k monitor.SortKey) {
	m.sortKey = k
	m.rebuild()
}

func (m *Model) startInput(to mode, text string) {
	m.mode = to
	m.input.Set(text)
}

// stepInterval moves the refresh interval to the next or previous step.
func (m *Model) stepInterval(dir int) tea.Cmd {
	next, ok := nextInterval(m.interval, dir)
	if !ok {
		which := "longest"
		if dir < 0 {
			which = "shortest"
		}
		m.notice = notice{text: "already at the " + which + " step"}
		return nil
	}
	return m.setInterval(next)
}

// inputKey edits the line in modeFilter and modeInterval.
func (m *Model) inputKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return m.commitInput()
	case "esc":
		m.endInput()
	case "backspace":
		m.input.Backspace()
	case "ctrl+u":
		m.input.Reset()
	default:
		if msg.Text != "" {
			m.input.Insert(msg.Text)
		}
	}
	return m, nil
}

func (m *Model) endInput() {
	m.mode = modeNormal
	m.input.Reset()
}

// commitInput applies what was typed. An empty filter clears it; an invalid
// interval or nice is reported in the status line and changes nothing.
func (m *Model) commitInput() (tea.Model, tea.Cmd) {
	from, text := m.mode, m.input.String()
	m.endInput()

	switch from {
	case modeFilter:
		m.filter = strings.TrimSpace(text)
		m.rebuild()
	default:
		d, err := parseInterval(text)
		if err != nil {
			m.notice = notice{text: fmt.Sprintf("invalid interval %q: %v", text, err), isErr: true}
			return m, nil
		}
		return m, m.setInterval(d)
	}
	return m, nil
}
