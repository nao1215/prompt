package prompt

import (
	"slices"
	"strings"
	"unicode"
)

// This file holds the line editor: what a keystroke does to the buffer and to
// the cursor, and the questions the read loop asks about the line it is on --
// where the current line begins and ends, where the next word boundary is, what
// a vertical move lands on.
//
// It is separate from the read loop because the two are about different things.
// The loop owns the terminal, the history and the completion menu; this owns a
// []rune and an index into it, and every function here can be written by looking
// at those two alone. The buffer is runes rather than bytes throughout: a
// position measured in bytes has caused more than one bug here, most of them
// reachable only in a line not written in ASCII.

func (p *Prompt) insertRune(r rune) {
	p.buffer = append(p.buffer[:p.cursor], append([]rune{r}, p.buffer[p.cursor:]...)...)
	p.cursor++
}

// insertPastedRune inserts one rune of bracketed-paste content and returns it,
// so the caller can pass it back as prev on the next call.
//
// A line break becomes exactly one "\n" however the terminal spells it: pasting
// Windows text delivers CR LF, and inserting a newline for each of them turned
// every line break into a blank line. Control bytes other than TAB are dropped
// rather than inserted, because they are neither text the user pasted nor
// commands they pressed.
func (p *Prompt) insertPastedRune(r, prev rune) rune {
	switch {
	case r == '\r':
		p.insertRune('\n')
	case r == '\n':
		if prev != '\r' {
			p.insertRune('\n')
		}
	case r == '\t' || (r >= 32 && r != 0x7f):
		p.insertRune(r)
	}
	return r
}

// insertNewline breaks the line at the cursor and opens the next one with
// whatever the auto-indent hook asks for. Every way a line can break goes
// through here, so a continuation looks the same however it was asked for.
func (p *Prompt) insertNewline() {
	// The indenter is given the line it is continuing, so it is asked before the
	// newline is inserted rather than after.
	var indent string
	if p.config.AutoIndent != nil {
		indent = p.config.AutoIndent(string(p.buffer[:p.cursor]))
	}
	p.insertRune('\n')
	if indent != "" {
		p.insertText(indent)
	}
}

func (p *Prompt) insertText(text string) {
	runes := []rune(text)
	p.buffer = append(p.buffer[:p.cursor], append(runes, p.buffer[p.cursor:]...)...)
	p.cursor += len(runes)
}

func (p *Prompt) setBuffer(text string) {
	p.buffer = []rune(text)
	p.cursor = len(p.buffer)
}

// findWordBoundary returns where Ctrl+Right (direction above zero) or Ctrl+Left
// (below) puts the cursor: the end of the word ahead, or the start of the word
// behind. Ctrl+W deletes back to the same place Ctrl+Left would move to, which
// is why one function answers for all three.
//
// A run of separators is crossed before the word is, in both directions, so a
// cursor sitting between words moves over a word rather than stopping on the gap
// and having to be pressed again. What counts as a separator is isWordChar.
func (p *Prompt) findWordBoundary(direction int) int {
	if direction > 0 {
		// Find next word start (Ctrl+Right)
		pos := p.cursor
		for pos < len(p.buffer) && !isWordChar(p.buffer[pos]) {
			pos++ // Skip non-word characters
		}
		for pos < len(p.buffer) && isWordChar(p.buffer[pos]) {
			pos++ // Skip word characters
		}
		return pos
	}
	// Find previous word start (Ctrl+Left)
	pos := p.cursor
	if pos > 0 {
		pos-- // Move back one position
	}
	for pos > 0 && !isWordChar(p.buffer[pos]) {
		pos-- // Skip non-word characters
	}
	for pos > 0 && isWordChar(p.buffer[pos-1]) {
		pos-- // Skip word characters
	}
	return pos
}

// isWordChar reports whether r is part of a word for the purpose of moving and
// deleting by words. A letter, a digit, a combining mark and an underscore are;
// everything else, punctuation and whitespace alike, separates. The underscore
// is there because what a user of this library types is mostly identifiers.
//
// A letter is a letter in any script. Testing for a-z alone made every other
// alphabet a separator, so word navigation walked over a word written in
// Japanese as if it were whitespace and carried on into the word before it, and
// a letter with a diacritic split its own word in two.
//
// A mark is part of a word because it is part of the letter in front of it. In
// decomposed text -- which is what macOS hands back for a filename, so a pasted
// path carries it -- an accent is a rune of its own, and a mark that separated
// words stopped the cursor between a letter and its accent: Backspace there took
// the accent off a letter the user meant to keep, and the renderer, which
// measures a mark as no cells at all, drew the cursor on the letter's own cell.
// A mark only ever follows what it belongs to, so this cannot join two words
// that were separate.
//
// Completion decides what a word is by its own rule, which this is not. See
// Document.GetWordBeforeCursor.
func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_'
}

// isShiftEnter detects if we should add a newline instead of submitting
func (p *Prompt) isShiftEnter() bool {
	currentLine := p.getCurrentLineText()

	// Check for backslash continuation - if present, add newline
	if strings.HasSuffix(strings.TrimRight(currentLine, " \t"), "\\") {
		// Remove the backslash and add newline for continuation
		p.removeTrailingBackslash()
		return true // Add newline for continuation
	}

	// If no backslash, Enter submits (both single-line and multiline modes)
	return false
}

// isMultiLine checks if the current buffer contains newline characters
func (p *Prompt) isMultiLine() bool {
	return slices.Contains(p.buffer, '\n')
}

// findLineStart finds the start of the current line
func (p *Prompt) findLineStart() int {
	return p.findLineBoundary(p.cursor, -1)
}

// findLineEnd finds the end of the current line
func (p *Prompt) findLineEnd() int {
	return p.findLineBoundary(p.cursor, 1)
}

// findLineBoundary finds the line boundary in the given direction
// direction < 0: finds line start, direction > 0: finds line end
func (p *Prompt) findLineBoundary(start int, direction int) int {
	pos := start
	if direction < 0 {
		// Find line start
		for pos > 0 && p.buffer[pos-1] != '\n' {
			pos--
		}
	} else {
		// Find line end
		for pos < len(p.buffer) && p.buffer[pos] != '\n' {
			pos++
		}
	}
	return pos
}

// findCursorUp moves cursor to the same column on the previous line
func (p *Prompt) findCursorUp() int {
	return p.findCursorVertical(-1)
}

// findCursorDown moves cursor to the same column on the next line
func (p *Prompt) findCursorDown() int {
	return p.findCursorVertical(1)
}

// findCursorVertical moves cursor vertically maintaining column position
// direction < 0: move up, direction > 0: move down
func (p *Prompt) findCursorVertical(direction int) int {
	lineStart := p.findLineStart()
	lineEnd := p.findLineEnd()
	column := p.cursor - lineStart

	if direction < 0 {
		// Move up
		if lineStart == 0 {
			return p.cursor // Already at first line
		}

		// Find start of previous line
		prevLineEnd := lineStart - 1 // Skip the newline
		prevLineStart := 0
		for i := prevLineEnd - 1; i >= 0; i-- {
			if p.buffer[i] == '\n' {
				prevLineStart = i + 1
				break
			}
		}

		// Calculate new cursor position
		prevLineLength := prevLineEnd - prevLineStart
		if column < prevLineLength {
			return prevLineStart + column
		}
		return prevLineEnd
	}

	// Move down
	if lineEnd >= len(p.buffer) {
		return p.cursor // Already at last line
	}

	// Find end of next line
	nextLineStart := lineEnd + 1 // Skip the newline
	nextLineEnd := len(p.buffer)
	for i := nextLineStart; i < len(p.buffer); i++ {
		if p.buffer[i] == '\n' {
			nextLineEnd = i
			break
		}
	}

	// Calculate new cursor position
	nextLineLength := nextLineEnd - nextLineStart
	if column < nextLineLength {
		return nextLineStart + column
	}
	return nextLineEnd
}

// getCurrentLineText returns the text of the current line where the cursor is positioned
func (p *Prompt) getCurrentLineText() string {
	lineStart := p.findLineStart()
	lineEnd := p.findLineEnd()
	return string(p.buffer[lineStart:lineEnd])
}

// removeTrailingBackslash removes the trailing backslash from the current line
func (p *Prompt) removeTrailingBackslash() {
	lineStart := p.findLineStart()
	line := p.buffer[lineStart:p.findLineEnd()]

	// The backslash is found by walking the runes rather than by measuring a
	// string. The buffer is a []rune and lineStart indexes it, so adding the byte
	// length of the line's text put the position three cells further along for
	// every multi-byte rune: the slice went past the end of the buffer and the
	// prompt panicked, or, when the buffer's capacity happened to reach that far,
	// deleted a rune that was not the backslash.
	end := len(line)
	for end > 0 && (line[end-1] == ' ' || line[end-1] == '\t') {
		end--
	}
	if end == 0 || line[end-1] != '\\' {
		return
	}

	backslashPos := lineStart + end - 1
	p.buffer = append(p.buffer[:backslashPos], p.buffer[backslashPos+1:]...)
	// The cursor takes the backslash's place, which is where the newline goes.
	p.cursor = backslashPos
}
