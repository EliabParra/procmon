package tui

import (
	"testing"
	"time"
)

func TestHumanKB(t *testing.T) {
	tests := []struct {
		kb   uint64
		want string
	}{
		{0, "0K"},
		{512, "512K"},
		{1023, "1023K"},
		{1024, "1.0M"},
		{10 * 1024, "10M"},
		{1023 * 1024, "1023M"},
		{1024 * 1024, "1.0G"},
		{1536 * 1024, "1.5G"},
		{16 * 1024 * 1024, "16.0G"},
		{200 * 1024 * 1024, "200G"},
		{2 * 1024 * 1024 * 1024, "2.0T"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := humanKB(tt.kb); got != tt.want {
				t.Errorf("humanKB(%d) = %q, want %q", tt.kb, got, tt.want)
			}
		})
	}
}

func TestFormatCPUTime(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00.00"},
		{83*time.Second + 450*time.Millisecond, "1:23.45"},
		{59*time.Minute + 59*time.Second + 990*time.Millisecond, "59:59.99"},
		{time.Hour, "1:00:00"},
		{3725 * time.Second, "1:02:05"},
		{100 * time.Hour, "100:00:00"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatCPUTime(tt.d); got != tt.want {
				t.Errorf("formatCPUTime(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00:00"},
		{4*time.Hour + 5*time.Minute + 12*time.Second, "4:05:12"},
		{3*24*time.Hour + 4*time.Hour + 5*time.Minute, "3d 4:05"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatUptime(tt.d); got != tt.want {
				t.Errorf("formatUptime(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}
