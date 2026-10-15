package tui

import (
	"fmt"

	"github.com/EliabParra/procmon/internal/monitor"
)

// cpuMode is how %CPU is shown. The monitor keeps one canonical value, the
// course formula (a share of the whole machine); the mode only decides how it
// is drawn, so sorting and the stored history never depend on it.
type cpuMode int

const (
	cpuMachine cpuMode = iota // the course formula: 100 is every core busy
	cpuPerCore                // like top's Irix mode: 100 is one core, threads can pass it
)

func (c cpuMode) name() string {
	if c == cpuPerCore {
		return "per-core"
	}
	return "machine"
}

// toggleCPUMode switches between the two modes and says what 100 means now.
func (m *Model) toggleCPUMode() {
	if m.cpu == cpuMachine {
		m.cpu = cpuPerCore
		m.notice = notice{text: "%CPU per core: 100 is one core flat out, a multithreaded process can pass it"}
		return
	}
	m.cpu = cpuMachine
	m.notice = notice{text: "%CPU of the whole machine: 100 is every core busy"}
}

// cpuValue is a %CPU of the monitor as the active mode shows it.
func (m *Model) cpuValue(share float64) float64 {
	if m.cpu == cpuPerCore {
		return monitor.PerCore(share, m.snap.Cores)
	}
	return share
}

// cpuSeries is a CPU history as the active mode shows it. It never edits its
// argument.
func (m *Model) cpuSeries(shares []float64) []float64 {
	if m.cpu == cpuMachine {
		return shares
	}
	out := make([]float64, len(shares))
	for i, v := range shares {
		out[i] = m.cpuValue(v)
	}
	return out
}

// cpuText writes a %CPU figure.
func cpuText(v float64) string {
	return fmt.Sprintf("%.1f", v)
}
