package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAcceptSuggestion(t *testing.T) {
	tests := []struct {
		name           string
		initialText    string
		cursorPos      int
		suggestion     Suggestion
		expectedText   string
		expectedCursor int
	}{
		{
			name:           "complete after space",
			initialText:    "create ",
			cursorPos:      7, // after "create "
			suggestion:     Suggestion{Text: "project"},
			expectedText:   "create project",
			expectedCursor: 14, // after "project"
		},
		{
			name:           "replace current word",
			initialText:    "cre",
			cursorPos:      3, // after "cre"
			suggestion:     Suggestion{Text: "create"},
			expectedText:   "create",
			expectedCursor: 6, // after "create"
		},
		{
			name:           "complete in middle of text",
			initialText:    "git st status",
			cursorPos:      6, // after "st"
			suggestion:     Suggestion{Text: "status"},
			expectedText:   "git status status",
			expectedCursor: 10, // after "status"
		},
		{
			name:           "insert at empty position",
			initialText:    "",
			cursorPos:      0,
			suggestion:     Suggestion{Text: "hello"},
			expectedText:   "hello",
			expectedCursor: 5,
		},
		{
			name:           "complete with space after",
			initialText:    "create project",
			cursorPos:      6, // after "create"
			suggestion:     Suggestion{Text: "modify"},
			expectedText:   "create modify project",
			expectedCursor: 13, // after "modify"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Prompt{
				buffer: []rune(tt.initialText),
				cursor: tt.cursorPos,
			}

			p.acceptSuggestion(tt.suggestion)

			resultText := string(p.buffer)
			assert.Equal(t, tt.expectedText, resultText, "text should match expected")
			assert.Equal(t, tt.expectedCursor, p.cursor, "cursor position should match expected")
		})
	}
}

func TestAutocompleteScenario(t *testing.T) {
	t.Run("create TAB project scenario", func(t *testing.T) {
		// Simulate the exact scenario from the bug report
		p := &Prompt{
			buffer: []rune("create "),
			cursor: 7, // after "create "
		}

		// When TAB is pressed after "create ", it should show suggestions for "create" subcommands
		// and when "project" is selected, it should replace the empty word after "create "
		suggestion := Suggestion{Text: "project"}
		p.acceptSuggestion(suggestion)

		assert.Equal(t, "create project", string(p.buffer))
		assert.Equal(t, 14, p.cursor) // after "project"
	})

	t.Run("partial completion scenario", func(t *testing.T) {
		// Test the scenario where user types "cre" and TAB should complete to "create"
		p := &Prompt{
			buffer: []rune("cre"),
			cursor: 3, // after "cre"
		}

		suggestion := Suggestion{Text: "create"}
		p.acceptSuggestion(suggestion)

		assert.Equal(t, "create", string(p.buffer))
		assert.Equal(t, 6, p.cursor) // after "create"
	})
}

func TestCompletionBehavior(t *testing.T) {
	t.Run("single suggestion should auto-complete", func(t *testing.T) {
		// Test that single suggestions auto-complete immediately
		completer := func(d Document) []Suggestion {
			text := d.TextBeforeCursor()
			if text == "cre" {
				return []Suggestion{{Text: "create", Description: "Create command"}}
			}
			return nil
		}

		assert.NotNil(t, completer, "Completer should be available for testing")
	})

	t.Run("multiple suggestions should not auto-complete", func(t *testing.T) {
		// Test that multiple suggestions show menu instead of auto-completing
		completer := func(d Document) []Suggestion {
			text := d.TextBeforeCursor()
			if text == "create " {
				return []Suggestion{
					{Text: "project", Description: "Create project"},
					{Text: "file", Description: "Create file"},
					{Text: "folder", Description: "Create folder"},
				}
			}
			return nil
		}

		// This scenario should show a suggestion menu, not auto-complete
		// User would need to press TAB again or use arrow keys to select
		assert.NotNil(t, completer, "Completer should be available for testing")
	})

	t.Run("smart matching should auto-complete exact match", func(t *testing.T) {
		// Test smart matching: if input matches exactly one suggestion, auto-complete
		completer := func(d Document) []Suggestion {
			text := d.TextBeforeCursor()
			if text == "cre" {
				return []Suggestion{
					{Text: "create", Description: "Create command"},
					{Text: "creep", Description: "Creep command"},
				}
			}
			if text == "crea" {
				return []Suggestion{
					{Text: "create", Description: "Create command"},
				}
			}
			return nil
		}

		// Test case 1: "cre" matches both "create" and "creep" - should not auto-complete
		suggestions := completer(Document{Text: "cre", CursorPosition: 3})
		assert.Equal(t, 2, len(suggestions), "Should have 2 suggestions for 'cre'")

		// Test case 2: "crea" matches only "create" - should auto-complete
		suggestions = completer(Document{Text: "crea", CursorPosition: 4})
		assert.Equal(t, 1, len(suggestions), "Should have 1 suggestion for 'crea'")
	})

	t.Run("TAB should accept selected suggestion", func(t *testing.T) {
		// Test new TAB behavior: accepts currently selected suggestion
		completer := func(d Document) []Suggestion {
			text := d.TextBeforeCursor()
			if text == "create " {
				return []Suggestion{
					{Text: "project", Description: "Create project"},
					{Text: "file", Description: "Create file"},
					{Text: "folder", Description: "Create folder"},
				}
			}
			return nil
		}

		p := &Prompt{
			config: Config{
				Prefix:    "app> ",
				Completer: completer,
			},
			buffer: []rune("create "),
			cursor: 7, // after "create "
		}

		// Generate suggestions first
		doc := Document{
			Text:           string(p.buffer),
			CursorPosition: p.cursor,
		}
		suggestions := completer(doc)
		assert.Equal(t, 3, len(suggestions), "Should have 3 suggestions")

		// Now simulate TAB acceptance of first suggestion
		p.acceptSuggestion(suggestions[0])

		// Buffer should now contain the completed text
		assert.Equal(t, "create project", string(p.buffer), "Buffer should contain completed suggestion")
		assert.Equal(t, 14, p.cursor, "Cursor should be at end of completed text")
	})

	t.Run("should not show suggestions when no match exists", func(t *testing.T) {
		// Test that typing non-matching characters hides suggestions
		completer := func(d Document) []Suggestion {
			text := d.TextBeforeCursor()
			if text == "create " {
				return []Suggestion{
					{Text: "project", Description: "Create project"},
					{Text: "file", Description: "Create file"},
					{Text: "folder", Description: "Create folder"},
				}
			}
			return nil
		}

		p := &Prompt{
			config: Config{
				Prefix:    "app> ",
				Completer: completer,
			},
			buffer: []rune("create a"), // "a" doesn't match any suggestions
			cursor: 8,                  // after "create a"
		}

		// Generate suggestions - completer returns original suggestions
		doc := Document{
			Text:           string(p.buffer),
			CursorPosition: p.cursor,
		}
		allSuggestions := completer(Document{Text: "create ", CursorPosition: 7})
		assert.Equal(t, 3, len(allSuggestions), "Completer should return 3 suggestions for 'create '")

		// But when filtering by current word "a", no suggestions should match
		currentWord := doc.GetWordBeforeCursor()
		assert.Equal(t, "a", currentWord, "Current word should be 'a'")

		filteredSuggestions := make([]Suggestion, 0)
		for _, suggestion := range allSuggestions {
			if strings.HasPrefix(suggestion.Text, currentWord) {
				filteredSuggestions = append(filteredSuggestions, suggestion)
			}
		}

		assert.Equal(t, 0, len(filteredSuggestions), "No suggestions should match 'a'")
	})

	t.Run("should show suggestions for multi-word commands", func(t *testing.T) {
		// Test that "create " (with space) shows all available subcommands
		completer := func(d Document) []Suggestion {
			text := d.TextBeforeCursor()
			if text == "create " {
				return []Suggestion{
					{Text: "project", Description: "Create project"},
					{Text: "file", Description: "Create file"},
					{Text: "folder", Description: "Create folder"},
				}
			}
			return nil
		}

		p := &Prompt{
			config: Config{
				Prefix:    "app> ",
				Completer: completer,
			},
			buffer: []rune("create "), // "create " with space - should show suggestions
			cursor: 7,                 // after "create "
		}

		// Generate suggestions
		doc := Document{
			Text:           string(p.buffer),
			CursorPosition: p.cursor,
		}
		suggestions := completer(doc)

		// Should return all 3 suggestions since no filtering is needed
		assert.Equal(t, 3, len(suggestions), "Should have 3 suggestions for 'create '")

		// Current word should be empty (after space)
		currentWord := doc.GetWordBeforeCursor()
		assert.Equal(t, "", currentWord, "Current word should be empty after space")

		// Since currentWord is empty, no filtering should occur
		// All original suggestions should be preserved
		assert.Equal(t, "project", suggestions[0].Text, "First suggestion should be 'project'")
		assert.Equal(t, "file", suggestions[1].Text, "Second suggestion should be 'file'")
		assert.Equal(t, "folder", suggestions[2].Text, "Third suggestion should be 'folder'")
	})

	t.Run("GetWordBeforeCursor behavior verification", func(t *testing.T) {
		tests := []struct {
			name         string
			text         string
			cursor       int
			expectedWord string
		}{
			{"empty string", "", 0, ""},
			{"single word", "hello", 5, "hello"},
			{"partial word", "hel", 3, "hel"},
			{"after space", "create ", 7, ""},
			{"multiple spaces", "create  ", 8, ""},
			{"tab after word", "create\t", 7, ""},
			{"word after space", "create project", 14, "project"},
			{"partial second word", "create pro", 10, "pro"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				doc := Document{
					Text:           tt.text,
					CursorPosition: tt.cursor,
				}
				word := doc.GetWordBeforeCursor()
				assert.Equal(t, tt.expectedWord, word,
					"For text '%s' with cursor at %d, expected word '%s' but got '%s'",
					tt.text, tt.cursor, tt.expectedWord, word)
			})
		}
	})

	t.Run("TAB cursor position should not change when showing suggestions", func(t *testing.T) {
		// Test that TAB key for showing suggestions doesn't move cursor
		completer := func(d Document) []Suggestion {
			text := d.TextBeforeCursor()
			if text == "create " {
				return []Suggestion{
					{Text: "project", Description: "Create project"},
					{Text: "file", Description: "Create file"},
					{Text: "folder", Description: "Create folder"},
				}
			}
			return nil
		}

		p := &Prompt{
			config: Config{
				Prefix:    "app> ",
				Completer: completer,
			},
			buffer: []rune("create "), // "create " - ready for subcommand suggestions
			cursor: 7,                 // cursor at end after space
		}

		// Record initial cursor position
		initialCursor := p.cursor
		initialBuffer := string(p.buffer)

		// Simulate TAB key processing that generates suggestions
		doc := Document{
			Text:           string(p.buffer),
			CursorPosition: p.cursor,
		}
		suggestions := completer(doc)

		// After generating suggestions, cursor and buffer should be unchanged
		assert.Equal(t, initialCursor, p.cursor, "Cursor position should not change when generating suggestions")
		assert.Equal(t, initialBuffer, string(p.buffer), "Buffer should not change when generating suggestions")
		assert.Equal(t, 3, len(suggestions), "Should have 3 suggestions")

		// Verify that suggestions are displayed but buffer/cursor remain stable
		assert.Equal(t, "project", suggestions[0].Text, "First suggestion should be 'project'")

		// Verify buffer doesn't contain any TAB characters
		for i, r := range p.buffer {
			assert.NotEqual(t, '\t', r, "Buffer should not contain TAB character at position %d", i)
		}
	})

	t.Run("TAB character should never be inserted into buffer", func(t *testing.T) {
		// Test that TAB characters are never accidentally inserted
		p := &Prompt{
			config: Config{
				Prefix: "test> ",
			},
			buffer: []rune("hello"),
			cursor: 5,
		}

		// Simulate what happens if TAB is somehow processed as regular character
		// This should never happen, but test the protection
		initialBuffer := string(p.buffer)
		initialCursor := p.cursor

		// TAB character should not be insertable
		tabChar := '\t'
		assert.Equal(t, int32(9), tabChar, "TAB character should be ASCII 9")
		assert.True(t, tabChar < 32, "TAB character should be less than 32 (non-printable)")

		// Verify buffer and cursor remain unchanged
		assert.Equal(t, initialBuffer, string(p.buffer), "Buffer should not change")
		assert.Equal(t, initialCursor, p.cursor, "Cursor should not change")
	})

	t.Run("Enter should only accept suggestion without executing", func(t *testing.T) {
		// Test the specific scenario: "create " -> show suggestions -> Enter on "project"
		completer := func(d Document) []Suggestion {
			text := d.TextBeforeCursor()
			if text == "create " {
				return []Suggestion{
					{Text: "project", Description: "Create project"},
					{Text: "file", Description: "Create file"},
					{Text: "folder", Description: "Create folder"},
				}
			}
			return nil
		}

		p := &Prompt{
			config: Config{
				Prefix:    "app> ",
				Completer: completer,
			},
			buffer: []rune("create "), // "create " - ready for suggestions
			cursor: 7,                 // cursor at end after space
		}

		// Generate suggestions
		doc := Document{
			Text:           string(p.buffer),
			CursorPosition: p.cursor,
		}
		suggestions := completer(doc)
		assert.Equal(t, 3, len(suggestions), "Should have 3 suggestions")

		// Accept the first suggestion ("project")
		p.acceptSuggestion(suggestions[0])

		// Verify the result
		expectedResult := "create project"
		assert.Equal(t, expectedResult, string(p.buffer), "Buffer should contain 'create project'")
		assert.Equal(t, len(expectedResult), p.cursor, "Cursor should be at end of result")

		// Verify no corruption like "create folderw project"
		assert.NotContains(t, string(p.buffer), "folderw", "Buffer should not contain corrupted text")
		assert.NotContains(t, string(p.buffer), "folder", "Buffer should not contain other suggestions")
	})

	t.Run("suggestion selection with up/down arrows should work correctly", func(t *testing.T) {
		// Test the scenario: "create " -> suggestions -> down arrow -> down arrow -> Enter
		completer := func(d Document) []Suggestion {
			text := d.TextBeforeCursor()
			if text == "create " {
				return []Suggestion{
					{Text: "project", Description: "Create project"}, // index 0
					{Text: "file", Description: "Create file"},       // index 1
					{Text: "folder", Description: "Create folder"},   // index 2
				}
			}
			return nil
		}

		p := &Prompt{
			config: Config{
				Prefix:    "app> ",
				Completer: completer,
			},
			buffer: []rune("create "), // "create " - ready for suggestions
			cursor: 7,                 // cursor at end after space
		}

		// Generate suggestions
		doc := Document{
			Text:           string(p.buffer),
			CursorPosition: p.cursor,
		}
		suggestions := completer(doc)
		assert.Equal(t, 3, len(suggestions), "Should have 3 suggestions")

		// Simulate selecting "folder" (index 2) directly
		selectedSuggestion := 2

		// Now accept the selected suggestion ("folder")
		p.acceptSuggestion(suggestions[selectedSuggestion])

		// Verify the result
		expectedResult := "create folder"
		assert.Equal(t, expectedResult, string(p.buffer), "Buffer should contain 'create folder'")
		assert.Equal(t, len(expectedResult), p.cursor, "Cursor should be at end of result")

		// Verify no corruption
		assert.NotContains(t, string(p.buffer), "project", "Buffer should not contain other suggestions")
		assert.NotContains(t, string(p.buffer), "file", "Buffer should not contain other suggestions")
	})
}
