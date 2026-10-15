package monitor

import (
	"fmt"

	"github.com/EliabParra/procmon/internal/oserr"
)

// Verify checks that pid is still the process that was listed with startTime:
// it reads the process again, right now, and compares the start time. A
// snapshot can be a second old, and Linux reuses PIDs, so acting on a PID
// without this check could signal a stranger.
//
// The error wraps oserr.ErrProcessGone when the process exited or its PID now
// belongs to another process, and oserr.ErrPermission when it may not be
// inspected. A zombie is still the same process and verifies fine.
func (m *Monitor) Verify(pid int, startTime uint64) error {
	st, err := m.src.Stat(pid)
	if err != nil {
		return fmt.Errorf("monitor: verify %d: %w", pid, err)
	}
	if st.StartTime != startTime {
		return fmt.Errorf("monitor: pid %d now belongs to another process: %w", pid, oserr.ErrProcessGone)
	}
	return nil
}
