// Package monitor is the logic layer of the process monitor: it turns raw
// /proc readings (through a Source) into snapshots with %CPU computed from
// deltas, a summary, sorting, filtering, a parent/child tree and CPU history.
// It never touches the filesystem itself, except for /etc/passwd in Users.
package monitor

import "time"

// Process is one row of the process table: values read from /proc plus the
// fields this package computes (CPUPercent, MemPercent, CPUTime, Command).
type Process struct {
	PID        int
	PPID       int
	User       string  // user name, or "?" when /proc/[pid]/status was not readable
	State      byte    // R, S, D, Z, T, ...
	CPUPercent float64 // share of the whole machine since the last refresh, 0-100
	MemPercent float64
	RSSKB      uint64
	Threads    int
	Priority   int
	Nice       int
	CPUTime    time.Duration
	Command    string // full command line, or "[comm]" when it is empty
	Comm       string // executable name from /proc/[pid]/stat
	StartTime  uint64 // clock ticks after boot; with PID it identifies one process
}

// Summary is the header of the screen. The process counts come from the states
// of the processes read in this snapshot, not from /proc/loadavg.
type Summary struct {
	Total     int // processes in the snapshot
	Running   int // state R
	Sleeping  int // state S
	DiskSleep int // state D, uninterruptible sleep (usually waiting on I/O)
	Stopped   int // states T and t
	Zombie    int // state Z
	Idle      int // state I (idle kernel threads)

	Load1, Load5, Load15 float64

	MemTotalKB     uint64
	MemAvailableKB uint64
	MemUsedKB      uint64 // MemTotalKB - MemAvailableKB
	SwapTotalKB    uint64
	SwapUsedKB     uint64

	Uptime time.Duration
}

// Snapshot is the state of the machine at one Refresh.
type Snapshot struct {
	Processes []Process // ascending PID; use Sort, Filter and BuildTree to present them
	Summary   Summary
	Cores     int // logical CPUs, the factor of the per-core view of %CPU (PerCore)
	TakenAt   time.Time
}
