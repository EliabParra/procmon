package monitor

import "github.com/EliabParra/procmon/internal/procfs"

// Source is everything the monitor reads from the operating system. It is the
// port of the hexagon: procfs.FS is the adapter that reads /proc, and tests
// plug in a fake whose values change between two refreshes.
type Source interface {
	PIDs() ([]int, error)
	Stat(pid int) (procfs.ProcStat, error)
	Status(pid int) (procfs.ProcStatus, error)
	Cmdline(pid int) (string, error)
	Threads(pid int) ([]procfs.ProcStat, error)
	SystemStat() (procfs.SystemStat, error)
	MemInfo() (procfs.MemInfo, error)
	LoadAvg() (procfs.LoadAvg, error)
	Uptime() (float64, error)
	ClockTicks() int
}

// The real adapter must keep satisfying the port; this fails the build otherwise.
var _ Source = procfs.FS{}
