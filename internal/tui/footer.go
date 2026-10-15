package tui

import (
	"strings"

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
	return m.helpLine(w)
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
		if m.refreshErr != nil {
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
	case m.refreshErr != nil:
		return "refresh failed: " + clean(m.refreshErr.Error())
	}
	return ""
}
