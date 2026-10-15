package tui

import tea "charm.land/bubbletea/v2"

// handleKey routes a key press by mode. Ctrl+C quits from any mode, so there
// is always a way out through tea.Quit, which restores the terminal.
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	return m.normalKey(msg)
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
		m.moveTo(m.cursor - 1)
	case "down", "j":
		m.moveTo(m.cursor + 1)
	case "pgup":
		m.moveTo(m.cursor - page)
	case "pgdown":
		m.moveTo(m.cursor + page)
	case "home":
		m.moveTo(0)
	case "end":
		m.moveTo(len(m.rows) - 1)
	}
	return m, nil
}
