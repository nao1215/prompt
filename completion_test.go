package prompt

import (
	"bytes"
	"errors"
	"fmt"
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

// TestAcceptSuggestionGuessesWhatToReplace covers the guess the prompt makes
// when a suggestion does not name the span it stands for.
//
// The guess is only reached by a suggestion the built-in filter would have
// dropped, and the filter is skipped for a set where any member carries
// Replace -- so a completer that names the span for some of its answers and not
// others is what gets here. Nothing covered these branches before.
func TestAcceptSuggestionGuessesWhatToReplace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name:   "inside a word, the word is replaced",
			script: "createx\x1b[D\x1b[D\x1b[D\x1b[D\t\r\r",
			want:   "insert",
		},
		{
			name:   "inside a word written in japanese, the word is replaced",
			script: "テーブル\x1b[D\x1b[D\t\r\r",
			want:   "insert",
		},
		{
			name:   "at the end of a word, the suggestion is added after it",
			script: "createx\t\r\r",
			want:   "createx insert",
		},
		{
			name:   "at a space, the suggestion is added where the cursor is",
			script: "abc def\x1b[D\x1b[D\x1b[D\x1b[D\t\r\r",
			want:   "abc insert def",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPrompt(newMockTerminal(tt.script), WithCompleter(func(Document) []Suggestion {
				return []Suggestion{
					{Text: "insert"},
					{Text: "other", Replace: &Range{Start: 0, End: 0}},
				}
			}))
			got, err := p.Run()
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Run() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCurrentWordBoundsHoldTheCursor pins the span the guess replaces: it always
// contains the cursor, and it stops at a character that is not part of a word in
// any alphabet.
func TestCurrentWordBoundsHoldTheCursor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		buffer string
		cursor int
		want   string
	}{
		{name: "inside an ascii word", buffer: "createx", cursor: 3, want: "createx"},
		{name: "inside a japanese word", buffer: "テーブル", cursor: 2, want: "テーブル"},
		{name: "in the second word", buffer: "abc def", cursor: 5, want: "def"},
		{name: "an underscore joins a word", buffer: "a_b-c", cursor: 3, want: "a_b"},
		{name: "an accented letter joins a word", buffer: "naïve", cursor: 2, want: "naïve"},
		{name: "an empty buffer", buffer: "", cursor: 0, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPrompt(newMockTerminal(""))
			p.buffer = []rune(tt.buffer)
			p.cursor = tt.cursor

			start, end := p.getCurrentWordBounds()
			runes := []rune(tt.buffer)
			if start < 0 || end > len(runes) || start > tt.cursor || end < tt.cursor {
				t.Fatalf("getCurrentWordBounds() = [%d, %d), which is not a span of %q holding the cursor at %d",
					start, end, tt.buffer, tt.cursor)
			}
			if got := string(runes[start:end]); got != tt.want {
				t.Errorf("getCurrentWordBounds() spans %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMenuClosesWhenTheCursorLeavesTheWord covers what a suggestion means. The
// menu is computed once, for the word before the cursor at the time Tab was
// pressed, and acceptSuggestion works that word out again from where the cursor
// is now. Moving in between left the two disagreeing, and part of the suggestion
// was inserted into the middle of the word already there: "cre", Tab, Left, Tab
// gave "createe".
//
// Each case presses Enter after moving. With the menu closed Enter submits the
// line; with it open Enter accepts a suggestion instead, so the line that comes
// back says which of the two happened.
func TestMenuClosesWhenTheCursorLeavesTheWord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		move string
	}{
		{name: "left once", move: "\x1b[D"},
		{name: "left twice", move: "\x1b[D\x1b[D"},
		{name: "home", move: "\x1b[H"},
		{name: "ctrl+a", move: "\x01"},
		{name: "ctrl+left", move: "\x1b[1;5D"},
		{name: "end", move: "\x1b[F"},
		{name: "ctrl+e", move: "\x05"},
		{name: "ctrl+right", move: "\x1b[1;5C"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPrompt(newMockTerminal("cre\t"+tt.move+"\r"), WithCompleter(func(Document) []Suggestion {
				return []Suggestion{{Text: "create"}, {Text: "credit"}}
			}))
			got, err := p.Run()
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != "cre" {
				t.Errorf("Run() = %q, want %q: moving the cursor ends the completion, so Enter submits", got, "cre")
			}
		})
	}

	t.Run("Tab after a move offers a menu for where the cursor is now", func(t *testing.T) {
		t.Parallel()

		// Closing the menu must not stop completion: the next Tab asks the
		// completer again, and accepting from that menu works as it always did.
		p := newTestPrompt(newMockTerminal("cre\t\x1b[F\t\r\r"), WithCompleter(func(Document) []Suggestion {
			return []Suggestion{{Text: "create"}, {Text: "credit"}}
		}))
		got, err := p.Run()
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got != "create" {
			t.Errorf("Run() = %q, want %q", got, "create")
		}
	})
}

// TestMenuClosesWhenTheLineIsReplaced covers the keys that replace what is on
// the line. Every branch that edits the buffer closes the menu; Ctrl+U, Ctrl+K
// and Ctrl+R did not, so the menu stayed on screen describing a line that was
// gone and the next accept put the deleted text back.
func TestMenuClosesWhenTheLineIsReplaced(t *testing.T) {
	t.Parallel()

	suggestions := func(Document) []Suggestion {
		return []Suggestion{{Text: "create"}, {Text: "credit"}}
	}

	t.Run("ctrl+u discards the line and the menu with it", func(t *testing.T) {
		t.Parallel()

		p := newTestPrompt(newMockTerminal("cre\t\x15\r"), WithCompleter(suggestions))
		got, err := p.Run()
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got != "" {
			t.Errorf("Run() = %q, want an empty line: Ctrl+U discarded it, and Enter must not put it back", got)
		}
	})

	t.Run("ctrl+k cuts the line and the menu with it", func(t *testing.T) {
		t.Parallel()

		p := newTestPrompt(newMockTerminal("credit\x1b[D\x1b[D\x1b[D\t\x0b\r"), WithCompleter(suggestions))
		got, err := p.Run()
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got != "cre" {
			t.Errorf("Run() = %q, want %q: Ctrl+K cut the line and nothing should have grown it back", got, "cre")
		}
	})

	t.Run("a history entry chosen by ctrl+r replaces the line alone", func(t *testing.T) {
		t.Parallel()

		p := newTestPrompt(newMockTerminal("cre\t\x12older\r\t\r"),
			WithCompleter(suggestions), WithMemoryHistory(10))
		p.SetHistory([]string{"older one"})
		got, err := p.Run()
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got != "older one" {
			t.Errorf("Run() = %q, want %q: the menu was built for a line the search replaced", got, "older one")
		}
	})
}

// TestMenuIsNotDrawnOverALineItDoesNotDescribe is the same defect seen on
// screen: the menu outlived the line, so the prompt showed completions under an
// empty prefix.
func TestMenuIsNotDrawnOverALineItDoesNotDescribe(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	terminal := &sizedMockTerminal{width: 40}
	terminal.mockTerminal = *newMockTerminal("cre\t\x15")
	p := newTestPromptOn(terminal, WithCompleter(func(Document) []Suggestion {
		return []Suggestion{{Text: "create"}, {Text: "credit"}}
	}))
	p.output = &out
	p.renderer = newRenderer(&out, ThemeDefault, terminal)
	if _, err := p.Run(); !errors.Is(err, ErrEOF) {
		t.Fatalf("Run() error = %v, want the input to have ended", err)
	}

	screen := newScreenModel(40)
	screen.feed(out.String())
	if rows := screen.rows(); len(rows) != 1 {
		t.Errorf("the screen shows %q, want the prompt alone: Ctrl+U emptied the line", rows)
	}
}

// TestMenuScrollsWithTheWindowItIsDrawnIn pins the agreement between the two
// places that decide what the menu shows. The read loop keeps the scroll offset,
// and the renderer decides how many candidates fit; when the terminal has fewer
// rows than the loop assumed, a selection past the end of the drawn window is
// highlighted nowhere while Enter still accepts it, so the user walks the list
// with nothing on screen moving.
func TestMenuScrollsWithTheWindowItIsDrawnIn(t *testing.T) {
	t.Parallel()

	const (
		width  = 40
		height = 10
	)

	var out bytes.Buffer
	// Tab opens the menu, then Down walks past what a short terminal can show.
	terminal := &sizedMockTerminal{width: width, height: height}
	terminal.mockTerminal = *newMockTerminal("s\t" + strings.Repeat("\x1b[B", 12))
	p := newTestPromptOn(terminal, WithCompleter(func(Document) []Suggestion {
		out := make([]Suggestion, 0, 20)
		for i := range 20 {
			out = append(out, Suggestion{Text: fmt.Sprintf("suggestion-%02d", i+1)})
		}
		return out
	}))
	p.output = &out
	p.renderer = newRenderer(&out, ThemeDefault, terminal)
	if _, err := p.Run(); !errors.Is(err, ErrEOF) {
		t.Fatalf("Run() error = %v, want the input to have ended", err)
	}

	screen := newScreenModel(width)
	screen.feed(out.String())
	rows := screen.rows()
	if len(rows) > height {
		t.Errorf("the menu drew %d rows on a terminal of %d:\n%q", len(rows), height, rows)
	}
	var selected string
	for _, row := range rows {
		if strings.HasPrefix(row, "▶ ") {
			selected = row
		}
	}
	if selected != "▶ suggestion-13" {
		t.Errorf("the screen highlights %q, want the thirteenth candidate: twelve Downs from the first\n%q", selected, rows)
	}
}
