package procfs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/EliabParra/procmon/internal/oserr"
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

// ProcStatus is the subset of /proc/[pid]/status the monitor needs.
type ProcStatus struct {
	UID     int    // real UID (first field of the Uid line)
	VmRSSKB uint64 // resident memory in kB; 0 for kernel threads
}

// firstFieldAfterComm is the proc(5) field number of the first field that
// follows "(comm)": field 3, the state.
const firstFieldAfterComm = 3

// Positions inside the fields that follow the closing ')' of comm: the field
// number of proc(5) minus firstFieldAfterComm, so state is position 0.
const (
	statState      = 3 - firstFieldAfterComm
	statPPID       = 4 - firstFieldAfterComm
	statUTime      = 14 - firstFieldAfterComm
	statSTime      = 15 - firstFieldAfterComm
	statPriority   = 18 - firstFieldAfterComm
	statNice       = 19 - firstFieldAfterComm
	statNumThreads = 20 - firstFieldAfterComm
	statStartTime  = 22 - firstFieldAfterComm
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

// Status parses /proc/[pid]/status.
func (f FS) Status(pid int) (ProcStatus, error) {
	data, err := f.readPidFile(pid, "status")
	if err != nil {
		return ProcStatus{}, err
	}
	st, err := parseStatus(data)
	if err != nil {
		return ProcStatus{}, fmt.Errorf("procfs: pid %d: %w", pid, err)
	}
	return st, nil
}

// parseStatus reads the "Key:\tvalue" lines. Uid is "real effective saved fs";
// VmRSS is absent for kernel threads, which is not an error (it stays 0).
func parseStatus(data []byte) (ProcStatus, error) {
	var st ProcStatus
	var p fieldParser
	haveUID := false

	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			continue
		}
		switch key {
		case "Uid":
			st.UID = p.int(fields[0])
			haveUID = true
		case "VmRSS":
			st.VmRSSKB = p.uint(fields[0])
		}
	}
	if p.err != nil {
		return ProcStatus{}, fmt.Errorf("malformed status: %w", p.err)
	}
	if !haveUID {
		return ProcStatus{}, fmt.Errorf("malformed status: no Uid line")
	}
	return st, nil
}

// Cmdline returns /proc/[pid]/cmdline with the NUL separators turned into
// spaces. Kernel threads and zombies have an empty command line; the caller
// falls back to "[comm]" in that case.
func (f FS) Cmdline(pid int) (string, error) {
	data, err := f.readPidFile(pid, "cmdline")
	if err != nil {
		return "", err
	}
	data = bytes.TrimRight(data, "\x00")
	return string(bytes.ReplaceAll(data, []byte{0}, []byte{' '})), nil
}

// Threads parses /proc/[pid]/task/[tid]/stat for every thread of pid, sorted
// by TID. Each result's PID field holds the thread id. A thread that exits
// between listing the directory and reading its stat is skipped.
func (f FS) Threads(pid int) ([]ProcStat, error) {
	taskDir := filepath.Join(f.Root, strconv.Itoa(pid), "task")
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return nil, oserr.Classify(err)
	}

	var tids []int
	for _, e := range entries {
		if tid, ok := parsePID(e.Name()); ok {
			tids = append(tids, tid)
		}
	}
	sort.Ints(tids)

	threads := make([]ProcStat, 0, len(tids))
	for _, tid := range tids {
		data, err := f.readPidFile(pid, "task", strconv.Itoa(tid), "stat")
		if err != nil {
			if isGone(err) {
				continue // the thread exited while we were looking
			}
			return nil, err
		}
		st, err := parseStat(data)
		if err != nil {
			return nil, fmt.Errorf("procfs: pid %d thread %d: %w", pid, tid, err)
		}
		threads = append(threads, st)
	}
	return threads, nil
}
