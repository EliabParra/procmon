package tui

import (
	"strings"
	"testing"
)

func TestHelpLineDropsTheLastHintsWhenNarrow(t *testing.T) {
	m := loaded(30, 20, proc(1, "init", 0))
	out := screen(m)
	help := out[len(out)-1]
	if !strings.Contains(help, "q quit") {
		t.Errorf("quit must always be listed: %q", help)
	}
	if strings.Contains(help, "set interval") {
		t.Errorf("the last hints must be dropped to fit: %q", help)
	}
	assertFits(t, m, 30, 20)
}
