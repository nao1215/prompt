package prompt

import (
	"context"
	"io"
	"testing"
)

// Tests for the line editor: what a keystroke does to the buffer and to the
// cursor, and the questions the read loop asks about the line it is on. Nothing
// here touches a terminal, because nothing in editor.go does.

func TestPromptBufferManipulation(t *testing.T) {
	t.Parallel()

	mock := &mockTerminal{}
	p := &Prompt{
		config:   options{Prefix: "test> "},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
		buffer:   []rune{},
		cursor:   0,
	}

	// Test insertRune
	p.insertRune('a')
	if string(p.buffer) != "a" {
		t.Errorf("Expected buffer 'a', got %q", string(p.buffer))
	}
	if p.cursor != 1 {
		t.Errorf("Expected cursor position 1, got %d", p.cursor)
	}

	// Test insertText
	p.insertText("bc")
	if string(p.buffer) != "abc" {
		t.Errorf("Expected buffer 'abc', got %q", string(p.buffer))
	}
	if p.cursor != 3 {
		t.Errorf("Expected cursor position 3, got %d", p.cursor)
	}

	// Test setBuffer
	p.setBuffer("hello")
	if string(p.buffer) != "hello" {
		t.Errorf("Expected buffer 'hello', got %q", string(p.buffer))
	}
	if p.cursor != 5 {
		t.Errorf("Expected cursor position 5, got %d", p.cursor)
	}
}

func TestWordBoundaryFunctions(t *testing.T) {
	t.Parallel()

	config := options{Prefix: "$ "}
	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Forward ends at the word ahead and backward starts at the word behind,
	// which is what readline does and what Ctrl+W deletes back to. The comment
	// here used to name position 11 the start of "test"; it is the end of
	// "world", and a reader who believed the comment would have "fixed" the code
	// to move a word further.
	p.buffer = []rune("hello world test")
	p.cursor = 6 // the "w" of "world"

	// Ctrl+Right
	newPos := p.findWordBoundary(1)
	if newPos != 11 { // the end of "world", where the space before "test" is
		t.Errorf("Expected cursor at position 11, got %d", newPos)
	}

	// Ctrl+Left
	p.cursor = 11
	newPos = p.findWordBoundary(-1)
	if newPos != 6 { // the start of "world"
		t.Errorf("Expected cursor at position 6, got %d", newPos)
	}
}

func TestIsWordChar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		char     rune
		expected bool
	}{
		{'a', true},
		{'Z', true},
		{'0', true},
		{'9', true},
		{'_', true},
		{' ', false},
		{'-', false},
		{'!', false},
		{'@', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			result := isWordChar(tt.char)
			if result != tt.expected {
				t.Errorf("isWordChar(%q) = %v, want %v", tt.char, result, tt.expected)
			}
		})
	}
}

func TestPromptInsertRuneAdvanced(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test inserting a rune
	p.insertRune('a')
	if string(p.buffer) != "a" {
		t.Errorf("Expected buffer 'a', got %q", string(p.buffer))
	}

	// Test inserting another rune
	p.insertRune('b')
	if string(p.buffer) != "ab" {
		t.Errorf("Expected buffer 'ab', got %q", string(p.buffer))
	}

	// Test cursor position after insert
	if p.cursor != 2 {
		t.Errorf("Expected cursor position 2, got %d", p.cursor)
	}
}

func TestPromptInsertTextAdvanced(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test inserting text
	p.insertText("hello")
	if string(p.buffer) != "hello" {
		t.Errorf("Expected buffer 'hello', got %q", string(p.buffer))
	}

	// Test inserting more text
	p.insertText(" world")
	if string(p.buffer) != "hello world" {
		t.Errorf("Expected buffer 'hello world', got %q", string(p.buffer))
	}

	// Test cursor position after insert
	if p.cursor != 11 {
		t.Errorf("Expected cursor position 11, got %d", p.cursor)
	}
}

func TestPromptSetBufferAdvanced(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "initial")
	defer p.Close()

	// Test setting buffer
	p.setBuffer("new text")
	if string(p.buffer) != "new text" {
		t.Errorf("Expected buffer 'new text', got %q", string(p.buffer))
	}

	// Test cursor is set to end
	if p.cursor != len(p.buffer) {
		t.Errorf("Expected cursor at end (%d), got %d", len(p.buffer), p.cursor)
	}

	// Test setting empty buffer
	p.setBuffer("")
	if string(p.buffer) != "" {
		t.Errorf("Expected empty buffer, got %q", string(p.buffer))
	}
	if p.cursor != 0 {
		t.Errorf("Expected cursor at 0, got %d", p.cursor)
	}
}

func TestMultilineNavigation(t *testing.T) {
	// Create a prompt for testing multiline functions
	p := &Prompt{
		config: options{
			Prefix: "test> ",
			historyConfig: &historyConfig{
				Enabled:    true,
				MaxEntries: 100,
			},
		},
		terminal: newMockTerminal(""),
		keyMap:   NewDefaultKeyMap(),
		history:  []string{},
	}

	// Test findLineStart
	t.Run("findLineStart", func(t *testing.T) {
		// Single line
		p.buffer = []rune("hello world")
		p.cursor = 6
		start := p.findLineStart()
		if start != 0 {
			t.Errorf("Expected line start 0, got %d", start)
		}

		// Multiple lines - cursor in middle of second line
		p.buffer = []rune("first line\nsecond line\nthird line")
		p.cursor = 17 // Position in "second line"
		start = p.findLineStart()
		expected := 11 // Start of "second line"
		if start != expected {
			t.Errorf("Expected line start %d, got %d", expected, start)
		}

		// Cursor at beginning of line
		p.cursor = 11
		start = p.findLineStart()
		if start != 11 {
			t.Errorf("Expected line start 11, got %d", start)
		}

		// Cursor at newline
		p.cursor = 10 // At the '\n' between first and second line
		start = p.findLineStart()
		if start != 0 {
			t.Errorf("Expected line start 0, got %d", start)
		}
	})

	// Test findLineEnd
	t.Run("findLineEnd", func(t *testing.T) {
		// Single line
		p.buffer = []rune("hello world")
		p.cursor = 6
		end := p.findLineEnd()
		if end != 11 {
			t.Errorf("Expected line end 11, got %d", end)
		}

		// Multiple lines - cursor in middle of second line
		p.buffer = []rune("first line\nsecond line\nthird line")
		p.cursor = 17 // Position in "second line"
		end = p.findLineEnd()
		expected := 22 // End of "second line"
		if end != expected {
			t.Errorf("Expected line end %d, got %d", expected, end)
		}

		// Cursor at end of line
		p.cursor = 22
		end = p.findLineEnd()
		if end != 22 {
			t.Errorf("Expected line end 22, got %d", end)
		}

		// Last line without newline
		p.cursor = 28 // In "third line"
		end = p.findLineEnd()
		if end != len(p.buffer) {
			t.Errorf("Expected line end %d, got %d", len(p.buffer), end)
		}
	})

	// Test findCursorUp
	t.Run("findCursorUp", func(t *testing.T) {
		// Single line - should stay at current position
		p.buffer = []rune("hello world")
		p.cursor = 6
		newPos := p.findCursorUp()
		if newPos != 6 {
			t.Errorf("Expected cursor to stay at 6, got %d", newPos)
		}

		// Multiple lines - move from second to first line
		p.buffer = []rune("first line\nsecond line\nthird line")
		p.cursor = 17 // Position 6 in "second line" (s-e-c-o-n-d)
		newPos = p.findCursorUp()
		expected := 6 // Same column position in "first line" (l-i-n-e)
		if newPos != expected {
			t.Errorf("Expected cursor at %d, got %d", expected, newPos)
		}

		// Move from third to second line
		p.cursor = 28 // Position 5 in "third line"
		newPos = p.findCursorUp()
		expected = 16 // Position 5 in "second line"
		if newPos != expected {
			t.Errorf("Expected cursor at %d, got %d", expected, newPos)
		}

		// Column beyond previous line length
		p.buffer = []rune("short\nthis is a very long line\nend")
		p.cursor = 20 // Far position in long line
		newPos = p.findCursorUp()
		expected = 5 // End of "short" line
		if newPos != expected {
			t.Errorf("Expected cursor at %d, got %d", expected, newPos)
		}

		// Already at first line
		p.cursor = 3
		newPos = p.findCursorUp()
		if newPos != 3 {
			t.Errorf("Expected cursor to stay at 3, got %d", newPos)
		}
	})

	// Test findCursorDown
	t.Run("findCursorDown", func(t *testing.T) {
		// Single line - should stay at current position
		p.buffer = []rune("hello world")
		p.cursor = 6
		newPos := p.findCursorDown()
		if newPos != 6 {
			t.Errorf("Expected cursor to stay at 6, got %d", newPos)
		}

		// Multiple lines - move from first to second line
		p.buffer = []rune("first line\nsecond line\nthird line")
		p.cursor = 6 // Position 6 in "first line"
		newPos = p.findCursorDown()
		expected := 17 // Same column position in "second line"
		if newPos != expected {
			t.Errorf("Expected cursor at %d, got %d", expected, newPos)
		}

		// Move from second to third line
		p.cursor = 16 // Position 5 in "second line"
		newPos = p.findCursorDown()
		expected = 28 // Position 5 in "third line"
		if newPos != expected {
			t.Errorf("Expected cursor at %d, got %d", expected, newPos)
		}

		// Column beyond next line length
		p.buffer = []rune("this is a very long line\nshort\nend")
		p.cursor = 20 // Far position in long line
		newPos = p.findCursorDown()
		expected = 30 // End of "short" line
		if newPos != expected {
			t.Errorf("Expected cursor at %d, got %d", expected, newPos)
		}

		// Already at last line
		p.buffer = []rune("first\nsecond")
		p.cursor = 8 // In "second"
		newPos = p.findCursorDown()
		if newPos != 8 {
			t.Errorf("Expected cursor to stay at 8, got %d", newPos)
		}
	})
}

func TestMultilineEdgeCases(t *testing.T) {
	p := &Prompt{
		config: options{
			Prefix: "test> ",
			historyConfig: &historyConfig{
				Enabled:    true,
				MaxEntries: 100,
			},
		},
		terminal: newMockTerminal(""),
		keyMap:   NewDefaultKeyMap(),
		history:  []string{},
	}

	t.Run("EmptyBuffer", func(t *testing.T) {
		p.buffer = []rune{}
		p.cursor = 0

		start := p.findLineStart()
		if start != 0 {
			t.Errorf("Expected line start 0 for empty buffer, got %d", start)
		}

		end := p.findLineEnd()
		if end != 0 {
			t.Errorf("Expected line end 0 for empty buffer, got %d", end)
		}

		up := p.findCursorUp()
		if up != 0 {
			t.Errorf("Expected cursor up 0 for empty buffer, got %d", up)
		}

		down := p.findCursorDown()
		if down != 0 {
			t.Errorf("Expected cursor down 0 for empty buffer, got %d", down)
		}
	})

	t.Run("OnlyNewlines", func(t *testing.T) {
		p.buffer = []rune("\n\n\n")
		p.cursor = 2 // Second newline

		start := p.findLineStart()
		if start != 2 {
			t.Errorf("Expected line start 2, got %d", start)
		}

		end := p.findLineEnd()
		if end != 2 {
			t.Errorf("Expected line end 2, got %d", end)
		}

		up := p.findCursorUp()
		if up != 1 {
			t.Errorf("Expected cursor up 1, got %d", up)
		}

		down := p.findCursorDown()
		if down != 3 {
			t.Errorf("Expected cursor down 3, got %d", down)
		}
	})

	t.Run("CursorAtBoundaries", func(t *testing.T) {
		p.buffer = []rune("abc\ndef\nghi")

		// Cursor at very beginning
		p.cursor = 0
		start := p.findLineStart()
		if start != 0 {
			t.Errorf("Expected line start 0, got %d", start)
		}

		// Cursor at very end
		p.cursor = len(p.buffer)
		end := p.findLineEnd()
		if end != len(p.buffer) {
			t.Errorf("Expected line end %d, got %d", len(p.buffer), end)
		}

		// Test navigation from boundaries
		p.cursor = 0
		down := p.findCursorDown()
		if down != 4 { // Same column in next line
			t.Errorf("Expected cursor down 4, got %d", down)
		}

		p.cursor = len(p.buffer)
		up := p.findCursorUp()
		if up != 7 { // Same column in previous line
			t.Errorf("Expected cursor up 7, got %d", up)
		}
	})

	t.Run("UnicodeCharacters", func(t *testing.T) {
		p.buffer = []rune("こんにちは\n世界\nテスト")
		p.cursor = 7 // In "世界"

		start := p.findLineStart()
		if start != 6 {
			t.Errorf("Expected line start 6, got %d", start)
		}

		end := p.findLineEnd()
		if end != 8 {
			t.Errorf("Expected line end 8, got %d", end)
		}

		up := p.findCursorUp()
		if up != 1 { // Same position in first line
			t.Errorf("Expected cursor up 1, got %d", up)
		}

		down := p.findCursorDown()
		if down != 10 { // Same position in third line
			t.Errorf("Expected cursor down 10, got %d", down)
		}
	})
}

// TestWordBoundaryKeepsACombiningMarkWithItsLetter covers a word written in
// decomposed form, where an accent is a rune of its own following the letter it
// belongs to. A mark is not a letter, so it was a word separator: Ctrl+Right
// stopped between the letter and its accent, which puts the cursor inside a
// character. Backspace there deletes the accent and leaves the letter bare, and
// the renderer draws the cursor on the letter's own cell, so the screen and the
// buffer disagree about where the cursor is.
//
// macOS returns filenames in this form, so a pasted path carries it.
func TestWordBoundaryKeepsACombiningMarkWithItsLetter(t *testing.T) {
	t.Parallel()

	const (
		decomposed  = "caf\u0065\u0301" // e followed by a combining acute
		precomposed = "caf\u00e9"       // one rune, and a letter
	)

	tests := []struct {
		name string
		word string
	}{
		{name: "decomposed", word: decomposed},
		{name: "precomposed", word: precomposed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			forward := &Prompt{buffer: []rune(tt.word + " au lait"), cursor: 0}
			end := forward.findWordBoundary(1)
			if got := string(forward.buffer[:end]); got != tt.word {
				t.Errorf("findWordBoundary(1) from the start of the line ended at %d, leaving %q, want %q: the cursor is inside a character", end, got, tt.word)
			}

			backward := &Prompt{buffer: []rune("x " + tt.word)}
			backward.cursor = len(backward.buffer)
			start := backward.findWordBoundary(-1)
			if got := string(backward.buffer[start:]); got != tt.word {
				t.Errorf("findWordBoundary(-1) from the end of the line started at %d, leaving %q, want %q", start, got, tt.word)
			}
		})
	}

	t.Run("a mark does not make a word out of what is not one", func(t *testing.T) {
		t.Parallel()

		// A mark only ever follows the character it belongs to, so counting one as
		// part of a word cannot join two words that were separate.
		p := &Prompt{buffer: []rune("a\u0301 b\u0301"), cursor: 0}
		if end := p.findWordBoundary(1); end != 2 {
			t.Errorf("findWordBoundary(1) ended at %d, want 2: the space still separates", end)
		}
	})
}

// TestATrailingBackslashContinuesTheLineInEitherMode pins the one exception to
// "Enter submits": a line ending in an odd number of backslashes opens a new
// line instead, with or without WithMultiline and whatever the WithIsComplete
// predicate says, and the last backslash is taken out of the entry because it
// said how to read the line rather than being part of it.
func TestATrailingBackslashContinuesTheLineInEitherMode(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		script string
		opts   []Option
		want   string
	}{
		{name: "without multiline", script: "a \\\rb\r", want: "a \nb"},
		{name: "with multiline", script: "a \\\rb\r", opts: []Option{WithMultiline()}, want: "a \nb"},
		{
			name:   "whatever the predicate says",
			script: "a \\\rb\r",
			opts:   []Option{WithMultiline(), WithIsComplete(func(string) bool { return true })},
			want:   "a \nb",
		},
		{
			name:   "the whitespace after it does not hide it",
			script: "a \\ \rb\r",
			want:   "a \nb ",
		},
		{name: "two are an escaped backslash and submit", script: "a\\\\\r", want: `a\\`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := options{Prefix: "$ ", ColorScheme: ThemeDefault, KeyMap: NewDefaultKeyMap()}
			for _, o := range tt.opts {
				o(&config)
			}
			p, err := newFromConfigOn(config, newMockTerminal(tt.script), io.Discard)
			if err != nil {
				t.Fatalf("newFromConfigOn() error = %v", err)
			}
			defer p.Close()

			got, err := p.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Run() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestEnterCountsTheBackslashesAtTheEndOfTheLine is the rule the case above is
// one line of: how many backslashes there are, not whether there is one. An odd
// number ends in a continuation marker and one is removed; an even number is
// data and submits whole, which is the only way to end an entry in a backslash.
func TestEnterCountsTheBackslashesAtTheEndOfTheLine(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		// typed is the first line, and what follows Enter is always "b".
		typed      string
		wantEntry  string
		wantSubmit bool
	}{
		{name: "none", typed: "a", wantEntry: "a", wantSubmit: true},
		{name: "one", typed: `a\`, wantEntry: "a\nb"},
		{name: "two", typed: `a\\`, wantEntry: `a\\`, wantSubmit: true},
		{name: "three", typed: `a\\\`, wantEntry: "a\\\\\nb"},
		{name: "four", typed: `a\\\\`, wantEntry: `a\\\\`, wantSubmit: true},
		{name: "one, with spaces after it", typed: `a\  `, wantEntry: "a\nb  "},
		{name: "two, with spaces after it", typed: `a\\  `, wantEntry: `a\\  `, wantSubmit: true},
		{name: "a line of one backslash", typed: `\`, wantEntry: "\nb"},
		{name: "a line of two backslashes", typed: `\\`, wantEntry: `\\`, wantSubmit: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			script := tt.typed + "\r"
			if !tt.wantSubmit {
				script += "b\r"
			}

			p, err := newFromConfigOn(options{
				Prefix:      "$ ",
				ColorScheme: ThemeDefault,
				KeyMap:      NewDefaultKeyMap(),
			}, newMockTerminal(script), io.Discard)
			if err != nil {
				t.Fatalf("newFromConfigOn() error = %v", err)
			}
			defer p.Close()

			got, err := p.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != tt.wantEntry {
				t.Errorf("typing %q then Enter gave %q, want %q", tt.typed, got, tt.wantEntry)
			}
		})
	}
}
