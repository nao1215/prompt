package prompt

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewRenderer(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	colorScheme := ThemeDefault

	renderer := newRenderer(&output, colorScheme)

	if renderer == nil {
		t.Error("Expected non-nil renderer")
		return
	}
	if renderer.output != &output {
		t.Error("Expected output to be set")
	}
	if renderer.colorScheme != colorScheme {
		t.Error("Expected color scheme to be set")
	}
	if renderer.lastLines != 0 {
		t.Errorf("Expected lastLines to be 0, got %d", renderer.lastLines)
	}
}

func TestRendererRender(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	colorScheme := ThemeDefault
	renderer := newRenderer(&output, colorScheme)

	err := renderer.render("$ ", "hello world", 6)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	result := output.String()
	if !strings.Contains(result, "$ ") {
		t.Error("Expected output to contain prefix")
	}
	if !strings.Contains(result, "hello world") {
		t.Error("Expected output to contain input text")
	}
}

func TestRendererRenderWithSuggestions(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	colorScheme := ThemeDefault
	renderer := newRenderer(&output, colorScheme)

	suggestions := []Suggestion{
		{Text: "hello", Description: "greeting"},
		{Text: "help", Description: "assistance"},
	}

	err := renderer.renderWithSuggestionsOffset("$ ", "he", 2, suggestions, 0, 0)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	result := output.String()
	if !strings.Contains(result, "$ ") {
		t.Error("Expected output to contain prefix")
	}
	if !strings.Contains(result, "he") {
		t.Error("Expected output to contain input text")
	}
	if !strings.Contains(result, "hello") {
		t.Error("Expected output to contain suggestion")
	}
}

func TestRendererSplitIntoLines(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderer := newRenderer(&output, ThemeDefault)

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "single line",
			input:    "hello world",
			expected: 1,
		},
		{
			name:     "multi line",
			input:    "line1\nline2\nline3",
			expected: 3,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 1,
		},
		{
			name:     "trailing newline",
			input:    "line1\nline2\n",
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lines := renderer.splitIntoLines(tt.input)
			if len(lines) != tt.expected {
				t.Errorf("splitIntoLines(%q) returned %d lines, want %d",
					tt.input, len(lines), tt.expected)
			}
		})
	}
}

func TestRendererFindCursorPosition(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderer := newRenderer(&output, ThemeDefault)

	tests := []struct {
		name     string
		input    string
		cursor   int
		expected struct {
			line int
			col  int
		}
	}{
		{
			name:   "simple case",
			input:  "hello",
			cursor: 3,
			expected: struct {
				line int
				col  int
			}{line: 0, col: 3},
		},
		{
			name:   "cursor at start",
			input:  "hello",
			cursor: 0,
			expected: struct {
				line int
				col  int
			}{line: 0, col: 0},
		},
		{
			name:   "multiline input",
			input:  "line1\nline2",
			cursor: 7, // Position in "line2"
			expected: struct {
				line int
				col  int
			}{line: 1, col: 1}, // Second line, first char
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inputRunes := []rune(tt.input)
			line, col := renderer.findCursorPosition(inputRunes, tt.cursor)
			if line != tt.expected.line {
				t.Errorf("Expected line %d, got %d", tt.expected.line, line)
			}
			if col != tt.expected.col {
				t.Errorf("Expected col %d, got %d", tt.expected.col, col)
			}
		})
	}
}

func TestRendererPositionCursor(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderer := newRenderer(&output, ThemeDefault)

	lines := []string{"line1", "line2", "line3"}

	// This mainly tests that the method doesn't crash
	renderer.positionCursor(lines, 1, 2, 2)

	// Check that some output was written
	result := output.String()
	// positionCursor writes ANSI escape sequences
	if len(result) == 0 {
		t.Error("Expected some output from positionCursor")
	}
}

func TestRendererMultipleRenders(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderer := newRenderer(&output, ThemeDefault)

	// First render
	err := renderer.render("$ ", "hello", 5)
	if err != nil {
		t.Errorf("First render failed: %v", err)
	}

	// Second render should clear previous
	output.Reset()
	err = renderer.render("$ ", "world", 5)
	if err != nil {
		t.Errorf("Second render failed: %v", err)
	}

	result := output.String()
	if strings.Contains(result, "hello") {
		t.Error("Previous render should be cleared")
	}
	if !strings.Contains(result, "world") {
		t.Error("Current render should be visible")
	}
}
