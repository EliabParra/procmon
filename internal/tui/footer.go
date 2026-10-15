package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// hint is one entry of the help line: a key and what it does.
type hint struct{ key, desc string }

// normalHints lists the keys of the table that work, most important first.
// The help line shows as many as fit.
var normalHints = []hint{
	{"q", "quit"},
	{"↑/↓", "move"},
	{"c", "cpu"},
	{"m", "mem"},
	{"p", "pid"},
	{"/", "filter"},
	{"+/-", "interval"},
	{"i", "set interval"},
	{"pgup/pgdn", "page"},
	{"home/end", "jump"},
}

// hints are the keys the current screen answers to.
func (m *Model) hints() []hint {
	return normalHints
}

// footer is the single line at the bottom: the input line while typing, the
// question while one is open, otherwise a message (if any) followed by the keys
// that fit.
func (m *Model) footer(w int) string {
	switch {
	case m.typing():
		return m.promptLine(w)
	default:
		return m.helpLine(w)
	}
}

// helpLine shows the hot keys, dropping the last ones when the line is short.
func (m *Model) helpLine(w int) string {
	var parts []string
	used := 0
	add := func(s string) {
		width := ansi.StringWidth(s)
		if len(parts) > 0 {
			width += len(gap)
		}
		if used+width > w && len(parts) > 0 {
			return
		}
		parts = append(parts, s)
		used += width
	}
	if msg := m.statusText(); msg != "" {
		style := m.styles.dim
		if m.refreshErr != nil || m.notice.isErr {
			style = m.styles.err
		}
		if ansi.StringWidth(msg) > w {
			msg = ansi.Truncate(msg, w, "…")
		}
		parts = append(parts, style.Render(msg))
		used = ansi.StringWidth(msg)
	}
	for _, h := range m.hints() {
		add(m.styles.accent.Render(h.key) + " " + m.styles.dim.Render(h.desc))
	}
	return strings.Join(parts, gap)
}

// statusText is the message shown before the help: the last notice, or else
// the reason the last refresh failed.
func (m *Model) statusText() string {
	switch {
	case m.notice.text != "":
		return clean(m.notice.text)
	case m.refreshErr != nil:
		return "refresh failed: " + clean(m.refreshErr.Error())
	}
	return ""
}

// promptLabel is what the input line asks for on a line w columns wide. The
// nice question is long, so on a narrow line it shrinks to "nice 4321 (name)
// [0]:" (the number in brackets is the current nice) and, if that is still too
// long, loses the end of the name: the typed value must always have room.
func (m *Model) promptLabel(w int) string {
	switch m.mode {
	case modeInterval:
		return "interval (seconds):"
	}
	return "/"
}

// promptParts are the label and the part of the typed text that is visible
// on a line w columns wide: its end, if it does not fit.
func (m *Model) promptParts(w int) (label, text string) {
	label, text = m.promptLabel(w), m.input.String()
	room := max(0, w-ansi.StringWidth(label)-2) // the space after the label and the cursor
	if ansi.StringWidth(text) > room {
		text = ansi.TruncateLeft(text, ansi.StringWidth(text)-room+1, "…")
	}
	return label, text
}

// promptLine is the input line: the label, the text typed so far and, when
// there is room, how to finish.
func (m *Model) promptLine(w int) string {
	label, text := m.promptParts(w)
	line := m.styles.accent.Render(label) + " " + m.styles.text.Render(text)

	const hints = "enter apply  esc cancel"
	if used := ansi.StringWidth(label) + 1 + ansi.StringWidth(text) + 1; used+len(gap)+len(hints) <= w {
		line += strings.Repeat(" ", w-used-len(hints)+1) + m.styles.dim.Render(hints)
	}
	return line
}

// inputCursor places the terminal cursor at the end of the typed text while an
// input is open, and hides it otherwise.
func (m *Model) inputCursor() *tea.Cursor {
	if !m.typing() {
		return nil
	}
	label, text := m.promptParts(m.width)
	x := ansi.StringWidth(label) + 1 + ansi.StringWidth(text)
	c := tea.NewCursor(min(x, m.width-1), m.height-1)
	c.Shape = tea.CursorBar
	return c
}
