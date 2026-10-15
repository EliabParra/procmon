package tui

// lineInput is the text being typed in the bottom line.
type lineInput struct{ text []rune }

func (l *lineInput) String() string { return string(l.text) }

// Set replaces the text.
func (l *lineInput) Set(s string) { l.text = []rune(clean(s)) }

// Insert adds typed or pasted text at the end. Control characters become '?',
// so a paste cannot smuggle a newline or an escape sequence in.
func (l *lineInput) Insert(s string) { l.text = append(l.text, []rune(clean(s))...) }

// Backspace removes the last rune.
func (l *lineInput) Backspace() {
	if len(l.text) > 0 {
		l.text = l.text[:len(l.text)-1]
	}
}

// Reset empties the line.
func (l *lineInput) Reset() { l.text = nil }
