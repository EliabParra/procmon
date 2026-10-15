package tui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
)

// sgrFg is the part of an escape sequence that switches the foreground to c,
// as the renderer writes it: 38;2;R;G;B. The renderer may join it with other
// attributes, so tests look for it inside the line, not for a whole sequence.
func sgrFg(c color.Color) string { return "38;2;" + rgb(c) }

func rgb(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("%d;%d;%d", r>>8, g>>8, b>>8)
}

// rawLine returns the styled line of the screen that holds marker.
func rawLine(t *testing.T, m *Model, marker string) string {
	t.Helper()
	for _, l := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(l, marker) {
			return l
		}
	}
	t.Fatalf("no line contains %q:\n%s", marker, screenText(m))
	return ""
}
