package tui

import "testing"

func TestViewUsesTheAlternateScreen(t *testing.T) {
	m := loaded(100, 30, proc(1, "init", 0))
	if !m.View().AltScreen {
		t.Error("the view must ask for the alternate screen")
	}
}
