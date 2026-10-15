package monitor

import (
	"fmt"
	"time"
)

// Thread is one thread (task) of a process, read from /proc/[pid]/task/[tid]/stat.
type Thread struct {
	TID     int
	Comm    string
	State   byte
	CPUTime time.Duration // utime+stime of this thread
}

// Threads lists the threads of pid in the order the Source gives them (by TID
// for procfs). The error wraps oserr.ErrProcessGone when the process exited
// and oserr.ErrPermission when it may not be inspected.
func (m *Monitor) Threads(pid int) ([]Thread, error) {
	stats, err := m.src.Threads(pid)
	if err != nil {
		return nil, fmt.Errorf("monitor: threads of %d: %w", pid, err)
	}
	hz := m.src.ClockTicks()
	threads := make([]Thread, len(stats))
	for i, st := range stats {
		threads[i] = Thread{
			TID:     st.PID, // procfs puts the thread id in the PID field
			Comm:    st.Comm,
			State:   st.State,
			CPUTime: ticksToDuration(st.UTime+st.STime, hz),
		}
	}
	return threads, nil
}
