package tui

import (
	"fmt"
	"strings"
)

const (
	memBarWidth  = 10
	swapBarWidth = 8
	gap          = "  " // between the items of a summary line
)

// headerBox is the rounded box with the system summary. The refresh interval
// sits in its top border as a pill, the time of the last refresh at the
// right end.
func (m *Model) headerBox(w int) []string {
	left := m.styles.title.Render("procmon") + " " +
		m.styles.pill("refresh", formatInterval(m.interval))

	right := ""
	if m.haveSnap {
		right = m.styles.dim.Render(m.snap.TakenAt.Format("3:04:05 PM")) // 12-hour clock
	}
	return m.frame(w, left, right, m.summaryLines(w-4))
}

// summaryLines are the three lines of the header: task counts, load and
// uptime, memory and swap. The first one is also the whole header of a small
// terminal.
func (m *Model) summaryLines(w int) []string {
	if !m.haveSnap {
		return []string{m.styles.dim.Render("waiting for the first refresh…"), "", ""}
	}
	s := m.snap.Summary
	kv := func(label string, value any) string {
		return m.styles.dim.Render(label) + " " + m.styles.text.Render(fmt.Sprint(value))
	}

	// Idle kernel threads (state I) are sleepers too, and top counts them so;
	// that way the counts add up to the total.
	tasks := strings.Join([]string{
		kv("Tasks", s.Total),
		kv("running", s.Running),
		kv("sleeping", s.Sleeping+s.Idle),
		kv("disk-sleep", s.DiskSleep),
		kv("stopped", s.Stopped),
		kv("zombie", s.Zombie),
	}, gap)

	load := strings.Join([]string{
		kv("Load", fmt.Sprintf("%.2f %.2f %.2f", s.Load1, s.Load5, s.Load15)),
		kv("up", formatUptime(s.Uptime)),
	}, gap+gap)

	mem := strings.Join([]string{
		m.styles.dim.Render("Mem") + " " + m.bar(float64(s.MemUsedKB), float64(s.MemTotalKB), memBarWidth) + " " +
			m.styles.text.Render(humanKB(s.MemUsedKB)+" / "+humanKB(s.MemTotalKB)),
		kv("avail", humanKB(s.MemAvailableKB)),
		m.styles.dim.Render("Swap") + " " + m.bar(float64(s.SwapUsedKB), float64(s.SwapTotalKB), swapBarWidth) + " " +
			m.styles.text.Render(humanKB(s.SwapUsedKB)+" / "+humanKB(s.SwapTotalKB)),
	}, gap+gap)

	return []string{tasks, load, mem}
}

// bar draws used/total as filled and empty cells.
func (m *Model) bar(used, total float64, width int) string {
	frac := 0.0
	if total > 0 {
		frac = used / total
	}
	n := barCells(frac, width)
	return m.styles.barFull.Render(strings.Repeat("█", n)) +
		m.styles.barEmpty.Render(strings.Repeat("░", width-n))
}
