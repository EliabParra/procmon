package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// The palette: low-contrast borders, one accent, a soft grey for pills and the
// selected row. Everything the screen colours comes from here.
var (
	colBorder = lipgloss.Color("#3b4261")
	colDim    = lipgloss.Color("#737aa2")
	colText   = lipgloss.Color("#c0caf5")
	colAccent = lipgloss.Color("#7aa2f7")
	colError  = lipgloss.Color("#f7768e")
	colPill   = lipgloss.Color("#292e42")
	colSelect = lipgloss.Color("#2f3549")

	// Process states that need an eye: each has its own colour.
	colZombie    = colError
	colDiskSleep = lipgloss.Color("#e0af68") // amber: blocked on I/O
	colStopped   = lipgloss.Color("#bb9af7") // violet: suspended
	colOnState   = lipgloss.Color("#1a1b26") // text on a state chip
)

// Rounded caps of a pill: Nerd Font half circles.
const (
	capLeft  = "\ue0b6"
	capRight = "\ue0b4"
)

// WithPlain draws the pills with square ends, for terminals whose font has
// no Nerd Font glyphs.
func WithPlain() Option {
	return func(m *Model) { m.styles = newStyles(true) }
}

type styles struct {
	border   lipgloss.Style // box lines
	dim      lipgloss.Style // labels and hints
	text     lipgloss.Style // values
	accent   lipgloss.Style // keys, the sorted column
	title    lipgloss.Style // the app name
	err      lipgloss.Style // error messages
	selected lipgloss.Style // the selected table row
	head     lipgloss.Style // table column titles
	barFull  lipgloss.Style
	barEmpty lipgloss.Style

	capLeft, capRight string
	pillCap           lipgloss.Style // the round ends
	pillLabel         lipgloss.Style // "refresh"
	pillValue         lipgloss.Style // "1s"
}

func newStyles(plain bool) styles {
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	s := styles{
		border:   fg(colBorder),
		dim:      fg(colDim),
		text:     fg(colText),
		accent:   fg(colAccent),
		title:    fg(colAccent).Bold(true),
		err:      fg(colError),
		selected: lipgloss.NewStyle().Foreground(colText).Background(colSelect).Bold(true),
		head:     fg(colDim).Bold(true),
		barFull:  fg(colAccent),
		barEmpty: fg(colBorder),

		capLeft:   capLeft,
		capRight:  capRight,
		pillCap:   fg(colPill),
		pillLabel: lipgloss.NewStyle().Foreground(colDim).Background(colPill),
		pillValue: lipgloss.NewStyle().Foreground(colAccent).Background(colPill).Bold(true),
	}
	if plain {
		s.capLeft, s.capRight = " ", " "
		s.pillCap = lipgloss.NewStyle().Background(colPill)
	}
	return s
}

// stateStyle is how one process state is drawn: its colour for text and a
// solid chip for the one-letter state cell.
type stateStyle struct {
	color color.Color
	chip  lipgloss.Style
}

// state returns the highlight of a process state, and false for the states
// that need none: zombies (Z), blocked on I/O (D) and stopped (T, t) stand
// out, everything else is drawn as usual.
func (s styles) state(b byte) (stateStyle, bool) {
	var c color.Color
	switch b {
	case 'Z':
		c = colZombie
	case 'D':
		c = colDiskSleep
	case 'T', 't':
		c = colStopped
	default:
		return stateStyle{}, false
	}
	return stateStyle{
		color: c,
		chip:  lipgloss.NewStyle().Foreground(colOnState).Background(c).Bold(true),
	}, true
}

// pill is a small label with round ends: a dim name and an accent value.
func (s styles) pill(label, value string) string {
	return s.pillCap.Render(s.capLeft) +
		s.pillLabel.Render(" "+label+" ") +
		s.pillValue.Render(value+" ") +
		s.pillCap.Render(s.capRight)
}
