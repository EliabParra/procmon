package monitor

import (
	"math"
	"testing"
	"time"
)

func TestCPUPercent(t *testing.T) {
	tests := []struct {
		name                string
		procDelta, sysDelta uint64
		want                float64
	}{
		{"idle process", 0, 400, 0},
		{"one core of four is a quarter of the machine", 100, 400, 25},
		{"half a core of four", 50, 400, 12.5},
		{"the whole machine", 400, 400, 100},
		{"single core machine, whole machine", 100, 100, 100},
		{"zero system delta cannot divide", 10, 0, 0},
		{"fraction", 1, 3, 100.0 / 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CPUPercent(tt.procDelta, tt.sysDelta)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("CPUPercent(%d, %d) = %v, want %v", tt.procDelta, tt.sysDelta, got, tt.want)
			}
		})
	}
}

func TestPerCore(t *testing.T) {
	tests := []struct {
		name  string
		share float64
		cores int
		want  float64
	}{
		{"idle", 0, 4, 0},
		{"a quarter of four cores is one core flat out", 25, 4, 100},
		{"the whole machine is every core", 100, 4, 400},
		{"single core machine changes nothing", 40, 1, 40},
		{"zero cores is treated as one", 50, 0, 50},
		{"negative cores is treated as one", 50, -3, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PerCore(tt.share, tt.cores)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("PerCore(%v, %d) = %v, want %v", tt.share, tt.cores, got, tt.want)
			}
		})
	}
}

func TestMemPercent(t *testing.T) {
	tests := []struct {
		name           string
		rssKB, totalKB uint64
		want           float64
	}{
		{"quarter of memory", 250, 1000, 25},
		{"nothing resident (kernel thread)", 0, 1000, 0},
		{"unknown total cannot divide", 100, 0, 0},
		{"all memory", 1000, 1000, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MemPercent(tt.rssKB, tt.totalKB); math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("MemPercent(%d, %d) = %v, want %v", tt.rssKB, tt.totalKB, got, tt.want)
			}
		})
	}
}

func TestTicksToDuration(t *testing.T) {
	tests := []struct {
		name  string
		ticks uint64
		hz    int
		want  time.Duration
	}{
		{"hundred ticks at 100 Hz is one second", 100, 100, time.Second},
		{"fractional second", 150, 100, 1500 * time.Millisecond},
		{"other tick rate", 250, 250, time.Second},
		{"rate that does not divide a second", 1, 1024, time.Second / 1024},
		{"zero ticks", 0, 100, 0},
		{"unknown rate cannot divide", 100, 0, 0},
		{"huge counter does not overflow", 5_000_000_000_000, 1000, 5_000_000_000 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ticksToDuration(tt.ticks, tt.hz); got != tt.want {
				t.Fatalf("ticksToDuration(%d, %d) = %v, want %v", tt.ticks, tt.hz, got, tt.want)
			}
		})
	}
}
