package tui

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/EliabParra/procmon/internal/monitor"
)

// DefaultInterval is the refresh interval when none is chosen.
const DefaultInterval = time.Second

// warmup is how soon the second refresh comes after start: %CPU needs two
// samples, so waiting a whole interval would leave the column at zero.
const warmup = 500 * time.Millisecond

// Messages the model sends itself.
type (
	// tickMsg fires when the refresh interval has passed. gen tells which tick
	// chain it belongs to, so a tick scheduled before an interval change is
	// recognised as stale and dropped.
	tickMsg struct{ gen int }
	// refreshedMsg carries the result of one Monitor.Refresh.
	refreshedMsg struct {
		snap monitor.Snapshot
		err  error
	}
)

// Model is the Bubble Tea model of the whole screen.
type Model struct {
	mon    Monitor
	ctl    Control
	styles styles

	width, height int

	interval   time.Duration
	gen        int  // current tick chain
	refreshing bool // a Refresh is running; never start a second one

	snap       monitor.Snapshot
	haveSnap   bool
	refreshErr error // the last Refresh failed; the old snapshot stays on screen

	sortKey monitor.SortKey

	rows   []monitor.Process // the table: filtered, sorted and, in the tree, in tree order
	cursor int               // index in rows of the selected row
	offset int               // index in rows of the first visible row
	selPID int               // the selected process; the cursor follows it once picked
}

// Option customizes a Model.
type Option func(*Model)

// New returns a Model that reads through mon and acts through ctl.
func New(mon Monitor, ctl Control, opts ...Option) *Model {
	m := &Model{
		mon:      mon,
		ctl:      ctl,
		styles:   newStyles(false),
		interval: DefaultInterval,
		sortKey:  monitor.ByCPU,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Init starts the first refresh and the tick that keeps them coming.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), tickAfter(min(m.interval, warmup), m.gen))
}

// Update handles one message. It never blocks: Refresh runs inside a command.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scrollToCursor()
	case tickMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		return m, tea.Batch(m.refreshCmd(), tickAfter(m.interval, m.gen))
	case refreshedMsg:
		m.refreshing = false
		m.refreshErr = msg.err
		if msg.err == nil {
			m.snap, m.haveSnap = msg.snap, true
			m.rebuild()
		}
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// refreshCmd reads the system in the background. It returns nil while a
// refresh is already running, so a slow /proc cannot pile requests up.
func (m *Model) refreshCmd() tea.Cmd {
	if m.refreshing {
		return nil
	}
	m.refreshing = true
	mon := m.mon
	return func() tea.Msg {
		snap, err := mon.Refresh()
		msg := refreshedMsg{snap: snap, err: err}
		return msg
	}
}

func tickAfter(d time.Duration, gen int) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{gen: gen} })
}

// rebuild recomputes the table from the last snapshot: it sorts the rows.
func (m *Model) rebuild() {
	matched := slices.Clone(m.snap.Processes)

	rows := matched
	monitor.Sort(rows, m.sortKey)

	m.rows = rows

	m.moveTo(m.cursor)
}

// selected is the PID of the selected row, or 0 when the table is empty.
func (m *Model) selected() int { return m.selPID }

// moveTo puts the cursor on row i (clamped) and scrolls it into view.
func (m *Model) moveTo(i int) {
	if len(m.rows) == 0 {
		m.cursor, m.offset, m.selPID = 0, 0, 0
		return
	}
	m.cursor = min(max(i, 0), len(m.rows)-1)
	m.selPID = m.rows[m.cursor].PID
	m.scrollToCursor()
}

// scrollToCursor moves the visible window the least needed to show the cursor.
func (m *Model) scrollToCursor() {
	n := m.layout().tableRows
	if n <= 0 {
		m.offset = m.cursor
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+n {
		m.offset = m.cursor - n + 1
	}
	m.offset = min(max(m.offset, 0), max(0, len(m.rows)-n))
}
