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

// detailFixed is how many lines of the detail box come before the thread rows:
// identity (2), blank, chart (2), blank, thread title and column titles.
const detailFixed = 8

// detailData is what the detail screen reads about one process on every
// refresh: its CPU history and its threads.
type detailData struct {
	pid     int // 0 when nothing has been read yet
	history []float64
	threads []monitor.Thread
}

// readDetail reads the detail of pid. It runs inside a command, off the
// update loop, because listing the threads of a big process takes a while.
func readDetail(mon Monitor, pid int) detailData {
	threads, _ := mon.Threads(pid)
	return detailData{pid: pid, history: mon.History(pid), threads: threads}
}

// detailMsg carries the detail read right after the screen was opened, so it
// does not wait for the next refresh.
type detailMsg detailData

// detail is the open detail screen. PID and start time together identify the
// process: the same PID with another start time is a different process.
type detail struct {
	pid       int
	startTime uint64
	last      monitor.Process // the last row seen, to name the process once it is gone
	data      detailData
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
	mon := m.mon
	return func() tea.Msg { return detailMsg(readDetail(mon, p.PID)) }
}

func (m *Model) closeDetail() {
	m.view = viewTable
	m.detail = detail{}
	m.scrollToCursor()
}

// watchedPID is the process whose detail the next refresh must read, or 0.
func (m *Model) watchedPID() int {
	if m.view == viewDetail {
		return m.detail.pid
	}
	return 0
}

// applyDetail stores freshly read detail data, unless the screen has moved on.
func (m *Model) applyDetail(d detailData) {
	if m.view == viewDetail && d.pid != 0 && d.pid == m.detail.pid {
		m.detail.data = d
	}
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
	if ok && p.StartTime == m.detail.startTime {
		return p, true
	}
	return m.detail.last, false
}

// detailRows is how many lines the detail box holds inside its borders.
func (m *Model) detailRows() int { return m.layout().tableRows + 1 }

// threadRows is how many threads fit on screen.
func (m *Model) threadRows() int { return max(0, m.detailRows()-detailFixed) }

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

// detailLines are exactly n lines: who the process is, its CPU history and its
// threads, or the news that it is gone.
func (m *Model) detailLines(w, n int) []string {
	var lines []string
	if p, alive := m.detailProc(); alive {
		lines = append(lines, m.identityLines(p)...)
		lines = append(lines, "")
		lines = append(lines, m.chartLines(w)...)
		lines = append(lines, "")
		lines = append(lines, m.threadLines(w)...)
	} else {
		lines = m.goneLines(p)
	}
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

// chartLines are the title with the figures of the series and the sparkline
// of the CPU history, as wide as the box.
func (m *Model) chartLines(w int) []string {
	hist := m.detail.data.history
	title := m.styles.dim.Render("CPU history")
	if len(hist) == 0 {
		return []string{title, m.styles.dim.Render("no samples yet")}
	}
	lo, avg, hi := seriesStats(hist)
	top := sparkScale(hist, 100)
	pct := func(v float64) string { return fmt.Sprintf("%.1f%%", v) }
	title += gap + strings.Join([]string{
		m.kv("min", pct(lo)),
		m.kv("avg", pct(avg)),
		m.kv("max", pct(hi)),
		m.styles.dim.Render(fmt.Sprintf("%d samples", len(hist))),
		m.styles.dim.Render(fmt.Sprintf("scale 0-%.0f%%", top)), // the steps are whole numbers
	}, gap)
	return []string{title, m.styles.accent.Render(sparkline(hist, w, top))}
}

// threadLines are the thread list: its title, the column titles and the rows
// that fit from the scroll position, or why there is nothing to list.
func (m *Model) threadLines(w int) []string {
	d := m.detail.data
	title := m.styles.dim.Render("Threads")
	switch {
	case d.pid == 0:
		return []string{title, m.styles.dim.Render("reading threads…")}
	}

	rows := m.threadRows()
	switch {
	case rows == 0 && len(d.threads) > 0:
		title += " " + m.styles.text.Render(fmt.Sprintf("(%d, enlarge the window)", len(d.threads)))
	case len(d.threads) > rows:
		first := 1
		last := min(rows, len(d.threads))
		title += " " + m.styles.text.Render(fmt.Sprintf("%d-%d of %d", first, last, len(d.threads)))
	default:
		title += " " + m.styles.text.Render(fmt.Sprintf("(%d)", len(d.threads)))
	}

	const tidW, stateW, timeW = 7, 1, 9
	nameW := w - tidW - stateW - timeW - 3
	lines := []string{title, m.styles.head.Render(strings.Join([]string{
		cell("TID", tidW, true), cell("S", stateW, false), cell("TIME", timeW, true), cell("NAME", nameW, false),
	}, " "))}
	for i := 0; i < len(d.threads) && i < rows; i++ {
		t := d.threads[i]
		lines = append(lines, m.paint(t.State, []string{
			cell(strconv.Itoa(t.TID), tidW, true),
			cell(clean(string(rune(t.State))), stateW, false),
			cell(formatCPUTime(t.CPUTime), timeW, true),
			cell(clean(t.Comm), nameW, false),
		}, 1, lipgloss.NewStyle()))
	}
	return lines
}

// goneLines say that the process is no more, and what it was.
func (m *Model) goneLines(last monitor.Process) []string {
	lines := []string{m.styles.err.Render(fmt.Sprintf("process %d no longer exists", m.detail.pid))}
	if last.Command != "" {
		lines = append(lines, m.styles.dim.Render("it was "+clean(last.Command)))
	}
	return append(lines, "", m.styles.dim.Render("press esc to go back to the table"))
}
