package tui

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
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

// barCells is how many of width cells of a bar are filled for a fraction in
// 0..1. A used resource always shows at least one cell.
func barCells(frac float64, width int) int {
	if width <= 0 || math.IsNaN(frac) || frac <= 0 {
		return 0
	}
	if frac >= 1 {
		return width
	}
	return max(1, int(math.Round(frac*float64(width))))
}

// clean makes text from /proc safe to draw. Any process may put anything in
// its name or arguments, so control characters (C0, DEL and C1: an ESC would
// start a terminal sequence, a newline would add a row, a tab would shift the
// columns) and invalid UTF-8 are replaced by a visible '?'. Call it before
// measuring or truncating, never after.
func clean(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if (r == utf8.RuneError && size == 1) || unicode.IsControl(r) {
			b.WriteByte('?')
		} else {
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

// cell makes s exactly w columns wide: longer text is cut with an ellipsis,
// shorter text is padded on the left or on the right. s must not hold styling.
func cell(s string, w int, right bool) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	pad := strings.Repeat(" ", max(0, w-ansi.StringWidth(s)))
	if right {
		return pad + s
	}
	return s + pad
}
