package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EliabParra/procmon/internal/monitor"
)

// detail is the open detail screen. PID and start time together identify the
// process: the same PID with another start time is a different process.
type detail struct {
	pid       int
	startTime uint64
	last      monitor.Process // the last row seen, to name the process once it is gone
}

// snapProc finds a process of the last snapshot.
func (m *Model) snapProc(pid int) (monitor.Process, bool) {
	i := slices.IndexFunc(m.snap.Processes, func(p monitor.Process) bool { return p.PID == pid })
	if i < 0 {
		return monitor.Process{}, false
	}
	return m.snap.Processes[i], true
}

// selectedProc is the selected row, if there is one.
func (m *Model) selectedProc() (monitor.Process, bool) {
	if len(m.rows) == 0 {
		return monitor.Process{}, false
	}
	return m.rows[m.cursor], true
}

// openDetail shows the detail of the selected process. The selection is kept
// on it, so esc lands on the same row whatever the order becomes meanwhile.
func (m *Model) openDetail() tea.Cmd {
	p, ok := m.selectedProc()
	if !ok {
		m.notice = notice{text: "no process selected"}
		return nil
	}
	m.picked = true
	m.view = viewDetail
	m.detail = detail{pid: p.PID, startTime: p.StartTime, last: p}
	return nil
}

func (m *Model) closeDetail() {
	m.view = viewTable
	m.detail = detail{}
	m.scrollToCursor()
}

// trackDetail remembers the open process as the last snapshot shows it.
func (m *Model) trackDetail() {
	if m.view != viewDetail {
		return
	}
	if p, ok := m.snapProc(m.detail.pid); ok && p.StartTime == m.detail.startTime {
		m.detail.last = p
	}
}

// detailProc is the open process as the last snapshot shows it. alive is false
// when it exited, or when its PID now belongs to another process; the row is
// then the last one seen.
func (m *Model) detailProc() (p monitor.Process, alive bool) {
	p, ok := m.snapProc(m.detail.pid)
	if ok {
		return p, true
	}
	return m.detail.last, false
}

// detailRows is how many lines the detail box holds inside its borders.
func (m *Model) detailRows() int { return m.layout().tableRows + 1 }

// detailKey handles the keys of the detail screen: esc or enter goes back.
func (m *Model) detailKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.closeDetail()
	}
	return m, nil
}

// detailBox is the rounded box of the detail screen, n lines tall inside.
func (m *Model) detailBox(w, n int) []string {
	return m.frame(w, m.styles.dim.Render("detail"), m.styles.dim.Render(strconv.Itoa(m.detail.pid)), m.detailLines(w-4, n))
}

// detailLines are exactly n lines: who the process is.
func (m *Model) detailLines(w, n int) []string {
	var lines []string
	p, _ := m.detailProc()
	lines = append(lines, m.identityLines(p)...)
	out := make([]string, n)
	copy(out, lines)
	return out
}

func (m *Model) kv(label, value string) string {
	return m.styles.dim.Render(label) + " " + m.styles.text.Render(value)
}

// identityLines are who the process is: the numbers of the table row, then
// the command line.
func (m *Model) identityLines(p monitor.Process) []string {
	first := strings.Join([]string{
		m.kv("PID", strconv.Itoa(p.PID)),
		m.styles.dim.Render("state") + " " + m.stateLabel(p.State),
		m.kv("user", clean(p.User)),
		m.kv("nice", strconv.Itoa(p.Nice)),
		m.kv("PPID", strconv.Itoa(p.PPID)),
		m.kv("threads", strconv.Itoa(p.Threads)),
		m.kv("cpu", fmt.Sprintf("%.1f", p.CPUPercent)+"%"),
	}, gap)
	return []string{first, m.styles.dim.Render("cmd") + " " + m.styles.text.Render(clean(p.Command))}
}

// stateLabel is a state letter and its name, in the colour of the table.
func (m *Model) stateLabel(state byte) string {
	text := clean(string(rune(state))) + " " + stateName(state)
	if hot, ok := m.styles.state(state); ok {
		return lipgloss.NewStyle().Foreground(hot.color).Bold(true).Render(text)
	}
	return m.styles.text.Render(text)
}

func stateName(state byte) string {
	switch state {
	case 'R':
		return "running"
	case 'S':
		return "sleeping"
	case 'D':
		return "disk sleep"
	case 'Z':
		return "zombie"
	case 'T':
		return "stopped"
	case 't':
		return "tracing stop"
	case 'I':
		return "idle"
	case 'X', 'x':
		return "dead"
	default:
		return "unknown"
	}
}
