package prompt

// Document represents the current input state for completers
type Document struct {
	Text           string // The entire input text
	CursorPosition int    // Current cursor position in the text
}

// TextBeforeCursor returns the text before the cursor.
//
// CursorPosition is counted in runes, because it is an index into the prompt's
// []rune buffer. Slicing the text by it as if it were a byte offset cut a
// multi-byte identifier in half and returned a prefix shorter than what the
// user had typed, so a completer saw the wrong word.
func (d *Document) TextBeforeCursor() string {
	runes := []rune(d.Text)
	if d.CursorPosition < 0 || d.CursorPosition > len(runes) {
		return d.Text
	}
	return string(runes[:d.CursorPosition])
}

// TextAfterCursor returns the text after the cursor. CursorPosition is counted
// in runes; see TextBeforeCursor.
func (d *Document) TextAfterCursor() string {
	runes := []rune(d.Text)
	if d.CursorPosition < 0 || d.CursorPosition >= len(runes) {
		return ""
	}
	return string(runes[d.CursorPosition:])
}

// GetWordBeforeCursor returns the word before the cursor
func (d *Document) GetWordBeforeCursor() string {
	runes := []rune(d.TextBeforeCursor())
	if len(runes) == 0 {
		return ""
	}

	// If cursor is right after a whitespace character, return empty string
	if isWordSeparator(runes[len(runes)-1]) {
		return ""
	}

	// Find the start of the current word by scanning backwards
	start := len(runes) - 1
	for start >= 0 && !isWordSeparator(runes[start]) {
		start--
	}
	start++ // Move to the first character of the word

	return string(runes[start:])
}

// GetWordBeforeCursorEscaped is like GetWordBeforeCursor but treats whitespace
// that is backslash-escaped as part of the word, so a shell-style path such as
// "my\ data.csv" counts as a single word rather than two. A whitespace character
// is a word boundary only when an even number of backslashes precede it. The
// prompt uses it for completion when WithWordEscape is set.
func (d *Document) GetWordBeforeCursorEscaped() string {
	text := d.TextBeforeCursor()
	if len(text) == 0 {
		return ""
	}

	runes := []rune(text)
	last := len(runes) - 1
	if isWordSeparator(runes[last]) && !isEscaped(runes, last) {
		return ""
	}

	start := 0
	for i := last; i >= 0; i-- {
		if isWordSeparator(runes[i]) && !isEscaped(runes, i) {
			start = i + 1
			break
		}
	}
	return string(runes[start:])
}

// isWordSeparator reports whether r ends a word for completion purposes. It
// matches the separators GetWordBeforeCursor recognizes.
func isWordSeparator(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n'
}

// isEscaped reports whether the rune at index i is escaped, i.e. preceded by an
// odd number of backslashes.
func isEscaped(runes []rune, i int) bool {
	backslashes := 0
	for j := i - 1; j >= 0 && runes[j] == '\\'; j-- {
		backslashes++
	}
	return backslashes%2 == 1
}

// CurrentLine returns the current line
func (d *Document) CurrentLine() string {
	return d.Text
}
