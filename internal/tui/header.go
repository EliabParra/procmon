package tui

import (
	"fmt"
	"strings"
)

const (
	gap = "  " // between the items of a summary line
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

// summaryLines are the lines of the header; the first one is also the whole
// header of a small terminal.
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

	return []string{tasks, "", ""}
}
