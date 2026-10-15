package tui

import (
	"fmt"
	"time"
)

// humanKB writes a size given in KiB with the unit that keeps it short:
// 512K, 1.0M, 123M, 1.5G, 200G, 2.0T. It is at most five characters wide for
// any size below 1000 TiB.
func humanKB(kb uint64) string {
	if kb < 1024 {
		return fmt.Sprintf("%dK", kb)
	}
	size, unit := float64(kb)/1024, "M"
	for _, next := range []string{"G", "T"} {
		if size < 1024 {
			break
		}
		size, unit = size/1024, next
	}
	if (unit == "M" && size < 10) || (unit != "M" && size < 100) {
		return fmt.Sprintf("%.1f%s", size, unit)
	}
	return fmt.Sprintf("%.0f%s", size, unit)
}

// formatCPUTime is the TIME column: m:ss.cc below an hour, like top, and
// h:mm:ss from then on.
func formatCPUTime(d time.Duration) string {
	if d >= time.Hour {
		return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
	}
	return fmt.Sprintf("%d:%02d.%02d", int(d.Minutes()), int(d.Seconds())%60, d.Milliseconds()/10%100)
}

// formatUptime writes the time since boot as h:mm:ss, or d h:mm after a day.
func formatUptime(d time.Duration) string {
	if d >= 24*time.Hour {
		return fmt.Sprintf("%dd %d:%02d", int(d.Hours())/24, int(d.Hours())%24, int(d.Minutes())%60)
	}
	return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}
