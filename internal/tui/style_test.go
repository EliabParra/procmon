package tui

import (
	"strings"
	"testing"
)

func TestPillsUseRoundCapsByDefault(t *testing.T) {
	m := loaded(120, 30, proc(1, "init", 0))
	content := m.View().Content
	if !strings.Contains(content, capLeft) || !strings.Contains(content, capRight) {
		t.Error("pills must have round caps")
	}
}

func TestPlainModeAvoidsNerdFontGlyphs(t *testing.T) {
	m := New(&fakeMonitor{}, &fakeControl{}, WithPlain())
	m.Update(windowSize(120, 30))
	m.Update(refreshedMsg{snap: snapshotOf(proc(1, "init", 0))})

	content := m.View().Content
	if strings.Contains(content, capLeft) || strings.Contains(content, capRight) {
		t.Error("plain mode must not draw private-use glyphs")
	}
	if !strings.Contains(screenText(m), "refresh 1s") {
		t.Error("the pills must still show their text")
	}
	assertFits(t, m, 120, 30)
}
