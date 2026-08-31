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
