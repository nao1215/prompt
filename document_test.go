package prompt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDocumentIndexesByRune pins the unit CursorPosition is counted in. The
// prompt's cursor is an index into a []rune buffer, so a Document handed to a
// completer must be read the same way; reading it as a byte offset cut a
// multi-byte identifier in half and hid the word the user was typing.
func TestDocumentIndexesByRune(t *testing.T) {
	t.Parallel()

	// "SELECT * FROM 日本語 WHERE na" — 14 ASCII runes, 3 multi-byte runes, then
	// the rest. The cursor sits at the end, which is rune 25 and byte 31.
	const text = "SELECT * FROM 日本語 WHERE na"
	doc := Document{Text: text, CursorPosition: len([]rune(text))}

	t.Run("TextBeforeCursor returns the whole text when the cursor is at its end", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, text, doc.TextBeforeCursor())
	})

	t.Run("GetWordBeforeCursor returns the word being typed after a multi-byte one", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "na", doc.GetWordBeforeCursor())
	})

	t.Run("GetWordBeforeCursorEscaped returns the same word", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "na", doc.GetWordBeforeCursorEscaped())
	})

	t.Run("TextAfterCursor is empty at the end of the text", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", doc.TextAfterCursor())
	})
}

// TestDocumentSplitsAtARuneBoundary checks a cursor parked inside the
// multi-byte run rather than after it: the split must fall between runes.
func TestDocumentSplitsAtARuneBoundary(t *testing.T) {
	t.Parallel()

	doc := Document{Text: "日本語", CursorPosition: 2}

	assert.Equal(t, "日本", doc.TextBeforeCursor())
	assert.Equal(t, "語", doc.TextAfterCursor())
	assert.Equal(t, "日本", doc.GetWordBeforeCursor())
}

func TestDocumentMethods(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		text           string
		cursorPos      int
		expectedBefore string
		expectedAfter  string
		expectedWord   string
		expectedLine   string
	}{
		{
			name:           "basic text",
			text:           "hello world",
			cursorPos:      6,
			expectedBefore: "hello ",
			expectedAfter:  "world",
			expectedWord:   "", // Cursor is after space, so no current word
			expectedLine:   "hello world",
		},
		{
			name:           "cursor at start",
			text:           "hello world",
			cursorPos:      0,
			expectedBefore: "",
			expectedAfter:  "hello world",
			expectedWord:   "",
			expectedLine:   "hello world",
		},
		{
			name:           "cursor at end",
			text:           "hello world",
			cursorPos:      11,
			expectedBefore: "hello world",
			expectedAfter:  "",
			expectedWord:   "world",
			expectedLine:   "hello world",
		},
		{
			name:           "cursor out of bounds negative",
			text:           "hello world",
			cursorPos:      -1,
			expectedBefore: "hello world",
			expectedAfter:  "",
			expectedWord:   "world",
			expectedLine:   "hello world",
		},
		{
			name:           "cursor out of bounds positive",
			text:           "hello world",
			cursorPos:      20,
			expectedBefore: "hello world",
			expectedAfter:  "",
			expectedWord:   "world",
			expectedLine:   "hello world",
		},
		{
			name:           "multiple words",
			text:           "git commit -m message",
			cursorPos:      10,
			expectedBefore: "git commit",
			expectedAfter:  " -m message",
			expectedWord:   "commit",
			expectedLine:   "git commit -m message",
		},
		{
			name:           "empty text",
			text:           "",
			cursorPos:      0,
			expectedBefore: "",
			expectedAfter:  "",
			expectedWord:   "",
			expectedLine:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc := Document{Text: tt.text, CursorPosition: tt.cursorPos}

			before := doc.TextBeforeCursor()
			if before != tt.expectedBefore {
				t.Errorf("TextBeforeCursor() = %q, want %q", before, tt.expectedBefore)
			}

			after := doc.TextAfterCursor()
			if after != tt.expectedAfter {
				t.Errorf("TextAfterCursor() = %q, want %q", after, tt.expectedAfter)
			}

			word := doc.GetWordBeforeCursor()
			if word != tt.expectedWord {
				t.Errorf("GetWordBeforeCursor() = %q, want %q", word, tt.expectedWord)
			}

			line := doc.CurrentLine()
			if line != tt.expectedLine {
				t.Errorf("CurrentLine() = %q, want %q", line, tt.expectedLine)
			}
		})
	}
}

func TestDocumentTextMethodsAdvanced(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		text           string
		cursor         int
		expectedBefore string
		expectedAfter  string
		expectedWord   string
		expectedLine   string
	}{
		{
			name:           "cursor at beginning",
			text:           "hello world",
			cursor:         0,
			expectedBefore: "",
			expectedAfter:  "hello world",
			expectedWord:   "",
			expectedLine:   "hello world",
		},
		{
			name:           "cursor at end",
			text:           "hello world",
			cursor:         11,
			expectedBefore: "hello world",
			expectedAfter:  "",
			expectedWord:   "world",
			expectedLine:   "hello world",
		},
		{
			name:           "cursor in middle",
			text:           "hello world",
			cursor:         6,
			expectedBefore: "hello ",
			expectedAfter:  "world",
			expectedWord:   "", // Cursor is after space, so no current word
			expectedLine:   "hello world",
		},
		{
			name:           "cursor in word",
			text:           "hello world",
			cursor:         8,
			expectedBefore: "hello wo",
			expectedAfter:  "rld",
			expectedWord:   "wo",
			expectedLine:   "hello world",
		},
		{
			name:           "multiline text",
			text:           "line1\nline2\nline3",
			cursor:         8,
			expectedBefore: "line1\nli",
			expectedAfter:  "ne2\nline3",
			expectedWord:   "li",
			expectedLine:   "line2",
		},
		{
			name:           "the cursor on the first line of several",
			text:           "select 1,\nfrom t",
			cursor:         4,
			expectedBefore: "sele",
			expectedAfter:  "ct 1,\nfrom t",
			expectedWord:   "sele",
			expectedLine:   "select 1,",
		},
		{
			name:           "the cursor on the last line of several",
			text:           "select 1,\nfrom t",
			cursor:         12,
			expectedBefore: "select 1,\nfr",
			expectedAfter:  "om t",
			expectedWord:   "fr",
			expectedLine:   "from t",
		},
		{
			name:           "the cursor on the line break itself",
			text:           "select 1,\nfrom t",
			cursor:         9,
			expectedBefore: "select 1,",
			expectedAfter:  "\nfrom t",
			expectedWord:   "1,",
			expectedLine:   "select 1,",
		},
		{
			name:           "the cursor on an empty line between two others",
			text:           "one\n\nthree",
			cursor:         4,
			expectedBefore: "one\n",
			expectedAfter:  "\nthree",
			expectedWord:   "",
			expectedLine:   "",
		},
		{
			// The position is counted in runes, so a line found by byte offsets
			// would start in the middle of one of these.
			name:           "a line written outside ASCII",
			text:           "テーブル\n名前を選ぶ",
			cursor:         7,
			expectedBefore: "テーブル\n名前",
			expectedAfter:  "を選ぶ",
			expectedWord:   "名前",
			expectedLine:   "名前を選ぶ",
		},
		{
			name:           "a cursor past the end of the text",
			text:           "one\ntwo",
			cursor:         99,
			expectedBefore: "one\ntwo",
			expectedAfter:  "",
			expectedWord:   "two",
			expectedLine:   "two",
		},
		{
			name:           "empty text",
			text:           "",
			cursor:         0,
			expectedBefore: "",
			expectedAfter:  "",
			expectedWord:   "",
			expectedLine:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc := &Document{
				Text:           tt.text,
				CursorPosition: tt.cursor,
			}

			// Test TextBeforeCursor
			before := doc.TextBeforeCursor()
			if before != tt.expectedBefore {
				t.Errorf("TextBeforeCursor() = %q, want %q", before, tt.expectedBefore)
			}

			// Test TextAfterCursor
			after := doc.TextAfterCursor()
			if after != tt.expectedAfter {
				t.Errorf("TextAfterCursor() = %q, want %q", after, tt.expectedAfter)
			}

			// Test GetWordBeforeCursor
			word := doc.GetWordBeforeCursor()
			if word != tt.expectedWord {
				t.Errorf("GetWordBeforeCursor() = %q, want %q", word, tt.expectedWord)
			}

			// Test CurrentLine
			line := doc.CurrentLine()
			if line != tt.expectedLine {
				t.Errorf("CurrentLine() = %q, want %q", line, tt.expectedLine)
			}
		})
	}
}
