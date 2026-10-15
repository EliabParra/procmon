package monitor

import "time"

// CPUPercent is the share of the whole machine used by a process between two
// samples, the formula of the course spec: procDelta / sysDelta * 100.
// procDelta is the growth of the process's utime+stime and sysDelta the growth
// of the total jiffies of the aggregate cpu line of /proc/stat, which adds up
// all cores. So a process that keeps one core of four busy gives 25. A zero
// sysDelta gives 0.
func CPUPercent(procDelta, sysDelta uint64) float64 {
	if sysDelta == 0 {
		return 0
	}
	return float64(procDelta) / float64(sysDelta) * 100
}

// PerCore turns a share of the whole machine into the per-core figure of top's
// default Irix mode, where 100 is one core flat out and a multithreaded
// process can go beyond it. A count below one is treated as one.
func PerCore(share float64, cores int) float64 {
	return share * float64(max(cores, 1))
}

// MemPercent is the resident memory of a process as a share of total memory.
func MemPercent(rssKB, totalKB uint64) float64 {
	if totalKB == 0 {
		return 0
	}
	return float64(rssKB) / float64(totalKB) * 100
}

// ticksToDuration converts clock ticks to time, given the ticks per second.
// Whole seconds and the remainder are converted apart so a long-running
// process cannot overflow the multiplication.
func ticksToDuration(ticks uint64, hz int) time.Duration {
	if hz <= 0 {
		return 0
	}
	perSecond := uint64(hz)
	seconds := ticks / perSecond
	rest := ticks % perSecond
	return time.Duration(seconds)*time.Second + time.Duration(rest*uint64(time.Second)/perSecond)
}

// sub is a-b stopping at 0, so a counter that went backwards gives no delta
// instead of wrapping around.
func sub(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}
