package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Sizes of the fixed parts of the screen.
const (
	headerBoxRows = 5 // two borders and three summary lines
	tableChrome   = 3 // two borders and the column titles
	footerRows    = 1

	minBoxedWidth  = 40
	minBoxedHeight = 12
)

// layout is how the terminal is split. Big terminals get the rounded boxes;
// small ones get a plain summary line and table so the rows still fit.
type layout struct {
	boxed     bool
	tableRows int // process rows that fit on screen
}

func (m *Model) layout() layout {
	if m.width >= minBoxedWidth && m.height >= minBoxedHeight {
		return layout{boxed: true, tableRows: m.height - headerBoxRows - tableChrome - footerRows}
	}
	// summary line, column titles and footer
	return layout{tableRows: max(0, m.height-3)}
}

// View draws the whole terminal: exactly height lines, none wider than the
// width, so a redraw always overwrites what was there before.
func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "procmon"
	return v
}

func (m *Model) render() string {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		return ""
	}
	l := m.layout()
	var lines []string
	if l.boxed {
		lines = append(lines, m.headerBox(w)...)
	} else {
		lines = append(lines, m.summaryLines(w)[0])
	}
	return fitLines(lines, w, h)
}

// fitLines makes the output exactly h lines of at most w columns each.
func fitLines(lines []string, w, h int) string {
	out := make([]string, h)
	for i := range out {
		if i >= len(lines) {
			continue
		}
		out[i] = lines[i]
		if ansi.StringWidth(out[i]) > w {
			out[i] = ansi.Truncate(out[i], w, "")
		}
	}
	return strings.Join(out, "\n")
}

// fill cuts or pads a styled line to exactly w columns.
func fill(s string, w int) string {
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", max(0, w-ansi.StringWidth(s)))
}

// frame draws a rounded box w columns wide around body, whose lines are
// styled and get cut or padded to the inside width. left and right sit in the
// top border.
func (m *Model) frame(w int, left, right string, body []string) []string {
	b := lipgloss.RoundedBorder()
	side := m.styles.border.Render(b.Left)
	sideEnd := m.styles.border.Render(b.Right)

	out := make([]string, 0, len(body)+2)
	out = append(out, m.topBorder(w, left, right))
	for _, l := range body {
		out = append(out, side+" "+fill(l, w-4)+" "+sideEnd)
	}
	out = append(out, m.styles.border.Render(b.BottomLeft+strings.Repeat(b.Bottom, w-2)+b.BottomRight))
	return out
}

// topBorder is ╭─ left ──────── right ─╮. When there is no room for both
// labels, the right one goes first, then the left one is cut.
func (m *Model) topBorder(w int, left, right string) string {
	b := lipgloss.RoundedBorder()
	segment := func(s string) int {
		if s == "" {
			return 0
		}
		return ansi.StringWidth(s) + 2
	}
	room := w - 4 // the corners and the dash beside each
	if segment(left)+segment(right) > room {
		right = ""
	}
	if segment(left) > room {
		left = ansi.Truncate(left, max(0, room-2), "…")
	}
	dashes := max(0, room-segment(left)-segment(right))

	line := func(s string) string {
		if s == "" {
			return ""
		}
		return " " + s + " "
	}
	bs := m.styles.border
	return bs.Render(b.TopLeft+b.Top) + line(left) +
		bs.Render(strings.Repeat(b.Top, dashes)) + line(right) +
		bs.Render(b.Top+b.TopRight)
}
