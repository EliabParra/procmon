package procfs

import (
	"fmt"
	"strconv"
	"strings"
)

// ProcStat is the subset of /proc/[pid]/stat the monitor needs.
// Times are in clock ticks (see FS.ClockTicks), not seconds.
type ProcStat struct {
	PID        int
	Comm       string // executable name, at most 15 chars, without parentheses
	State      byte   // R, S, D, Z, T, ...
	PPID       int
	UTime      uint64 // user-mode CPU time, ticks
	STime      uint64 // kernel-mode CPU time, ticks
	Priority   int    // kernel priority (20 + nice for normal tasks)
	Nice       int
	NumThreads int
	StartTime  uint64 // ticks after boot when the process started
}

// Positions inside the fields that follow the closing ')' of comm. The field
// numbers of proc(5) minus 3, because field 3 (state) is the first one here.
const (
	statState      = 3 - 3
	statPPID       = 4 - 3
	statUTime      = 14 - 3
	statSTime      = 15 - 3
	statPriority   = 18 - 3
	statNice       = 19 - 3
	statNumThreads = 20 - 3
	statStartTime  = 22 - 3
	statMinFields  = statStartTime + 1
)

// Stat parses /proc/[pid]/stat.
func (f FS) Stat(pid int) (ProcStat, error) {
	data, err := f.readPidFile(pid, "stat")
	if err != nil {
		return ProcStat{}, err
	}
	st, err := parseStat(data)
	if err != nil {
		return ProcStat{}, fmt.Errorf("procfs: pid %d: %w", pid, err)
	}
	return st, nil
}

// parseStat splits a stat line. The comm field sits inside parentheses and may
// itself contain spaces and ')', so the split happens at the LAST ')'.
func parseStat(data []byte) (ProcStat, error) {
	line := string(data)
	open := strings.IndexByte(line, '(')
	end := strings.LastIndexByte(line, ')')
	if open < 0 || end < open {
		return ProcStat{}, fmt.Errorf("malformed stat: comm is not in parentheses")
	}

	rest := strings.Fields(line[end+1:])
	if len(rest) < statMinFields {
		return ProcStat{}, fmt.Errorf("malformed stat: %d fields after comm, need %d", len(rest), statMinFields)
	}
	if len(rest[statState]) != 1 {
		return ProcStat{}, fmt.Errorf("malformed stat: state %q is not one character", rest[statState])
	}

	var p fieldParser
	st := ProcStat{
		PID:        p.int(strings.TrimSpace(line[:open])),
		Comm:       line[open+1 : end],
		State:      rest[statState][0],
		PPID:       p.int(rest[statPPID]),
		UTime:      p.uint(rest[statUTime]),
		STime:      p.uint(rest[statSTime]),
		Priority:   p.int(rest[statPriority]),
		Nice:       p.int(rest[statNice]),
		NumThreads: p.int(rest[statNumThreads]),
		StartTime:  p.uint(rest[statStartTime]),
	}
	if p.err != nil {
		return ProcStat{}, fmt.Errorf("malformed stat: %w", p.err)
	}
	return st, nil
}

// fieldParser converts numeric text and remembers the first failure, so a
// parser can read many fields and check for an error once at the end.
type fieldParser struct {
	err error
}

func (p *fieldParser) int(s string) int {
	v, err := strconv.Atoi(s)
	p.fail(err)
	return v
}

func (p *fieldParser) uint(s string) uint64 {
	v, err := strconv.ParseUint(s, 10, 64)
	p.fail(err)
	return v
}

func (p *fieldParser) fail(err error) {
	if p.err == nil {
		p.err = err
	}
}
