package tui

import (
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
	switch m.mode {
	case modeFilter:
		return m.inputKey(msg)
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
	default:
		return m.tableKey(msg)
	}
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
	case "c":
		m.setSort(monitor.ByCPU)
	case "m":
		m.setSort(monitor.ByMem)
	case "p":
		m.setSort(monitor.ByPID)
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
	}
	return m, nil
}
