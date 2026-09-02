package prompt

import "strings"

// Document represents the current input state for completers.
//
// A completer is handed one of these on every Tab and answers from it. It is a
// snapshot: changing it changes nothing, and the prompt does not read it back.
type Document struct {
	// Text is the entire input, both sides of the cursor.
	Text string
	// CursorPosition is where the cursor is, counted in runes rather than
	// bytes. It indexes the same []rune the prompt edits, so on a line holding
	// any multi-byte character it is smaller than the byte offset -- a
	// completer building a Document of its own has to count runes, or it will
	// hand back a position past the end of its own text.
	CursorPosition int
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

// WordBeforeCursor returns the word before the cursor
func (d *Document) WordBeforeCursor() string {
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

// WordBeforeCursorEscaped is like WordBeforeCursor but treats whitespace
// that is backslash-escaped as part of the word, so a shell-style path such as
// "my\ data.csv" counts as a single word rather than two. A whitespace character
// is a word boundary only when an even number of backslashes precede it. The
// prompt uses it for completion when WithWordEscape is set.
func (d *Document) WordBeforeCursorEscaped() string {
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
// matches the separators WordBeforeCursor recognizes.
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

// CurrentLine returns the line the cursor is on: what lies between the line
// break before the cursor and the one after it, with neither of them.
//
// It is not the whole entry. An entry collected across several lines -- which is
// what a statement typed into a SQL shell is -- has one line per break in it,
// and a completer deciding from the current line wants the line being edited
// rather than all of them. Returning the entry gave such a completer a string
// with line breaks in it, which matched nothing or matched on a word from a line
// the user was not on.
//
// CursorPosition is counted in runes, and a position outside the text is
// answered the way TextBeforeCursor answers it.
func (d *Document) CurrentLine() string {
	before := d.TextBeforeCursor()
	if start := strings.LastIndex(before, "\n"); start >= 0 {
		before = before[start+1:]
	}
	after := d.TextAfterCursor()
	if end := strings.Index(after, "\n"); end >= 0 {
		after = after[:end]
	}
	return before + after
}
