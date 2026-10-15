package monitor

import (
	"errors"
	"fmt"
	"time"

	"github.com/EliabParra/procmon/internal/oserr"
	"github.com/EliabParra/procmon/internal/procfs"
)

// sample is what Refresh remembers about a process to measure it next time. The
// start time is part of the identity: a PID that comes back with another start
// time is a different process and must not be compared with the old one.
type sample struct {
	startTime uint64
	ticks     uint64 // utime + stime
	cpu       *ring  // %CPU of every refresh since this process was first seen
}

// Monitor takes snapshots of a Source and remembers the previous one, which is
// what turns cumulative CPU counters into a percentage.
type Monitor struct {
	src      Source
	users    *Users
	now      func() time.Time
	histSize int

	prev     map[int]sample // by PID, only processes seen in the last refresh
	prevSys  uint64         // total jiffies of /proc/stat at the last refresh
	havePrev bool
}

// Option customizes a Monitor.
type Option func(*Monitor)

// WithUsers sets the UID resolver (default: /etc/passwd).
func WithUsers(u *Users) Option { return func(m *Monitor) { m.users = u } }

// WithClock sets the time source used for Snapshot.TakenAt (default: time.Now).
func WithClock(now func() time.Time) Option { return func(m *Monitor) { m.now = now } }

// New returns a Monitor reading from src.
func New(src Source, opts ...Option) *Monitor {
	m := &Monitor{src: src, users: NewUsers("/etc/passwd"), now: time.Now, histSize: defaultHistory}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Refresh reads every process and the system counters and returns a Snapshot,
// processes in ascending PID order. %CPU is the growth of a process's
// utime+stime since the previous Refresh divided by the growth of the system's
// total jiffies (the course formula, a share of the whole machine), so the
// first call reports 0 for everything. A process that exits while being read is
// skipped; one whose status or command line is not readable keeps its row with
// what could be read. Each listed process also gets one more sample in its
// History, and the state of processes that are gone is dropped. Only a failure
// of a system-wide file returns an error, and then the previous baseline is
// kept.
func (m *Monitor) Refresh() (Snapshot, error) {
	pids, err := m.src.PIDs()
	if err != nil {
		return Snapshot{}, fmt.Errorf("monitor: list processes: %w", err)
	}
	sys, err := m.src.SystemStat()
	if err != nil {
		return Snapshot{}, fmt.Errorf("monitor: read cpu counters: %w", err)
	}
	mem, err := m.src.MemInfo()
	if err != nil {
		return Snapshot{}, fmt.Errorf("monitor: read memory: %w", err)
	}
	load, err := m.src.LoadAvg()
	if err != nil {
		return Snapshot{}, fmt.Errorf("monitor: read load average: %w", err)
	}
	uptime, err := m.src.Uptime()
	if err != nil {
		return Snapshot{}, fmt.Errorf("monitor: read uptime: %w", err)
	}

	hz := m.src.ClockTicks()
	cores := max(len(sys.Cores), 1)
	sysTotal := sys.CPU.Total()
	var sysDelta uint64
	if m.havePrev {
		sysDelta = sub(sysTotal, m.prevSys)
	}

	procs := make([]Process, 0, len(pids))
	next := make(map[int]sample, len(pids))
	for _, pid := range pids {
		p, ticks, ok := m.readProcess(pid, hz, mem.MemTotalKB)
		if !ok {
			continue
		}
		history := newRing(m.histSize)
		if old, seen := m.prev[pid]; seen && old.startTime == p.StartTime {
			p.CPUPercent = CPUPercent(sub(ticks, old.ticks), sysDelta)
			history = old.cpu
		}
		history.add(p.CPUPercent)
		next[pid] = sample{startTime: p.StartTime, ticks: ticks, cpu: history}
		procs = append(procs, p)
	}
	Sort(procs, ByPID)

	m.prev, m.prevSys, m.havePrev = next, sysTotal, true
	return Snapshot{
		Processes: procs,
		Summary:   summarize(procs, mem, load, uptime),
		Cores:     cores,
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
	status, err := m.src.Status(pid)
	switch {
	case errors.Is(err, oserr.ErrProcessGone):
		return Process{}, 0, false
	case err == nil:
		user, rss = m.users.Name(status.UID), status.VmRSSKB
	}

	command, err := m.src.Cmdline(pid)
	if errors.Is(err, oserr.ErrProcessGone) {
		return Process{}, 0, false
	}
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

// summarize counts the states of the processes that were read and copies the
// load, memory and uptime figures.
func summarize(procs []Process, mem procfs.MemInfo, load procfs.LoadAvg, uptime float64) Summary {
	s := Summary{
		Total:          len(procs),
		Load1:          load.One,
		Load5:          load.Five,
		Load15:         load.Fifteen,
		MemTotalKB:     mem.MemTotalKB,
		MemAvailableKB: mem.MemAvailableKB,
		MemUsedKB:      sub(mem.MemTotalKB, mem.MemAvailableKB),
		SwapTotalKB:    mem.SwapTotalKB,
		SwapUsedKB:     sub(mem.SwapTotalKB, mem.SwapFreeKB),
		Uptime:         time.Duration(uptime * float64(time.Second)),
	}
	for _, p := range procs {
		switch p.State {
		case 'R':
			s.Running++
		case 'S':
			s.Sleeping++
		case 'D':
			s.DiskSleep++
		case 'T', 't':
			s.Stopped++
		case 'Z':
			s.Zombie++
		case 'I':
			s.Idle++
		}
	}
	return s
}
