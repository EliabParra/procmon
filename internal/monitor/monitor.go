package monitor

import "time"

// Monitor takes snapshots of a Source.
type Monitor struct {
	src   Source
	users *Users
	now   func() time.Time
}

// Option customizes a Monitor.
type Option func(*Monitor)

// WithUsers sets the UID resolver (default: /etc/passwd).
func WithUsers(u *Users) Option { return func(m *Monitor) { m.users = u } }

// WithClock sets the time source used for Snapshot.TakenAt (default: time.Now).
func WithClock(now func() time.Time) Option { return func(m *Monitor) { m.now = now } }

// New returns a Monitor reading from src.
func New(src Source, opts ...Option) *Monitor {
	m := &Monitor{src: src, users: NewUsers("/etc/passwd"), now: time.Now}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Refresh reads every process and returns a Snapshot, processes in ascending
// PID order.
func (m *Monitor) Refresh() (Snapshot, error) {
	pids, err := m.src.PIDs()
	if err != nil {
		return Snapshot{}, err
	}
	mem, err := m.src.MemInfo()
	if err != nil {
		return Snapshot{}, err
	}

	hz := m.src.ClockTicks()

	procs := make([]Process, 0, len(pids))
	for _, pid := range pids {
		p, _, ok := m.readProcess(pid, hz, mem.MemTotalKB)
		if !ok {
			continue
		}
		procs = append(procs, p)
	}
	Sort(procs, ByPID)

	return Snapshot{
		Processes: procs,
		TakenAt:   m.now(),
	}, nil
}

// readProcess builds the row of one process without its %CPU, and returns its
// cumulative CPU ticks. ok is false when the process has to be left out: it
// exited, or its stat file (the base of every other field) cannot be used.
func (m *Monitor) readProcess(pid, hz int, memTotalKB uint64) (p Process, ticks uint64, ok bool) {
	st, err := m.src.Stat(pid)
	if err != nil {
		return Process{}, 0, false
	}

	user, rss := "?", uint64(0)
	if status, err := m.src.Status(pid); err == nil {
		user, rss = m.users.Name(status.UID), status.VmRSSKB
	}

	command, err := m.src.Cmdline(pid)
	if err != nil || command == "" {
		command = "[" + st.Comm + "]"
	}

	ticks = st.UTime + st.STime
	return Process{
		PID:        pid,
		PPID:       st.PPID,
		User:       user,
		State:      st.State,
		MemPercent: MemPercent(rss, memTotalKB),
		RSSKB:      rss,
		Threads:    st.NumThreads,
		Priority:   st.Priority,
		Nice:       st.Nice,
		CPUTime:    ticksToDuration(ticks, hz),
		Command:    command,
		Comm:       st.Comm,
		StartTime:  st.StartTime,
	}, ticks, true
}
