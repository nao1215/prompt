package prompt

import (
	"testing"
)

// Tests for the line editor: what a keystroke does to the buffer and to the
// cursor, and the questions the read loop asks about the line it is on. Nothing
// here touches a terminal, because nothing in editor.go does.

func TestPromptBufferManipulation(t *testing.T) {
	t.Parallel()

	mock := &mockTerminal{}
	p := &Prompt{
		config:   Config{Prefix: "test> "},
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

	config := Config{Prefix: "$ "}
	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test findWordBoundary
	p.buffer = []rune("hello world test")
	p.cursor = 6 // Position after "hello "

	// Test moving forward (Ctrl+Right)
	newPos := p.findWordBoundary(1)
	if newPos != 11 { // Should move to start of "test"
		t.Errorf("Expected cursor at position 11, got %d", newPos)
	}

	// Test moving backward (Ctrl+Left)
	p.cursor = 11
	newPos = p.findWordBoundary(-1)
	if newPos != 6 { // Should move to start of "world"
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

	config := Config{
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

	config := Config{
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

	config := Config{
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
		config: Config{
			Prefix: "test> ",
			HistoryConfig: &HistoryConfig{
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
		config: Config{
			Prefix: "test> ",
			HistoryConfig: &HistoryConfig{
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
