package monitor

import (
	"errors"
	"testing"
	"time"

	"github.com/EliabParra/procmon/internal/oserr"
	"github.com/EliabParra/procmon/internal/procfs"
)

// fakeSource is a Source whose values the test edits between two Refresh
// calls. A pid that is listed but has no entry behaves like a process that
// exited: every per-process read returns oserr.ErrProcessGone.
type fakeSource struct {
	pids     []int
	stats    map[int]procfs.ProcStat
	statuses map[int]procfs.ProcStatus
	cmdlines map[int]string
	threads  map[int][]procfs.ProcStat

	sys    procfs.SystemStat
	mem    procfs.MemInfo
	load   procfs.LoadAvg
	uptime float64
	hz     int

	// Per-pid failures, checked before the maps above.
	statErr, statusErr, cmdlineErr, threadsErr map[int]error
	// System-wide failures.
	pidsErr, sysErr, memErr, loadErr, uptimeErr error
}

// fourCoreMachine is a Source with 4 cores, 100 ticks per second and no processes.
func fourCoreMachine() *fakeSource {
	return &fakeSource{
		stats:      map[int]procfs.ProcStat{},
		statuses:   map[int]procfs.ProcStatus{},
		cmdlines:   map[int]string{},
		threads:    map[int][]procfs.ProcStat{},
		statErr:    map[int]error{},
		statusErr:  map[int]error{},
		cmdlineErr: map[int]error{},
		threadsErr: map[int]error{},
		sys:        procfs.SystemStat{Cores: make([]procfs.CPUTimes, 4)},
		mem:        procfs.MemInfo{MemTotalKB: 1000, MemAvailableKB: 400},
		hz:         100,
	}
}

// proc describes one fake process.
type proc struct {
	pid, ppid int
	comm      string
	state     byte
	utime     uint64
	stime     uint64
	start     uint64
	uid       int
	rssKB     uint64
	cmdline   string
	threads   int
	priority  int
	nice      int
}

// add registers p and lists its pid (in the order of the calls).
func (f *fakeSource) add(p proc) *fakeSource {
	if p.state == 0 {
		p.state = 'S'
	}
	if p.start == 0 {
		p.start = 1000
	}
	if p.threads == 0 {
		p.threads = 1
	}
	f.pids = append(f.pids, p.pid)
	f.stats[p.pid] = procfs.ProcStat{
		PID: p.pid, Comm: p.comm, State: p.state, PPID: p.ppid,
		UTime: p.utime, STime: p.stime, StartTime: p.start,
		NumThreads: p.threads, Priority: p.priority, Nice: p.nice,
	}
	f.statuses[p.pid] = procfs.ProcStatus{UID: p.uid, VmRSSKB: p.rssKB}
	f.cmdlines[p.pid] = p.cmdline
	return f
}

// burn adds ticks of CPU time to a process.
func (f *fakeSource) burn(pid int, ticks uint64) {
	st := f.stats[pid]
	st.UTime += ticks
	f.stats[pid] = st
}

// tick adds ticks to the system-wide jiffies (the sum over all cores).
func (f *fakeSource) tick(ticks uint64) { f.sys.CPU.User += ticks }

// remove makes a process disappear from the listing and from /proc.
func (f *fakeSource) remove(pid int) {
	for i, p := range f.pids {
		if p == pid {
			f.pids = append(f.pids[:i:i], f.pids[i+1:]...)
			break
		}
	}
	delete(f.stats, pid)
	delete(f.statuses, pid)
	delete(f.cmdlines, pid)
	delete(f.threads, pid)
}

func (f *fakeSource) PIDs() ([]int, error) {
	if f.pidsErr != nil {
		return nil, f.pidsErr
	}
	return append([]int(nil), f.pids...), nil
}

func (f *fakeSource) Stat(pid int) (procfs.ProcStat, error) {
	if err := f.statErr[pid]; err != nil {
		return procfs.ProcStat{}, err
	}
	st, ok := f.stats[pid]
	if !ok {
		return procfs.ProcStat{}, oserr.ErrProcessGone
	}
	return st, nil
}

func (f *fakeSource) Status(pid int) (procfs.ProcStatus, error) {
	if err := f.statusErr[pid]; err != nil {
		return procfs.ProcStatus{}, err
	}
	st, ok := f.statuses[pid]
	if !ok {
		return procfs.ProcStatus{}, oserr.ErrProcessGone
	}
	return st, nil
}

func (f *fakeSource) Cmdline(pid int) (string, error) {
	if err := f.cmdlineErr[pid]; err != nil {
		return "", err
	}
	c, ok := f.cmdlines[pid]
	if !ok {
		return "", oserr.ErrProcessGone
	}
	return c, nil
}

func (f *fakeSource) Threads(pid int) ([]procfs.ProcStat, error) {
	if err := f.threadsErr[pid]; err != nil {
		return nil, err
	}
	th, ok := f.threads[pid]
	if !ok {
		return nil, oserr.ErrProcessGone
	}
	return th, nil
}

func (f *fakeSource) SystemStat() (procfs.SystemStat, error) { return f.sys, f.sysErr }
func (f *fakeSource) MemInfo() (procfs.MemInfo, error)       { return f.mem, f.memErr }
func (f *fakeSource) LoadAvg() (procfs.LoadAvg, error)       { return f.load, f.loadErr }
func (f *fakeSource) Uptime() (float64, error)               { return f.uptime, f.uptimeErr }
func (f *fakeSource) ClockTicks() int                        { return f.hz }

// newTestMonitor wires src to a deterministic user table and clock.
func newTestMonitor(t *testing.T, src Source, opts ...Option) *Monitor {
	t.Helper()
	passwd := writePasswd(t, passwdFixture)
	base := []Option{
		WithUsers(NewUsers(passwd)),
		WithClock(func() time.Time { return time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC) }),
	}
	return New(src, append(base, opts...)...)
}

// byPID finds a process of a snapshot, failing the test when it is absent.
func byPID(t *testing.T, s Snapshot, pid int) Process {
	t.Helper()
	for _, p := range s.Processes {
		if p.PID == pid {
			return p
		}
	}
	t.Fatalf("pid %d is not in the snapshot (have %v)", pid, pids(s.Processes))
	return Process{}
}

var errBoom = errors.New("boom")
