package prompt

import "strings"

// This file holds what a completion is once the completer has answered: the word
// a suggestion stands for, how one is applied to the buffer, and how far the
// menu scrolls. The renderer decides which candidates the menu can show, because
// that depends on the room the terminal has left.

// scrollToSelection returns the scroll offset that puts the selected suggestion
// inside the window the renderer will draw.
//
// The window is however many candidates fit under the input block, which depends
// on the terminal's height, on how far the input wraps, and on how far the
// candidates themselves wrap -- so the count is measured rather than assumed. It
// is measured again after each move of the offset, because a later window holds
// different candidates, which may take more rows than the ones they replaced.
func (p *Prompt) scrollToSelection(suggestions []Suggestion, selected, offset int) int {
	// Bounded by the list: every pass moves the offset toward the selection.
	for range suggestions {
		if selected < offset {
			offset = selected
			continue
		}
		window := p.renderer.suggestionWindow(p.config.Prefix, string(p.buffer), suggestions, offset)
		if window > 0 && selected >= offset+window {
			offset = selected - window + 1
			continue
		}
		return offset
	}
	return offset
}

// completionWord returns the word before the cursor used for completion matching
// and acceptance. It honors backslash-escaped whitespace when WithWordEscape is
// set so space-containing paths complete as one word.
func (p *Prompt) completionWord(doc Document) string {
	if p.config.WordEscape {
		return doc.GetWordBeforeCursorEscaped()
	}
	return doc.GetWordBeforeCursor()
}

// hasReplaceRange reports whether any suggestion names the span it replaces,
// which is how a completer says it owns matching for this set.
func hasReplaceRange(suggestions []Suggestion) bool {
	for _, s := range suggestions {
		if s.Replace != nil {
			return true
		}
	}
	return false
}

func (p *Prompt) acceptSuggestion(suggestion Suggestion) {
	// A suggestion that names the span it stands for is applied literally: the
	// completer knows what it matched, and the word-boundary guesswork below
	// cannot express a qualified name or a case-insensitive match.
	if suggestion.Replace != nil {
		p.replaceRange(*suggestion.Replace, suggestion.Text)
		return
	}

	// Get current document state for context
	doc := Document{
		Text:           string(p.buffer),
		CursorPosition: p.cursor,
	}

	// Determine how to apply the suggestion based on context
	beforeCursor := doc.TextBeforeCursor()
	currentWord := p.completionWord(doc)

	if currentWord == "" {
		// Cursor is at space or beginning, just insert the suggestion
		p.insertText(suggestion.Text)
	} else if strings.HasPrefix(suggestion.Text, currentWord) {
		// Suggestion is a completion of current word (e.g., "cre" -> "create")
		suffix := suggestion.Text[len(currentWord):]
		p.insertText(suffix)
	} else {
		// Suggestion is a replacement or subcommand
		// Check if we're at the end of a word (subcommand scenario)
		if p.cursor == len(p.buffer) || !isWordChar(p.buffer[p.cursor]) {
			// At end of word or at space, add space + suggestion
			if beforeCursor != "" && !strings.HasSuffix(beforeCursor, " ") {
				p.insertText(" ")
			}
			p.insertText(suggestion.Text)
		} else {
			// In middle of word, replace current word
			wordStart, wordEnd := p.getCurrentWordBounds()
			p.buffer = append(p.buffer[:wordStart], append([]rune(suggestion.Text), p.buffer[wordEnd:]...)...)
			p.cursor = wordStart + len([]rune(suggestion.Text))
		}
	}
}

// replaceRange overwrites the buffer's runes in r with text and leaves the
// cursor after it. A span outside the buffer, or an inverted one, is clamped
// instead of panicking: a completer's arithmetic mistake should not take the
// line editor down with it.
func (p *Prompt) replaceRange(r Range, text string) {
	start := min(max(r.Start, 0), len(p.buffer))
	end := min(max(r.End, start), len(p.buffer))

	replacement := []rune(text)
	tail := append(replacement, p.buffer[end:]...)
	p.buffer = append(p.buffer[:start:start], tail...)
	p.cursor = start + len(replacement)
}

// getCurrentWordBounds finds the start and end positions of the current word at cursor
func (p *Prompt) getCurrentWordBounds() (start, end int) {
	// Find word start (scan backwards from cursor)
	start = p.cursor
	for start > 0 && isWordChar(p.buffer[start-1]) {
		start--
	}

	// Find word end (scan forwards from cursor)
	end = p.cursor
	for end < len(p.buffer) && isWordChar(p.buffer[end]) {
		end++
	}

	return start, end
}
