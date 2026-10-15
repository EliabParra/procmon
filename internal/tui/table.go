package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/EliabParra/procmon/internal/monitor"
)

type colID int

const (
	colPID colID = iota
	colPPID
	colUser
	colState
	colCPU
	colMem
	colRes
	colThr
	colPri
	colNice
	colTime
	colCommand
)

// column describes one column of the table.
type column struct {
	id    colID
	title string
	width int  // 0 for COMMAND, which takes what the others leave
	right bool // numbers line up on the right
	drop  int  // 0 never goes; on a narrow terminal the lowest number goes first
}

var columns = []column{
	{colPID, "PID", 7, true, 0},
	{colPPID, "PPID", 7, true, 1},
	{colUser, "USER", 10, false, 4},
	{colState, "S", 1, false, 0},
	{colCPU, "%CPU", 6, true, 0},
	{colMem, "%MEM", 5, true, 7},
	{colRes, "RES", 5, true, 6},
	{colThr, "THR", 5, true, 2},
	{colPri, "PRI", 3, true, 3},
	{colNice, "NI", 3, true, 0},
	{colTime, "TIME", 9, true, 5},
	{colCommand, "COMMAND", 0, false, 0},
}

// minCommand is the least room worth giving the command before dropping
// another column.
const minCommand = 12

// fitColumns chooses the columns that fit in w and the width COMMAND gets.
// Columns are dropped (least important first) until the command has
// minCommand columns or nothing else can go.
func fitColumns(w int) (cols []column, commandWidth int) {
	cols = slices.Clone(columns)
	for {
		fixed := len(cols) - 1 // the gaps
		for _, c := range cols {
			fixed += c.width
		}
		commandWidth = w - fixed
		if commandWidth >= minCommand {
			return cols, commandWidth
		}
		victim := -1
		for i, c := range cols {
			if c.drop > 0 && (victim < 0 || c.drop < cols[victim].drop) {
				victim = i
			}
		}
		if victim < 0 {
			return cols, max(0, commandWidth)
		}
		cols = slices.Delete(cols, victim, victim+1)
	}
}

// cellText is the text of one field of a process, before padding.
func (m *Model) cellText(p monitor.Process, id colID) string {
	switch id {
	case colPID:
		return strconv.Itoa(p.PID)
	case colPPID:
		return strconv.Itoa(p.PPID)
	case colUser:
		return clean(p.User)
	case colState:
		return clean(string(rune(p.State)))
	case colCPU:
		return fmt.Sprintf("%.1f", p.CPUPercent)
	case colMem:
		return fmt.Sprintf("%.1f", p.MemPercent)
	case colRes:
		return humanKB(p.RSSKB)
	case colThr:
		return strconv.Itoa(p.Threads)
	case colPri:
		if p.Priority < -99 { // real-time priorities do not fit the column
			return "rt"
		}
		return strconv.Itoa(p.Priority)
	case colNice:
		return strconv.Itoa(p.Nice)
	case colTime:
		return formatCPUTime(p.CPUTime)
	default:
		return clean(p.Command)
	}
}

// sortColumn is the column the table is sorted by, and the arrow to show on it.
func (m *Model) sortColumn() (colID, string) {
	switch m.sortKey {
	case monitor.ByMem:
		return colMem, "▼"
	case monitor.ByPID:
		return colPID, "▲"
	default:
		return colCPU, "▼"
	}
}

// tableLines returns the column titles and exactly n body lines, all w
// columns wide: the visible window of the rows, then blank lines.
func (m *Model) tableLines(w, n int) (head string, body []string) {
	cols, commandWidth := fitColumns(w)
	width := func(c column) int {
		if c.id == colCommand {
			return commandWidth
		}
		return c.width
	}

	sorted, arrow := m.sortColumn()
	titles := make([]string, len(cols))
	for i, c := range cols {
		title := c.title
		style := m.styles.head
		if c.id == sorted {
			title += arrow
			style = m.styles.accent.Bold(true)
		}
		titles[i] = style.Render(cell(title, width(c), c.right))
	}
	head = strings.Join(titles, " ")

	body = make([]string, n)
	if len(m.rows) == 0 {
		if n > 0 {
			body[0] = m.styles.dim.Render(m.emptyMessage())
		}
		return head, body
	}
	for i := range body {
		at := m.offset + i
		if at >= len(m.rows) {
			break
		}
		p := m.rows[at]
		cells := make([]string, len(cols))
		for j, c := range cols {
			text := m.cellText(p, c.id)
			if c.id == colCommand && at < len(m.prefix) {
				text = m.prefix[at] + text // the tree connectors
			}
			cells[j] = cell(text, width(c), c.right)
		}
		body[i] = m.styleRow(p, cells, cols, at == m.cursor)
	}
	return head, body
}

// styleRow joins the cells of one row and colours it. Zombies, processes
// blocked on I/O and stopped ones are tinted, and their state cell becomes a
// solid chip; the selected row keeps its background under either look, and a
// tree row that is only an ancestor of a match is dimmed.
func (m *Model) styleRow(p monitor.Process, cells []string, cols []column, selected bool) string {
	base := lipgloss.NewStyle()
	if selected {
		base = m.styles.selected
	}
	if m.context[p.PID] && !selected {
		base = m.styles.dim // an ancestor shown only for context
	}
	at := slices.IndexFunc(cols, func(c column) bool { return c.id == colState })
	return m.paint(p.State, cells, at, base)
}

// paint joins cells with base as their style. When the state is one the table
// highlights, the text takes its colour and the cell at index at (the state
// letter) becomes a solid chip.
func (m *Model) paint(state byte, cells []string, at int, base lipgloss.Style) string {
	hot, isHot := m.styles.state(state)
	if isHot {
		base = base.Foreground(hot.color)
	}
	if !isHot || at < 0 || at >= len(cells) {
		return base.Render(strings.Join(cells, " "))
	}

	// Three runs instead of one per cell: what comes before the state cell,
	// the chip, and what comes after.
	var before, after string
	if at > 0 {
		before = base.Render(strings.Join(cells[:at], " ") + " ")
	}
	if at < len(cells)-1 {
		after = base.Render(" " + strings.Join(cells[at+1:], " "))
	}
	return before + hot.chip.Render(cells[at]) + after
}

func (m *Model) emptyMessage() string {
	if m.filter != "" {
		return fmt.Sprintf("no processes match %q", clean(m.filter))
	}
	return "no processes to show"
}

// tableBox is the rounded box around the table; the position of the selected
// row is in its top border.
func (m *Model) tableBox(w, n int) []string {
	head, body := m.tableLines(w-4, n)
	pos := ""
	if len(m.rows) > 0 {
		pos = m.styles.dim.Render(fmt.Sprintf("%d/%d", m.cursor+1, len(m.rows)))
	}
	return m.frame(w, m.styles.dim.Render("processes"), pos, append([]string{head}, body...))
}
