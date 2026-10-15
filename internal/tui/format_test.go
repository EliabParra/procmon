package tui

import (
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
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

func TestClean(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text is untouched", "/usr/bin/foo --bar=1 日本語", "/usr/bin/foo --bar=1 日本語"},
		{"tab, newline and carriage return", "a\tb\nc\rd", "a?b?c?d"},
		{"escape sequences lose their ESC", "\x1b[31mred\x1b[2J", "?[31mred?[2J"},
		{"DEL", "a\x7fb", "a?b"},
		{"C1 controls", "a\u0085b\u009bc", "a?b?c"},
		{"NUL", "a\x00b", "a?b"},
		{"invalid UTF-8", "a\xffb\xc3", "a?b?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clean(tt.in)
			if got != tt.want {
				t.Errorf("clean(%q) = %q, want %q", tt.in, got, tt.want)
			}
			for _, r := range got {
				if unicode.IsControl(r) || r == utf8.RuneError {
					t.Errorf("clean(%q) left %U in %q", tt.in, r, got)
				}
			}
		})
	}
}

func TestCell(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		w     int
		right bool
		want  string
	}{
		{"pads on the right", "ab", 5, false, "ab   "},
		{"pads on the left when right-aligned", "ab", 5, true, "   ab"},
		{"fits exactly", "abcde", 5, false, "abcde"},
		{"truncates with an ellipsis", "abcdefgh", 5, false, "abcd…"},
		{"truncates right-aligned text too", "abcdefgh", 5, true, "abcd…"},
		{"wide runes count two columns", "日本語", 5, false, "日本…"},
		{"zero width", "abc", 0, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cell(tt.s, tt.w, tt.right)
			if got != tt.want {
				t.Errorf("cell(%q, %d, %v) = %q, want %q", tt.s, tt.w, tt.right, got, tt.want)
			}
			if w := ansi.StringWidth(got); w != tt.w {
				t.Errorf("width = %d, want %d", w, tt.w)
			}
		})
	}
}
