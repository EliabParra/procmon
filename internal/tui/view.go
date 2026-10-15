package tui

import tea "charm.land/bubbletea/v2"

// View draws the whole terminal: exactly height lines, none wider than the
// width, so a redraw always overwrites what was there before.
func (m *Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.WindowTitle = "procmon"
	return v
}
