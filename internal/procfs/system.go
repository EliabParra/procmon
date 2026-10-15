package procfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CPUTimes holds cumulative CPU time in clock ticks, as in a /proc/stat "cpu" line.
type CPUTimes struct {
	User, Nice, System, Idle, IOWait, IRQ, SoftIRQ, Steal uint64
}

// Total is the sum of the eight columns above. The guest columns of
// /proc/stat are ignored on purpose: the kernel already counts guest time
// inside User, so adding it would count it twice.
func (c CPUTimes) Total() uint64 {
	return c.User + c.Nice + c.System + c.Idle + c.IOWait + c.IRQ + c.SoftIRQ + c.Steal
}

// SystemStat is the CPU part of /proc/stat.
type SystemStat struct {
	CPU   CPUTimes   // the aggregate "cpu" line
	Cores []CPUTimes // one entry per "cpuN" line, in file order
}

// MemInfo holds the /proc/meminfo values the monitor uses, all in kB.
type MemInfo struct {
	MemTotalKB     uint64
	MemFreeKB      uint64
	MemAvailableKB uint64 // 0 on kernels older than 3.14, which do not report it
	SwapTotalKB    uint64
	SwapFreeKB     uint64
}

// LoadAvg is /proc/loadavg.
type LoadAvg struct {
	One, Five, Fifteen float64
	Running            int // currently runnable kernel scheduling entities
	Total              int // existing kernel scheduling entities
}

// readSystemFile reads Root/<name>. These files always exist on a live
// system, so errors are NOT classified as "process gone".
func (f FS) readSystemFile(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(f.Root, name))
}

// SystemStat parses /proc/stat: the aggregate "cpu" line and one "cpuN" line
// per core. Every other line (intr, ctxt, btime, ...) is ignored.
func (f FS) SystemStat() (SystemStat, error) {
	data, err := f.readSystemFile("stat")
	if err != nil {
		return SystemStat{}, err
	}
	ss, err := parseSystemStat(data)
	if err != nil {
		return SystemStat{}, fmt.Errorf("procfs: /proc/stat: %w", err)
	}
	return ss, nil
}

func parseSystemStat(data []byte) (SystemStat, error) {
	var ss SystemStat
	haveAggregate := false

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		suffix := strings.TrimPrefix(fields[0], "cpu")
		if suffix != "" && !isDigits(suffix) {
			continue
		}
		times, err := parseCPUTimes(fields[1:])
		if err != nil {
			return SystemStat{}, fmt.Errorf("%s line: %w", fields[0], err)
		}
		if suffix == "" {
			ss.CPU = times
			haveAggregate = true
		} else {
			ss.Cores = append(ss.Cores, times)
		}
	}
	if !haveAggregate {
		return SystemStat{}, fmt.Errorf("no aggregate cpu line")
	}
	return ss, nil
}

// parseCPUTimes reads user nice system idle iowait irq softirq steal. Kernels
// before 2.6.11 report only the first four columns; missing ones stay 0.
// Columns after steal (guest, guest_nice) are ignored.
func parseCPUTimes(fields []string) (CPUTimes, error) {
	if len(fields) < 4 {
		return CPUTimes{}, fmt.Errorf("%d columns, need at least 4", len(fields))
	}
	var c CPUTimes
	columns := []*uint64{&c.User, &c.Nice, &c.System, &c.Idle, &c.IOWait, &c.IRQ, &c.SoftIRQ, &c.Steal}
	for i, dst := range columns {
		if i >= len(fields) {
			break
		}
		v, err := strconv.ParseUint(fields[i], 10, 64)
		if err != nil {
			return CPUTimes{}, err
		}
		*dst = v
	}
	return c, nil
}

// MemInfo parses /proc/meminfo ("Key:   value kB" lines).
func (f FS) MemInfo() (MemInfo, error) {
	data, err := f.readSystemFile("meminfo")
	if err != nil {
		return MemInfo{}, err
	}
	mi, err := parseMemInfo(data)
	if err != nil {
		return MemInfo{}, fmt.Errorf("procfs: /proc/meminfo: %w", err)
	}
	return mi, nil
}

func parseMemInfo(data []byte) (MemInfo, error) {
	var mi MemInfo
	var p fieldParser
	haveTotal := false

	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, ":")
		fields := strings.Fields(value)
		if !ok || len(fields) == 0 {
			continue
		}
		switch key {
		case "MemTotal":
			mi.MemTotalKB = p.uint(fields[0])
			haveTotal = true
		case "MemFree":
			mi.MemFreeKB = p.uint(fields[0])
		case "MemAvailable":
			mi.MemAvailableKB = p.uint(fields[0])
		case "SwapTotal":
			mi.SwapTotalKB = p.uint(fields[0])
		case "SwapFree":
			mi.SwapFreeKB = p.uint(fields[0])
		}
	}
	if p.err != nil {
		return MemInfo{}, p.err
	}
	if !haveTotal {
		return MemInfo{}, fmt.Errorf("no MemTotal line")
	}
	return mi, nil
}

// LoadAvg parses /proc/loadavg: "1min 5min 15min running/total lastpid".
func (f FS) LoadAvg() (LoadAvg, error) {
	data, err := f.readSystemFile("loadavg")
	if err != nil {
		return LoadAvg{}, err
	}
	la, err := parseLoadAvg(data)
	if err != nil {
		return LoadAvg{}, fmt.Errorf("procfs: /proc/loadavg: %w", err)
	}
	return la, nil
}

func parseLoadAvg(data []byte) (LoadAvg, error) {
	fields := strings.Fields(string(data))
	if len(fields) < 4 {
		return LoadAvg{}, fmt.Errorf("%d fields, need at least 4", len(fields))
	}
	running, total, ok := strings.Cut(fields[3], "/")
	if !ok {
		return LoadAvg{}, fmt.Errorf("entities %q is not running/total", fields[3])
	}

	var la LoadAvg
	var err error
	for i, dst := range []*float64{&la.One, &la.Five, &la.Fifteen} {
		if *dst, err = strconv.ParseFloat(fields[i], 64); err != nil {
			return LoadAvg{}, err
		}
	}
	var p fieldParser
	la.Running = p.int(running)
	la.Total = p.int(total)
	if p.err != nil {
		return LoadAvg{}, p.err
	}
	return la, nil
}
