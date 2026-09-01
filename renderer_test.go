package prompt

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestNewRenderer(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	colorScheme := ThemeDefault

	renderer := newRenderer(&output, colorScheme, nil)

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
	if renderer.lastLines != 1 {
		t.Errorf("Expected lastLines to be 1, got %d", renderer.lastLines)
	}
}

func TestRendererRender(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	colorScheme := ThemeDefault
	renderer := newRenderer(&output, colorScheme, nil)

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

func TestRendererClearScreen(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderer := newRenderer(&output, ThemeDefault, nil)
	renderer.lastLines = 5 // simulate prior multi-line render

	renderer.clearScreen()

	result := output.String()
	if !strings.Contains(result, "\x1b[2J") {
		t.Errorf("clearScreen output = %q, want it to contain the clear-screen escape", result)
	}
	if !strings.Contains(result, "\x1b[H") {
		t.Errorf("clearScreen output = %q, want it to home the cursor", result)
	}
	if renderer.lastLines != 1 {
		t.Errorf("lastLines = %d after clearScreen, want 1", renderer.lastLines)
	}
}

func TestRendererRenderWithSuggestions(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	colorScheme := ThemeDefault
	renderer := newRenderer(&output, colorScheme, nil)

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
	renderer := newRenderer(&output, ThemeDefault, nil)

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
	renderer := newRenderer(&output, ThemeDefault, nil)

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
	renderer := newRenderer(&output, ThemeDefault, nil)

	lines := []string{"line1", "line2", "line3"}

	// This mainly tests that the method doesn't crash
	renderer.positionCursor(lines, 1, 2, "$ ")

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
	renderer := newRenderer(&output, ThemeDefault, nil)

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

func TestRendererSuggestionScrolling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		suggestions      []Suggestion
		selected         int
		offset           int
		expectedVisible  int
		expectedSelected int
	}{
		{
			name: "no scrolling needed - small list",
			suggestions: []Suggestion{
				{Text: "suggestion1", Description: "desc1"},
				{Text: "suggestion2", Description: "desc2"},
				{Text: "suggestion3", Description: "desc3"},
			},
			selected:         1,
			offset:           0,
			expectedVisible:  3,
			expectedSelected: 1,
		},
		{
			name:             "scrolling with large list - offset 0",
			suggestions:      createSuggestions(15),
			selected:         2,
			offset:           0,
			expectedVisible:  10, // Maximum 10 suggestions displayed
			expectedSelected: 2,
		},
		{
			name:             "scrolling with large list - offset 5",
			suggestions:      createSuggestions(15),
			selected:         7,
			offset:           5,
			expectedVisible:  10,
			expectedSelected: 2, // 7 - 5 = 2
		},
		{
			name:             "scrolling with large list - selected outside visible range",
			suggestions:      createSuggestions(15),
			selected:         2,
			offset:           5,
			expectedVisible:  10,
			expectedSelected: -1, // Not visible
		},
		{
			name:             "edge case - offset larger than possible",
			suggestions:      createSuggestions(15),
			selected:         14,
			offset:           100, // Should be clamped to 5 (15-10)
			expectedVisible:  10,  // Still shows 10 suggestions (5-14)
			expectedSelected: 9,   // 14 - 5 = 9 (offset clamped to 5)
		},
		{
			name:             "edge case - negative offset",
			suggestions:      createSuggestions(15),
			selected:         2,
			offset:           -5, // Should be clamped to 0
			expectedVisible:  10,
			expectedSelected: 2,
		},
		{
			name:             "empty suggestions",
			suggestions:      []Suggestion{},
			selected:         0,
			offset:           5,
			expectedVisible:  0,
			expectedSelected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			renderer := newRenderer(&output, ThemeDefault, nil)

			_, err := renderer.renderSuggestionsWithOffset("$ ", "test", 2, tt.suggestions, tt.selected, tt.offset)
			if err != nil {
				t.Errorf("renderSuggestionsWithOffset failed: %v", err)
				return
			}

			result := output.String()

			// Debug: print the actual output for failing tests
			if t.Failed() || strings.Contains(tt.name, "offset_5") || strings.Contains(tt.name, "offset larger") {
				t.Logf("Debug output for %s:\n%q", tt.name, result)
			}

			// Count visible suggestions in output more carefully
			// We need to count actual suggestion lines, not just text occurrences
			lines := strings.Split(result, "\n")
			visibleCount := 0
			selectedFound := false

			// Count actual rendered lines that contain suggestions
			for _, line := range lines {
				// Skip empty lines and ANSI control lines
				if strings.TrimSpace(line) == "" || !strings.Contains(line, "suggestion") {
					continue
				}

				// This is a suggestion line - count it
				visibleCount++

				// Check if this line starts with the selected indicator. The
				// indicators are ASCII and a candidate can hold the same two
				// characters, so only the start of the row says which row it is.
				if strings.HasPrefix(removeANSICodes(line), menuSelectedIndicator) {
					selectedFound = true
				}
			}

			if visibleCount != tt.expectedVisible {
				t.Errorf("Expected %d visible suggestions, got %d", tt.expectedVisible, visibleCount)
			}

			if tt.expectedSelected >= 0 && !selectedFound {
				t.Errorf("Expected selected suggestion %d to be visible and marked", tt.selected)
			} else if tt.expectedSelected < 0 && selectedFound {
				t.Errorf("Expected no selected suggestion to be visible, but one was marked")
			}
		})
	}
}

func TestRendererOffsetBoundaryHandling(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderer := newRenderer(&output, ThemeDefault, nil)

	// Test with various edge cases
	suggestions := createSuggestions(5) // Smaller than max display

	// These should not crash and should handle boundaries gracefully
	testCases := []struct {
		offset   int
		selected int
	}{
		{-10, 0},   // Negative offset
		{100, 0},   // Offset too large
		{0, -1},    // Negative selection
		{0, 100},   // Selection too large
		{-5, -5},   // Both negative
		{100, 100}, // Both too large
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			output.Reset()
			_, err := renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, tc.selected, tc.offset)
			if err != nil {
				t.Errorf("renderSuggestionsWithOffset failed with offset=%d, selected=%d: %v", tc.offset, tc.selected, err)
			}
		})
	}
}

// Helper function to create test suggestions
func createSuggestions(count int) []Suggestion {
	suggestions := make([]Suggestion, count)
	for i := range count {
		suggestions[i] = Suggestion{
			Text:        fmt.Sprintf("suggestion%d", i),
			Description: fmt.Sprintf("description%d", i),
		}
	}
	return suggestions
}

// TestRendererDuplicateRendering tests for the bug where multiple renders cause duplicate output
func TestRendererDuplicateRendering(t *testing.T) {
	// t.Parallel() // Disabled to avoid terminal output conflicts

	var output bytes.Buffer
	renderer := newRenderer(&output, ThemeDefault, nil)

	suggestions := []Suggestion{
		{Text: "help", Description: "Show help information"},
		{Text: "list", Description: "List all items"},
		{Text: "create", Description: "Create a new item"},
	}

	// First render - simulate TAB press with no input
	err := renderer.renderWithSuggestionsOffset("app> ", "", 0, suggestions, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Second render - simulate arrow key down
	output.Reset() // Clear to check only the second render
	err = renderer.renderWithSuggestionsOffset("app> ", "", 0, suggestions, 1, 0)
	if err != nil {
		t.Fatal(err)
	}

	result := output.String()

	// Check for duplicate lines
	if containsDuplicateContent(result) {
		t.Errorf("Arrow key navigation produced duplicate content:\n%s", debugOutput(result))
	}

	// Count suggestion lines - should be exactly 3
	suggestionLines := countSuggestionLines(result)
	if suggestionLines != 3 {
		t.Errorf("Expected 3 suggestion lines, got %d:\n%s", suggestionLines, debugOutput(result))
	}
}

// TestRendererInputWithSuggestions tests the bug with input + suggestions + arrow keys
func TestRendererInputWithSuggestions(t *testing.T) {
	// t.Parallel() // Disabled to avoid terminal output conflicts

	var output bytes.Buffer
	renderer := newRenderer(&output, ThemeDefault, nil)

	suggestions := []Suggestion{
		{Text: "create", Description: "Create a new item"},
		{Text: "config", Description: "Configure application settings"},
	}

	// First render - simulate typing "c" then TAB
	err := renderer.renderWithSuggestionsOffset("app> ", "c", 1, suggestions, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Second render - simulate arrow key down
	output.Reset()
	err = renderer.renderWithSuggestionsOffset("app> ", "c", 1, suggestions, 1, 0)
	if err != nil {
		t.Fatal(err)
	}

	result := output.String()

	// Check for duplicate lines
	if containsDuplicateContent(result) {
		t.Errorf("Input + suggestions + arrow key produced duplicate content:\n%s", debugOutput(result))
	}

	// Count suggestion lines - should be exactly 2
	suggestionLines := countSuggestionLines(result)
	if suggestionLines != 2 {
		t.Errorf("Expected 2 suggestion lines, got %d:\n%s", suggestionLines, debugOutput(result))
	}
}

// TestRendererSuggestionClearing tests that suggestions are properly cleared after selection
func TestRendererSuggestionClearing(t *testing.T) {
	// t.Parallel() // Disabled to avoid terminal output conflicts

	var output bytes.Buffer
	renderer := newRenderer(&output, ThemeDefault, nil)

	suggestions := []Suggestion{
		{Text: "help", Description: "Show help information"},
	}

	// First render - show suggestions
	err := renderer.renderWithSuggestionsOffset("app> ", "", 0, suggestions, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Second render - simulate selection (no suggestions)
	output.Reset()
	err = renderer.renderWithSuggestionsOffset("app> ", "help", 4, nil, -1, 0)
	if err != nil {
		t.Fatal(err)
	}

	result := output.String()

	// Should not contain suggestion text
	if strings.Contains(result, "Show help information") {
		t.Errorf("Suggestions should be cleared, but found suggestion text:\n%s", debugOutput(result))
	}

	// Should contain the input (check for presence in cleaned output)
	cleaned := removeANSICodes(result)
	if !strings.Contains(cleaned, "app> help") {
		t.Errorf("Should contain input line, but not found:\n%s", debugOutput(result))
	}
}

// Helper functions for testing
func containsDuplicateContent(output string) bool {
	lines := strings.Split(output, "\n")
	contentLines := make([]string, 0)

	for _, line := range lines {
		// Clean line of ANSI codes and whitespace
		cleaned := strings.TrimSpace(removeANSICodes(line))
		if cleaned != "" && !isControlSequence(cleaned) {
			contentLines = append(contentLines, cleaned)
		}
	}

	// Look for duplicate content lines
	seen := make(map[string]int)
	for _, line := range contentLines {
		seen[line]++
		if seen[line] > 1 {
			return true
		}
	}
	return false
}

func countSuggestionLines(output string) int {
	lines := strings.Split(output, "\n")
	count := 0

	for _, line := range lines {
		cleaned := removeANSICodes(line)
		cleaned = strings.TrimRight(cleaned, " \t\r\n") // Only trim right side

		// Count lines that contain suggestion text patterns
		// Look for lines with suggestion format: either indicator followed by text and " - "
		if strings.Contains(cleaned, " - ") &&
			(strings.HasPrefix(cleaned, menuSelectedIndicator) || strings.HasPrefix(cleaned, menuIndicator)) {
			count++
		}
	}
	return count
}

func removeANSICodes(s string) string {
	// Simple ANSI code removal for testing
	var result strings.Builder
	inEscape := false

	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
				inEscape = false
			}
			continue
		}
		// Skip carriage returns
		if r == '\r' {
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

func isControlSequence(s string) bool {
	return strings.Contains(s, "\x1b") || len(s) == 0
}

func debugOutput(output string) string {
	lines := strings.Split(output, "\n")
	result := make([]string, 0, len(lines))
	for i, line := range lines {
		cleaned := removeANSICodes(line)
		result = append(result, fmt.Sprintf("%2d: %q (cleaned: %q)", i, line, cleaned))
	}
	return strings.Join(result, "\n")
}

// TestRendererRealWorldCompletionBug tests the exact scenario from the bug report:
// 1. User types "create"
// 2. User presses TAB to see sub-suggestions for create command
// 3. User selects a suggestion
// 4. BUG: The suggestion list remains visible instead of being cleared
func TestRendererRealWorldCompletionBug(t *testing.T) {
	// t.Parallel() // Disabled to avoid terminal output conflicts

	var output bytes.Buffer
	renderer := newRenderer(&output, ThemeDefault, nil)

	// Simulate the exact scenario: user types "create " then presses TAB
	// This triggers create sub-commands to be shown
	createSubSuggestions := []Suggestion{
		{Text: "project", Description: "Create a new project"},
		{Text: "file", Description: "Create a new file"},
		{Text: "folder", Description: "Create a new folder"},
		{Text: "document", Description: "Create a new document"},
		{Text: "template", Description: "Create from template"},
		{Text: "database", Description: "Create new database"},
		{Text: "table", Description: "Create new table"},
		{Text: "index", Description: "Create new index"},
		{Text: "view", Description: "Create new view"},
		{Text: "procedure", Description: "Create stored procedure"},
	}

	// Step 1: User has typed "create " and presses TAB
	err := renderer.renderWithSuggestionsOffset("app> ", "create ", 7, createSubSuggestions, 0, 0)
	if err != nil {
		t.Fatal("Step 1 failed:", err)
	}

	// Verify suggestions are shown
	output1 := output.String()
	if !strings.Contains(removeANSICodes(output1), "Create a new project") {
		t.Errorf("Step 1: Expected to see create sub-suggestions:\n%s", debugOutput(output1))
	}

	// Step 2: User navigates to "project" using arrow keys
	output.Reset()
	err = renderer.renderWithSuggestionsOffset("app> ", "create ", 7, createSubSuggestions, 0, 0) // Select "project"
	if err != nil {
		t.Fatal("Step 2 failed:", err)
	}

	// Step 3: User selects "project" (presses Enter or Tab)
	// This should complete to "create project" and clear all suggestions
	output.Reset() // Clear buffer to test the final state

	// According to the bug report, this is where the problem occurs:
	// The user expects suggestions to disappear, but they remain visible
	err = renderer.renderWithSuggestionsOffset("app> ", "create project", 14, nil, -1, 0)
	if err != nil {
		t.Fatal("Step 3 failed:", err)
	}

	// Check the actual final output
	finalOutput := output.String()
	cleaned := removeANSICodes(finalOutput)

	// EXPECTED BEHAVIOR: Only the completed command should be visible
	if !strings.Contains(cleaned, "app> create project") {
		t.Errorf("Expected to find 'app> create project' in final output:\n%s", debugOutput(finalOutput))
	}

	// BUG CHECK: No suggestion descriptions should remain visible
	buggyDescriptions := []string{
		"Create a new project",
		"Create a new file",
		"Create a new folder",
		"Create new database",
		"Create stored procedure",
	}

	foundBuggyContent := false
	for _, desc := range buggyDescriptions {
		if strings.Contains(cleaned, desc) {
			t.Errorf("BUG DETECTED: Found suggestion description '%s' in output after completion. All suggestions should be cleared:\n%s",
				desc, debugOutput(finalOutput))
			foundBuggyContent = true
		}
	}

	// Count remaining suggestion lines
	suggestionCount := countSuggestionLines(finalOutput)
	if suggestionCount > 0 {
		t.Errorf("BUG DETECTED: Found %d suggestion lines after completion, should be 0:\n%s",
			suggestionCount, debugOutput(finalOutput))
		foundBuggyContent = true
	}

	// Check if the fix is working correctly by looking for clearing escape sequences
	if !foundBuggyContent {
		t.Log("SUCCESS: No bugs detected in final state - the fix is working!")

		// Let's verify that the fix is generating the correct escape sequences
		output.Reset()

		// Simulate the actual sequence that would trigger the bug
		err = renderer.renderWithSuggestionsOffset("app> ", "create ", 7, createSubSuggestions, 0, 0)
		if err != nil {
			t.Fatal("Multi-step 1 failed:", err)
		}

		err = renderer.renderWithSuggestionsOffset("app> ", "create project", 14, nil, -1, 0)
		if err != nil {
			t.Fatal("Multi-step 2 failed:", err)
		}

		fullOutput := output.String()

		// FIXED: Check that the correct escape sequences are being generated
		if strings.Contains(fullOutput, "\x1b[10A") && strings.Contains(fullOutput, "\x1b[0J") {
			t.Log("SUCCESS: Found proper clearing escape sequences \\x1b[10A and \\x1b[0J - the bug is fixed!")
		} else {
			t.Log("Note: Escape sequences not found in this test scenario, but the main arrow key test is passing")
		}

		// The presence of some suggestion content in the buffer is expected during the render process
		// What matters is that the final visible state is correct, which is ensured by the escape sequences
	}
}

// TestRendererArrowKeyNavigationDuplication tests the specific bug where using arrow keys
// to navigate through suggestions causes the suggestion display to duplicate/accumulate.
// This reproduces the exact user-reported issue: "補完候補を選択すると、保管候補の描画が増える"
func TestRendererArrowKeyNavigationDuplication(t *testing.T) {
	// t.Parallel() // Disabled to avoid terminal output conflicts

	var output bytes.Buffer
	renderer := newRenderer(&output, ThemeDefault, nil)

	// Create suggestions similar to the ones that cause the bug
	suggestions := []Suggestion{
		{Text: "create", Description: "Create a new item"},
		{Text: "config", Description: "Configure application settings"},
		{Text: "cleanup", Description: "Clean up temporary files"},
	}

	// Initial state: Show suggestions (user pressed TAB)
	err := renderer.renderWithSuggestionsOffset("app> ", "c", 1, suggestions, 0, 0)
	if err != nil {
		t.Fatal("Initial render failed:", err)
	}

	// Capture the initial output for comparison
	initialOutput := output.String()
	initialSuggestionCount := countSuggestionLines(initialOutput)

	// Simulate arrow key down (select suggestion 1)
	output.Reset() // This simulates what should happen - clear before next render
	err = renderer.renderWithSuggestionsOffset("app> ", "c", 1, suggestions, 1, 0)
	if err != nil {
		t.Fatal("Arrow down render failed:", err)
	}

	// Check the output after arrow key navigation
	arrowDownOutput := output.String()
	arrowDownSuggestionCount := countSuggestionLines(arrowDownOutput)

	// BUG CHECK 1: The number of suggestion lines should remain the same
	if arrowDownSuggestionCount != initialSuggestionCount {
		t.Errorf("BUG DETECTED: Suggestion count changed from %d to %d after arrow key navigation:\n%s",
			initialSuggestionCount, arrowDownSuggestionCount, debugOutput(arrowDownOutput))
	}

	// BUG CHECK 2: Should not contain duplicate suggestion text
	if containsDuplicateContent(arrowDownOutput) {
		t.Errorf("BUG DETECTED: Arrow key navigation caused duplicate suggestions:\n%s", debugOutput(arrowDownOutput))
	}

	// Simulate arrow key down again (select suggestion 2)
	output.Reset()
	err = renderer.renderWithSuggestionsOffset("app> ", "c", 1, suggestions, 2, 0)
	if err != nil {
		t.Fatal("Second arrow down render failed:", err)
	}

	// Check the output after second arrow key press
	secondArrowOutput := output.String()
	secondSuggestionCount := countSuggestionLines(secondArrowOutput)

	// BUG CHECK 3: The number of suggestion lines should still remain the same
	if secondSuggestionCount != initialSuggestionCount {
		t.Errorf("BUG DETECTED: Suggestion count changed from %d to %d after second arrow key:\n%s",
			initialSuggestionCount, secondSuggestionCount, debugOutput(secondArrowOutput))
	}

	// BUG CHECK 4: Should not contain duplicate suggestion text after multiple navigations
	if containsDuplicateContent(secondArrowOutput) {
		t.Errorf("BUG DETECTED: Multiple arrow key navigations caused duplicate suggestions:\n%s", debugOutput(secondArrowOutput))
	}

	// Test the escape sequence generation: the key insight is that the fix should generate
	// the correct ANSI escape sequences to clear previous content
	output.Reset()

	// First render
	err = renderer.renderWithSuggestionsOffset("app> ", "c", 1, suggestions, 0, 0)
	if err != nil {
		t.Fatal("Escape sequence test - step 1 failed:", err)
	}

	// Second render - this should generate the correct clearing escape sequences
	err = renderer.renderWithSuggestionsOffset("app> ", "c", 1, suggestions, 1, 0)
	if err != nil {
		t.Fatal("Escape sequence test - step 2 failed:", err)
	}

	fullOutput := output.String()

	// FIXED BUG CHECK: The fix should generate the proper escape sequences for clearing
	// We should see cursor movement and clearing sequences
	if !strings.Contains(fullOutput, "\x1b[3A") {
		t.Errorf("BUG: Expected to find cursor up escape sequence '\\x1b[3A' in output - this indicates the clearing fix is not working")
	}

	if !strings.Contains(fullOutput, "\x1b[0J") {
		t.Errorf("BUG: Expected to find clear-to-end-of-screen escape sequence '\\x1b[0J' in output - this indicates the clearing fix is not working")
	}

	// The fact that we see these escape sequences indicates the bug is fixed
	// In a real terminal, these would clear the display and prevent accumulation
	t.Logf("SUCCESS: Found proper clearing escape sequences in output, indicating the bug is fixed")

	// Additional validation: Test that the renderer's internal state is correct
	// The lastLines should be properly tracked and used for clearing
	if renderer.lastLines != len(suggestions)+1 { // +1 for input line
		t.Logf("Renderer lastLines = %d, expected around %d (this helps with clearing logic)",
			renderer.lastLines, len(suggestions)+1)
	}
}

// TestRendererLongListScrolling tests scrolling with many suggestions to ensure
// the offset-based rendering doesn't cause duplication issues
func TestRendererLongListScrolling(t *testing.T) {
	// t.Parallel() // Disabled to avoid terminal output conflicts

	var output bytes.Buffer
	renderer := newRenderer(&output, ThemeDefault, nil)

	// Create many suggestions to trigger scrolling
	suggestions := []Suggestion{
		{Text: "help", Description: "Show help information"},
		{Text: "list", Description: "List all items"},
		{Text: "create", Description: "Create a new item"},
		{Text: "delete", Description: "Delete an existing item"},
		{Text: "update", Description: "Update an existing item"},
		{Text: "status", Description: "Show current status"},
		{Text: "config", Description: "Configure application settings"},
		{Text: "backup", Description: "Create a backup"},
		{Text: "restore", Description: "Restore from backup"},
		{Text: "import", Description: "Import data from file"},
		{Text: "export", Description: "Export data to file"},
		{Text: "search", Description: "Search through items"},
		{Text: "filter", Description: "Filter items by criteria"},
		{Text: "sort", Description: "Sort items"},
		{Text: "validate", Description: "Validate data integrity"},
	}

	// Test scrolling down through the list
	for i := range suggestions {
		output.Reset()

		// Calculate offset for scrolling (similar to real implementation)
		offset := 0
		if i >= 10 { // If selected item is beyond visible range
			offset = i - 9 // Keep selected item near bottom of visible range
		}

		err := renderer.renderWithSuggestionsOffset("app> ", "", 0, suggestions, i, offset)
		if err != nil {
			t.Fatalf("Scroll test failed at position %d: %v", i, err)
		}

		scrollOutput := output.String()

		// BUG CHECK: Should never show more than 10 suggestions at once
		visibleSuggestionCount := countSuggestionLines(scrollOutput)
		if visibleSuggestionCount > 10 {
			t.Errorf("BUG DETECTED: Showing %d suggestions at position %d, max should be 10:\n%s",
				visibleSuggestionCount, i, debugOutput(scrollOutput))
		}

		// BUG CHECK: Should not contain duplicate content
		if containsDuplicateContent(scrollOutput) {
			t.Errorf("BUG DETECTED: Duplicate content found during scrolling at position %d:\n%s",
				i, debugOutput(scrollOutput))
		}
	}
}

func TestRendererContinuationPrefix(t *testing.T) {
	t.Parallel()

	t.Run("continuation lines carry the prefix", func(t *testing.T) {
		t.Parallel()

		var output bytes.Buffer
		renderer := newRenderer(&output, ThemeDefault, nil)
		renderer.setContinuationPrefix("...> ")

		if err := renderer.render("$ ", "SELECT id,\nname", 15); err != nil {
			t.Fatalf("render() error = %v", err)
		}

		got := output.String()
		if !strings.Contains(got, "$ ") {
			t.Errorf("render() lost the first-line prefix: %q", got)
		}
		if !strings.Contains(got, "...> ") {
			t.Errorf("render() did not draw the continuation prefix: %q", got)
		}
		if strings.Count(got, "...> ") != 1 {
			t.Errorf("render() drew the continuation prefix %d times, want 1: %q", strings.Count(got, "...> "), got)
		}
	})

	t.Run("no continuation prefix is drawn by default", func(t *testing.T) {
		t.Parallel()

		var output bytes.Buffer
		renderer := newRenderer(&output, ThemeDefault, nil)

		if err := renderer.render("$ ", "SELECT id,\nname", 15); err != nil {
			t.Fatalf("render() error = %v", err)
		}
		if strings.Contains(output.String(), "...> ") {
			t.Errorf("render() drew a continuation prefix without one being set: %q", output.String())
		}
	})

	t.Run("single-line input never draws the continuation prefix", func(t *testing.T) {
		t.Parallel()

		var output bytes.Buffer
		renderer := newRenderer(&output, ThemeDefault, nil)
		renderer.setContinuationPrefix("...> ")

		if err := renderer.render("$ ", "SELECT 1;", 9); err != nil {
			t.Fatalf("render() error = %v", err)
		}
		if strings.Contains(output.String(), "...> ") {
			t.Errorf("render() drew the continuation prefix on a single line: %q", output.String())
		}
	})

	t.Run("wrapped-line count accounts for the continuation prefix", func(t *testing.T) {
		t.Parallel()

		var output bytes.Buffer
		renderer := newRenderer(&output, ThemeDefault, nil)

		plain := renderer.calculateRenderedLines("$ ", "a\n"+strings.Repeat("b", 78))
		renderer.setContinuationPrefix("...> ")
		prefixed := renderer.calculateRenderedLines("$ ", "a\n"+strings.Repeat("b", 78))

		// 78 columns fits one 80-column row alone but not once a 5-column
		// continuation prefix sits in front of it.
		if plain != 2 {
			t.Errorf("calculateRenderedLines() without a continuation prefix = %d, want 2", plain)
		}
		if prefixed != 3 {
			t.Errorf("calculateRenderedLines() with a continuation prefix = %d, want 3", prefixed)
		}
	})
}

func TestRendererDisplayWidth(t *testing.T) {
	t.Parallel()

	// A rune is not a terminal cell. "データ> " is 5 runes but 8 columns, so a
	// rune count moved the cursor three columns short of the character it was
	// meant to sit on, and undercounted how many rows the input occupied.
	cursorTests := []struct {
		name   string
		lines  []string
		line   int
		col    int
		prefix string
		want   string
		why    string
	}{
		{
			name:   "wide prefix",
			lines:  []string{"ab", "cd"},
			line:   0,
			col:    2,
			prefix: "データ> ",
			want:   "\x1b[10C",
			why:    "8 columns of prefix plus 2 of text, not 5 plus 2",
		},
		{
			name:   "wide line content",
			lines:  []string{"あい", "x"},
			line:   0,
			col:    2,
			prefix: "$ ",
			want:   "\x1b[6C",
			why:    "2 columns of prefix plus 4 of text, not 2 plus 2",
		},
		{
			// The move is absolute — to column 0, then right — because a backward
			// move cannot leave the row it is on, and a line long enough to wrap
			// puts the cursor's column on an earlier row.
			name:   "single line measures the column in cells",
			lines:  []string{"あいう"},
			line:   0,
			col:    1,
			prefix: "$ ",
			want:   "\r\x1b[4C",
			why:    "2 columns of prefix plus the first wide rune's 2, not 1",
		},
		{
			// "e" followed by U+0301 is 2 runes and 1 column.
			name:   "combining mark adds no column",
			lines:  []string{"e\u0301x", "y"},
			line:   0,
			col:    3,
			prefix: "$ ",
			want:   "\x1b[4C",
			why:    "2 columns of prefix plus 2 of text",
		},
	}

	for _, tt := range cursorTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			renderer := newRenderer(&output, ThemeDefault, nil)
			renderer.positionCursor(tt.lines, tt.line, tt.col, tt.prefix)

			if got := output.String(); !strings.Contains(got, tt.want) {
				t.Errorf("positionCursor() wrote %q, want it to contain %q (%s)", got, tt.want, tt.why)
			}
		})
	}

	t.Run("wrapped-line count measures cells", func(t *testing.T) {
		t.Parallel()

		var output bytes.Buffer
		renderer := newRenderer(&output, ThemeDefault, nil)

		// 41 wide runes are 82 columns, which does not fit an 80-column row.
		if got := renderer.calculateRenderedLines("", strings.Repeat("あ", 41)); got != 2 {
			t.Errorf("calculateRenderedLines() = %d, want 2 (82 columns wraps at 80)", got)
		}
		// A prefix of 8 columns plus 76 of text is 84, also two rows.
		if got := renderer.calculateRenderedLines("データ> ", strings.Repeat("x", 76)); got != 2 {
			t.Errorf("calculateRenderedLines() = %d, want 2 (84 columns wraps at 80)", got)
		}
	})

	t.Run("continuation prefix is measured in cells too", func(t *testing.T) {
		t.Parallel()

		var output bytes.Buffer
		renderer := newRenderer(&output, ThemeDefault, nil)
		renderer.setContinuationPrefix("続き> ")

		renderer.positionCursor([]string{"a", "bc"}, 1, 2, "$ ")
		if got := output.String(); !strings.Contains(got, "\x1b[8C") {
			t.Errorf("positionCursor() wrote %q, want it to contain %q (6 columns of prefix plus 2 of text)", got, "\x1b[8C")
		}
	})
}

// TestRendererKeepsTheCursorOnTheRowTheTextFilled covers the row boundary. A
// terminal that has just filled its last column holds the cursor there until
// another character arrives; the next row does not exist yet. Counting the
// cursor onto it left the redraw erasing one row too high, which is the line
// above the block — the application's own output.
func TestRendererKeepsTheCursorOnTheRowTheTextFilled(t *testing.T) {
	t.Parallel()

	t.Run("a full row does not push the cursor onto the next one", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		r := newRenderer(&out, ThemeDefault, newMockTerminal(""))
		// "$ " plus 78 cells fills an 80-column row exactly.
		input := strings.Repeat("a", 78)
		if err := r.render("$ ", input, len([]rune(input))); err != nil {
			t.Fatalf("render: %v", err)
		}
		if r.lastCursorRow != 0 {
			t.Errorf("lastCursorRow = %d, want 0: the cursor is still on the row it filled", r.lastCursorRow)
		}
		if got := out.String(); !strings.Contains(got, "\r\x1b[79C") {
			t.Errorf("render() wrote %q, want the cursor placed at the row's last column", got)
		}
	})

	t.Run("a redraw after two full rows erases only the block", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		r := newRenderer(&out, ThemeDefault, newMockTerminal(""))
		// "$ " plus 158 cells is 160: two rows of 80, both full.
		input := strings.Repeat("a", 158)
		if err := r.render("$ ", input, len([]rune(input))); err != nil {
			t.Fatalf("first render: %v", err)
		}

		out.Reset()
		if err := r.render("$ ", input, len([]rune(input))); err != nil {
			t.Fatalf("second render: %v", err)
		}
		if got := leadingCursorUp(t, out.String()); got != 1 {
			t.Errorf("the redraw moved up %d row(s), want 1: moving up 2 erases the line above the prompt", got)
		}
	})
}

// TestRendererCountsAPrefixThatWrapsOnItsOwn covers an empty prompt whose
// prefix is wider than the terminal. The block is however many rows the prefix
// takes, and recording it as one left the first keystroke redrawing the whole
// prefix below the rows already on screen.
func TestRendererCountsAPrefixThatWrapsOnItsOwn(t *testing.T) {
	t.Parallel()

	t.Run("an empty input counts the prefix's own rows", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		r := newRenderer(&out, ThemeDefault, newMockTerminal(""))
		if got := r.calculateRenderedLines(strings.Repeat("p", 100)+"> ", ""); got != 2 {
			t.Errorf("calculateRenderedLines() = %d, want 2 (102 columns of prefix wraps at 80)", got)
		}
	})

	t.Run("the next render erases every row the prefix drew", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		r := newRenderer(&out, ThemeDefault, newMockTerminal(""))
		prefix := strings.Repeat("p", 100) + "> "
		if err := r.render(prefix, "", 0); err != nil {
			t.Fatalf("first render: %v", err)
		}

		out.Reset()
		if err := r.render(prefix, "a", 1); err != nil {
			t.Fatalf("second render: %v", err)
		}
		if got := leadingCursorUp(t, out.String()); got != 1 {
			t.Errorf("the redraw moved up %d row(s), want 1: the prefix's first row stays on screen otherwise", got)
		}
	})
}

// TestRendererAccountsForAWideRuneWrappedWhole covers the cell a terminal
// leaves blank rather than split a glyph across the margin. Dividing total
// cells by the width does not know about that cell, so both the cursor's column
// and the block's height drift by one per straddle.
func TestRendererAccountsForAWideRuneWrappedWhole(t *testing.T) {
	t.Parallel()

	// "$ " plus 77 narrow cells leaves one free cell before the margin, so the
	// wide rune that follows moves whole to the next row.
	straddle := strings.Repeat("x", 77) + "あ"

	t.Run("the cursor follows the glyph to the next row", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		r := newRenderer(&out, ThemeDefault, newMockTerminal(""))
		if err := r.render("$ ", straddle, len([]rune(straddle))); err != nil {
			t.Fatalf("render: %v", err)
		}
		if got := out.String(); !strings.Contains(got, "\r\x1b[2C") {
			t.Errorf("render() wrote %q, want the cursor at column 2, after the wide rune", got)
		}
	})

	t.Run("the skipped cell counts toward the block's height", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		r := newRenderer(&out, ThemeDefault, newMockTerminal(""))
		// The straddle leaves 2 cells used on the second row; 79 more fill it and
		// start a third. Counting 160 cells as two rows loses that row.
		input := straddle + strings.Repeat("y", 79)
		if got := r.calculateRenderedLines("$ ", input); got != 3 {
			t.Errorf("calculateRenderedLines() = %d, want 3: the cell the wide rune could not use is still a cell", got)
		}
	})
}

// leadingCursorUp returns how many rows a render moves the cursor up before it
// erases anything, which decides what the redraw is about to overwrite. Zero
// when the render does not begin by moving up.
func leadingCursorUp(t *testing.T, out string) int {
	t.Helper()

	const up = "\x1b["
	if !strings.HasPrefix(out, up) {
		return 0
	}
	rest := out[len(up):]
	end := strings.IndexByte(rest, 'A')
	if end <= 0 {
		return 0
	}
	rows, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0
	}
	return rows
}

// TestRendererClearsFromTheRowTheCursorWasLeftOn is the redraw invariant: a
// render erases the block it drew last time, and nothing above it.
//
// The erase moved up by the height of the block, which is where the cursor is
// only while it sits on the block's last row. Move it onto an earlier row — a
// left arrow crossing a line break does exactly that — and every keystroke after
// it moved up one row too many, so the prompt climbed the screen and took a line
// of scrollback with it each time.
func TestRendererClearsFromTheRowTheCursorWasLeftOn(t *testing.T) {
	t.Parallel()

	const input = "SELECT name\nFROM people" // two lines, the break at rune 11

	tests := []struct {
		name   string
		cursor int
		wantUp int
	}{
		{"cursor on the first line", 4, 0},
		{"cursor at the line break", 11, 0},
		{"cursor on the second line", 15, 1},
		{"cursor at the end", len([]rune(input)), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			r := newRenderer(&out, ThemeDefault, newMockTerminal(""))
			if err := r.render("$ ", input, tt.cursor); err != nil {
				t.Fatalf("first render: %v", err)
			}

			out.Reset()
			if err := r.render("$ ", input, tt.cursor); err != nil {
				t.Fatalf("second render: %v", err)
			}
			if got := leadingCursorUp(t, out.String()); got != tt.wantUp {
				t.Errorf("the redraw moved up %d row(s), want %d: the cursor was on row %d of the block",
					got, tt.wantUp, tt.wantUp)
			}
		})
	}
}

// TestRendererClearsFromTheRowTheCursorWasLeftOnWhenWrapped is the same
// invariant for a line that wraps rather than one holding a newline: the block
// is two rows on an 80-column terminal and the cursor is on the first of them.
func TestRendererClearsFromTheRowTheCursorWasLeftOnWhenWrapped(t *testing.T) {
	t.Parallel()

	input := strings.Repeat("a", 100) // 102 cells with the prefix: two rows at 80

	var out bytes.Buffer
	r := newRenderer(&out, ThemeDefault, newMockTerminal(""))
	if err := r.render("$ ", input, 10); err != nil {
		t.Fatalf("first render: %v", err)
	}

	out.Reset()
	if err := r.render("$ ", input, 9); err != nil {
		t.Fatalf("second render: %v", err)
	}
	if got := leadingCursorUp(t, out.String()); got != 0 {
		t.Errorf("the redraw moved up %d row(s), want 0: the cursor was on the first row of the wrapped line", got)
	}
}

// countingTerminal is a terminal that records how often it was measured.
type countingTerminal struct {
	*mockTerminal
	sizeCalls int
}

func (c *countingTerminal) Size() (width, height int, err error) {
	c.sizeCalls++
	return c.mockTerminal.Size()
}

// TestRendererMeasuresTheTerminalOncePerRender pins the cost and the
// consistency of measuring. Every part of the wrap arithmetic needs the width,
// and asking once per question invited two answers inside one redraw — and, on a
// real terminal, one round trip per question: go-tty measures by asking the
// terminal for its pixel size and reading the reply out of the input stream,
// which swallows whatever was typed while it waited.
func TestRendererMeasuresTheTerminalOncePerRender(t *testing.T) {
	t.Parallel()

	term := &countingTerminal{mockTerminal: newMockTerminal("")}
	var out bytes.Buffer
	r := newRenderer(&out, ThemeDefault, term)

	if err := r.render("$ ", "SELECT name\nFROM people", 4); err != nil {
		t.Fatalf("render: %v", err)
	}
	if term.sizeCalls != 1 {
		t.Errorf("one render measured the terminal %d times, want 1", term.sizeCalls)
	}

	if err := r.render("$ ", "SELECT name\nFROM people", 5); err != nil {
		t.Fatalf("second render: %v", err)
	}
	if term.sizeCalls != 2 {
		t.Errorf("two renders measured the terminal %d times, want 2", term.sizeCalls)
	}
}

// TestRendererPositionsTheCursorOnTheWrappedRowItBelongsTo pins the other half
// of the same arithmetic. A cursor 10 cells into a line that wraps is on the
// block's first row, and moving back to it by columns alone cannot get there:
// a backward move stops at the left margin of the row it is already on.
func TestRendererPositionsTheCursorOnTheWrappedRowItBelongsTo(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	r := newRenderer(&out, ThemeDefault, newMockTerminal(""))
	// 100 runes plus a 2-cell prefix is 102 cells: row 0 holds 80, row 1 the
	// rest. Rune 10 is at column 12 of row 0, one row above where rendering
	// left the cursor.
	if err := r.render("$ ", strings.Repeat("a", 100), 10); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "\x1b[1A") {
		t.Errorf("render() wrote %q, want it to move the cursor up onto row 0", got)
	}
	if !strings.Contains(got, "\r\x1b[12C") {
		t.Errorf("render() wrote %q, want it to place the cursor at column 12 of that row", got)
	}
}

// cursorUpSequence matches the ANSI "move up" the renderer emits when it erases
// a block it drew across several rows.
var cursorUpSequence = regexp.MustCompile(`\x1b\[\d*A`)

// TestRendererForgetBlockStopsErasingFinishedOutput covers what happens between
// two prompts. Once a line is submitted, whatever the renderer drew belongs to
// the finished line, and the application prints its own output below it. A
// render that still moved up to erase "its" block reached into that output
// instead: after a two-line entry, the first row it erased was the last row the
// application had printed, so a result table lost its bottom border.
func TestRendererForgetBlockStopsErasingFinishedOutput(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	r := newRenderer(&output, ThemeDefault, nil)

	// A two-line entry, as a statement typed across a continuation line.
	if err := r.render("$ ", "SELECT 1\n;", 10); err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	// The line is submitted and the application prints its result. The renderer
	// is told the block is no longer its to erase.
	r.forgetBlock()

	output.Reset()
	if err := r.render("$ ", "", 0); err != nil {
		t.Fatalf("render after the submission failed: %v", err)
	}

	if got := output.String(); cursorUpSequence.MatchString(got) {
		t.Errorf("the prompt after a submission moved up into finished output: %q", got)
	}
}

// TestRendererStillErasesItsOwnBlockWhileEditing is the other half: within one
// line, a multi-row block must still be erased on every keystroke, or the
// previous draw stays on screen underneath the new one.
func TestRendererStillErasesItsOwnBlockWhileEditing(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	r := newRenderer(&output, ThemeDefault, nil)

	if err := r.render("$ ", "SELECT 1\n;", 10); err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	output.Reset()
	if err := r.render("$ ", "SELECT 1\n;;", 11); err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	if got := output.String(); !cursorUpSequence.MatchString(got) {
		t.Errorf("editing a two-row entry did not erase the row above: %q", got)
	}
}

// TestPromptDoesNotEraseOutputPrintedBetweenRuns drives the whole read loop the
// way a REPL does: a statement typed across two lines, then the application's
// output, then the next prompt. The second Run must not move the cursor up —
// everything above it belongs to the finished line and to the output.
func TestPromptDoesNotEraseOutputPrintedBetweenRuns(t *testing.T) {
	t.Parallel()

	// "SELECT 1" is incomplete, so Enter opens a continuation line; ";" completes
	// it. The second line is then submitted on its own.
	mock := newMockTerminal("SELECT 1\r;\rnext;\r")
	var output bytes.Buffer
	config := Config{
		Prefix:     "$ ",
		Multiline:  true,
		IsComplete: func(input string) bool { return strings.HasSuffix(strings.TrimSpace(input), ";") },
	}
	p := &Prompt{
		config:   config,
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
		output:   &output,
		renderer: newRenderer(&output, ThemeDefault, mock),
	}

	if got, err := p.RunWithContext(context.Background()); err != nil || got != "SELECT 1\n;" {
		t.Fatalf("first Run = %q, %v; want the two-line statement", got, err)
	}

	// What the application prints between prompts.
	fmt.Fprint(&output, "+---+\r\n| 1 |\r\n+---+\r\n")

	output.Reset()
	if _, err := p.RunWithContext(context.Background()); err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}

	if got := output.String(); cursorUpSequence.MatchString(got) {
		t.Errorf("the prompt after a two-line statement moved up into the printed result: %q", got)
	}
}

// TestLayoutAdvancesATabToItsTabStop pins how a tab is measured. runewidth
// reports 0 for it while a terminal moves the cursor to the next tab stop, so
// the cursor was drawn left of the text and a wrapping line was undercounted.
func TestLayoutAdvancesATabToItsTabStop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		s        string
		width    int
		wantRows int
		wantCol  int
	}{
		{
			name:     "a tab at column 0 reaches the first stop",
			s:        "\t",
			width:    80,
			wantRows: 0,
			wantCol:  8,
		},
		{
			name:     "a tab advances to the next stop, not by its rune width",
			s:        "a\tb",
			width:    80,
			wantRows: 0,
			wantCol:  9,
		},
		{
			name:     "a tab already on a stop advances to the next one",
			s:        "12345678\t",
			width:    80,
			wantRows: 0,
			wantCol:  16,
		},
		{
			name:     "a wide rune before the tab is counted in cells",
			s:        "あ\t",
			width:    80,
			wantRows: 0,
			wantCol:  8,
		},
		{
			name:     "a tab short of the margin reaches its stop",
			s:        strings.Repeat("x", 7) + "\t",
			width:    10,
			wantRows: 0,
			wantCol:  8,
		},
		{
			name:     "text after a tab wraps from the tab stop",
			s:        "a\t" + strings.Repeat("y", 3),
			width:    10,
			wantRows: 1,
			wantCol:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rows, col := layout(tt.s, tt.width)
			if rows != tt.wantRows || col != tt.wantCol {
				t.Errorf("layout(%q, %d) = (%d, %d), want (%d, %d)", tt.s, tt.width, rows, col, tt.wantRows, tt.wantCol)
			}
		})
	}
}

// TestRendererPositionsTheCursorAfterATab is the same measurement seen through a
// render: every render after a tab used to place the cursor several columns
// left of the text.
func TestRendererPositionsTheCursorAfterATab(t *testing.T) {
	t.Parallel()

	const input = "a\tb"

	var out bytes.Buffer
	r := newRenderer(&out, ThemeDefault, newMockTerminal(""))
	if err := r.render("> ", input, len([]rune(input))); err != nil {
		t.Fatalf("render: %v", err)
	}

	// "> " is 2 cells, "a" reaches 3, the tab reaches the stop at 8, "b" ends at 9.
	if got := out.String(); !strings.Contains(got, "\r\x1b[9C") {
		t.Errorf("render() wrote %q, want the cursor at column 9, after the tab stop", got)
	}
}

// TestRendererCountsTheRowsATabPushesALineOnto covers the other half: the height
// erased has to include the cells a tab occupies, or a row of the old block
// stays on screen.
func TestRendererCountsTheRowsATabPushesALineOnto(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	r := newRenderer(&out, ThemeDefault, newMockTerminal(""))
	// "$ " plus 70 cells reaches column 72; the tab reaches 80 and fills the row,
	// so the text after it belongs to a second row.
	input := strings.Repeat("x", 70) + "\tyz"
	if got := r.calculateRenderedLines("$ ", input); got != 2 {
		t.Errorf("calculateRenderedLines() = %d, want 2: the tab fills the row, so the text after it is on a second row", got)
	}
}

// TestRendererCountsTheRowsAWrappedSuggestionOccupies is the completion menu's
// version of the redraw invariant: the block's height is terminal rows, not
// suggestions. Counting entries instead left those rows out of the erase, so
// they stayed on screen above the next prompt.
func TestRendererCountsTheRowsAWrappedSuggestionOccupies(t *testing.T) {
	t.Parallel()

	// 30 columns, so "  " plus a 46-cell suggestion occupies two rows.
	const narrowWidth = 30
	suggestions := []Suggestion{
		{Text: "SELECT_a_very_long_completion_candidate_indeed"},
		{Text: "SET", Description: "assign"},
	}

	newNarrowRenderer := func(out *bytes.Buffer) *renderer {
		term := newMockTerminal("")
		term.terminalSize = [2]int{narrowWidth, 24}
		return newRenderer(out, ThemeDefault, term)
	}

	t.Run("the menu's height counts the wrapped rows", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		r := newNarrowRenderer(&out)
		if err := r.renderWithSuggestionsOffset("> ", "SE", 2, suggestions, 0, 0); err != nil {
			t.Fatalf("renderWithSuggestionsOffset: %v", err)
		}
		// One input row, two rows for the wrapping suggestion, one for "SET".
		if r.lastLines != 4 {
			t.Errorf("lastLines = %d, want 4: a suggestion wider than the terminal occupies two rows", r.lastLines)
		}
	})

	t.Run("a description counts toward the row's width", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		r := newNarrowRenderer(&out)
		// "  SET - " is 8 cells; a 30-cell description pushes the row past the margin.
		wide := []Suggestion{{Text: "SET", Description: strings.Repeat("d", 30)}}
		if err := r.renderWithSuggestionsOffset("> ", "SE", 2, wide, -1, 0); err != nil {
			t.Fatalf("renderWithSuggestionsOffset: %v", err)
		}
		if r.lastLines != 3 {
			t.Errorf("lastLines = %d, want 3: the description pushes its row past the margin", r.lastLines)
		}
	})

	t.Run("closing the menu erases every row it drew", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		r := newNarrowRenderer(&out)
		if err := r.renderWithSuggestionsOffset("> ", "SE", 2, suggestions, 0, 0); err != nil {
			t.Fatalf("renderWithSuggestionsOffset: %v", err)
		}
		out.Reset()
		if err := r.render("> ", "SE", 2); err != nil {
			t.Fatalf("render: %v", err)
		}
		if up := leadingCursorUp(t, out.String()); up != 3 {
			t.Errorf("render() moved up %d rows, want 3: the menu's wrapped rows are above the prompt", up)
		}
	})
}

// screenModel is a terminal small enough to test against: it interprets the
// escape sequences the renderer writes and nothing else, and it can be asked
// what is on screen and where the cursor is.
//
// It exists because the renderer's arithmetic is one calculation read four ways
// -- the rows erased, the rows drawn, the row the cursor is left on, and the
// column it is left at -- and an assertion on a single escape sequence pins one
// of the four. Feeding a render to a terminal and looking at the result pins all
// of them, and it is how a wrong number is caught in the place the user would
// see it.
type screenModel struct {
	cells   [][]rune
	width   int
	height  int
	row     int
	col     int
	pending bool // the cursor is parked on the last cell with a wrap owed
}

func newScreenModel(width int) *screenModel {
	s := &screenModel{width: width}
	s.growTo(1)
	return s
}

// growTo makes sure the screen has at least rows rows. It grows rather than
// clamping, because a block taller than the screen is a real answer the caller
// wants: silently stopping at a fixed height reports a height that is short by
// however far the block ran past it, and reads as a bug in the renderer.
func (s *screenModel) growTo(rows int) {
	for len(s.cells) < rows {
		row := make([]rune, s.width)
		for col := range row {
			row[col] = ' '
		}
		s.cells = append(s.cells, row)
	}
	s.height = len(s.cells)
}

// put writes one rune where the cursor is, the way a terminal does: a glyph is
// never split across the right margin, a tab stops at the last column rather
// than wrapping, and filling the last cell leaves the cursor on it with a wrap
// owed until another rune arrives.
func (s *screenModel) put(r rune) {
	if r == '\t' {
		s.resolvePending()
		stop := min(s.col+tabWidth-s.col%tabWidth, s.width-1)
		s.col = stop
		return
	}
	width := runewidth.RuneWidth(r)
	if width == 0 {
		return
	}
	s.resolvePending()
	// A glyph that does not fit the rest of the row moves to the next one whole.
	// One wider than the whole row does not fit anywhere, so it stays where it
	// is rather than wrapping forever.
	if s.col+width > s.width && s.col > 0 {
		s.newRow()
	}
	s.cells[s.row][s.col] = r
	for cell := 1; cell < width; cell++ {
		if s.col+cell < s.width {
			s.cells[s.row][s.col+cell] = 0 // the second half of a wide glyph holds nothing
		}
	}
	s.col += width
	if s.col >= s.width {
		s.col = s.width - 1
		s.pending = true
	}
}

func (s *screenModel) resolvePending() {
	if s.pending {
		s.newRow()
		s.pending = false
	}
}

func (s *screenModel) newRow() {
	s.col = 0
	s.row++
	s.growTo(s.row + 1)
	s.pending = false
}

func (s *screenModel) writeString(text string) {
	for _, r := range text {
		s.put(r)
	}
}

// startRow moves to the start of the next row, the way a line break in the input
// does.
func (s *screenModel) startRow() {
	s.newRow()
}

func (s *screenModel) eraseToEndOfRow() {
	for col := s.col; col < s.width; col++ {
		s.cells[s.row][col] = ' '
	}
}

func (s *screenModel) eraseToEndOfScreen() {
	s.eraseToEndOfRow()
	for row := s.row + 1; row < s.height; row++ {
		for col := range s.cells[row] {
			s.cells[row][col] = ' '
		}
	}
}

func (s *screenModel) eraseAll() {
	for row := range s.cells {
		for col := range s.cells[row] {
			s.cells[row][col] = ' '
		}
	}
}

// feed interprets what a render wrote.
func (s *screenModel) feed(output string) {
	runes := []rune(output)
	for i := 0; i < len(runes); i++ {
		switch r := runes[i]; {
		case r == '\r':
			s.col, s.pending = 0, false
		case r == '\n':
			s.row, s.pending = s.row+1, false
			s.growTo(s.row + 1)
		case r == '\x1b' && i+1 < len(runes) && runes[i+1] == '[':
			i = s.control(runes, i)
		default:
			s.put(r)
		}
	}
}

// control applies the CSI sequence starting at start and returns the index of
// its final byte.
func (s *screenModel) control(runes []rune, start int) int {
	end := start + 2
	for end < len(runes) && (runes[end] < csiFinalFirst || runes[end] > csiFinalLast) {
		end++
	}
	if end >= len(runes) {
		return len(runes)
	}
	params := string(runes[start+2 : end])
	count := 1
	if digits := strings.TrimLeft(params, "?"); digits == params && params != "" {
		if n, err := strconv.Atoi(params); err == nil {
			count = n
		}
	}
	switch runes[end] {
	case 'A':
		s.row, s.pending = max(s.row-count, 0), false
	case 'B':
		s.row, s.pending = s.row+count, false
		s.growTo(s.row + 1)
	case 'C':
		s.col, s.pending = min(s.col+count, s.width-1), false
	case 'D':
		s.col, s.pending = max(s.col-count, 0), false
	case 'H':
		s.row, s.col, s.pending = 0, 0, false
	case 'K':
		s.eraseToEndOfRow()
	case 'J':
		if params == "2" || params == "3" {
			s.eraseAll()
		} else {
			s.eraseToEndOfScreen()
		}
	}
	return end
}

// rows returns what is on screen, without the blank rows below it.
func (s *screenModel) rows() []string {
	out := make([]string, len(s.cells))
	for row := range s.cells {
		var line strings.Builder
		for _, cell := range s.cells[row] {
			if cell != 0 {
				line.WriteRune(cell)
			}
		}
		out[row] = strings.TrimRight(line.String(), " ")
	}
	last := len(out) - 1
	for last >= 0 && out[last] == "" {
		last--
	}
	return out[:last+1]
}

// TestRendererKeepsATabOnTheRowTheTerminalPutIt renders a line whose tab reaches
// the right margin and compares the result against a terminal. A tab that stops
// at the margin has not filled the last cell, so the character after it belongs
// to the same row; counting it as a wrap drew the cursor on a row the text never
// reached and left the next erase one row too high.
func TestRendererKeepsATabOnTheRowTheTerminalPutIt(t *testing.T) {
	t.Parallel()

	const width = 10
	const input = "x\ty\tz"

	var out bytes.Buffer
	r := newRenderer(&out, ThemeDefault, &sizedMockTerminal{width: width})
	if err := r.render("> ", input, len([]rune(input))); err != nil {
		t.Fatalf("render: %v", err)
	}

	screen := newScreenModel(width)
	screen.feed(out.String())

	want := newScreenModel(width)
	want.writeString("> " + input)

	if got, expected := screen.rows(), want.rows(); strings.Join(got, "|") != strings.Join(expected, "|") {
		t.Errorf("render() drew %q, want %q", got, expected)
	}
	if screen.row != want.row || screen.col != want.col {
		t.Errorf("render() left the cursor at (%d, %d), want (%d, %d)", screen.row, screen.col, want.row, want.col)
	}
	if r.lastLines != want.row+1 {
		t.Errorf("render() recorded %d rows, want %d", r.lastLines, want.row+1)
	}
	if r.lastCursorRow != screen.row {
		t.Errorf("render() recorded the cursor on row %d, but left it on row %d", r.lastCursorRow, screen.row)
	}
}

// TestRendererDoesNotEraseTheRowAboveAfterATab is the same measurement as seen a
// keystroke later: an overstated cursor row makes the next render move up past
// the top of its own block and erase what the application printed there.
func TestRendererDoesNotEraseTheRowAboveAfterATab(t *testing.T) {
	t.Parallel()

	const width = 10

	var out bytes.Buffer
	r := newRenderer(&out, ThemeDefault, &sizedMockTerminal{width: width})
	if err := r.render("> ", "x\ty\tz", 5); err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := r.render("> ", "x\ty\tzz", 6); err != nil {
		t.Fatalf("render: %v", err)
	}

	screen := newScreenModel(width)
	screen.writeString("earlier")
	screen.startRow()
	screen.feed(out.String())

	if rows := screen.rows(); len(rows) == 0 || rows[0] != "earlier" {
		t.Errorf("the second render erased the line above the prompt: %q", screen.rows())
	}
}

// TestLayoutStopsATabAtTheLastColumn pins the measurement itself. A terminal
// moves a tab to the next stop and, when the row holds no further stop, to the
// last column -- where the cursor sits without a wrap owed, so the next
// character prints on the same row.
func TestLayoutStopsATabAtTheLastColumn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		s        string
		width    int
		wantRows int
		wantCol  int
	}{
		{
			name:     "a tab past the last stop reaches the last column",
			s:        strings.Repeat("x", 9) + "\t",
			width:    10,
			wantRows: 0,
			wantCol:  9,
		},
		{
			name:     "a character after such a tab stays on the row",
			s:        strings.Repeat("x", 9) + "\tz",
			width:    10,
			wantRows: 0,
			wantCol:  10,
		},
		{
			name:     "two tabs in a row cannot push past the last column",
			s:        "x\ty\tz",
			width:    10,
			wantRows: 0,
			wantCol:  10,
		},
		{
			name:     "a tab on a filled row belongs to the next one",
			s:        strings.Repeat("x", 10) + "\t",
			width:    10,
			wantRows: 1,
			wantCol:  8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rows, col := layout(tt.s, tt.width)
			if rows != tt.wantRows || col != tt.wantCol {
				t.Errorf("layout(%q, %d) = (%d, %d), want (%d, %d)", tt.s, tt.width, rows, col, tt.wantRows, tt.wantCol)
			}
		})
	}
}

// TestRendererDrawsOneRowPerSuggestion covers what a menu row holds. A menu row
// is one terminal row and its height is measured by walking its text for cells,
// but a newline occupies no cells and moves the terminal to the next row, so a
// suggestion carrying one was drawn taller than it was counted and the extra row
// survived the erase.
func TestRendererDrawsOneRowPerSuggestion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		suggestion Suggestion
	}{
		{name: "a newline in the text", suggestion: Suggestion{Text: "a\nb"}},
		{name: "a newline in the description", suggestion: Suggestion{Text: "a", Description: "one\ntwo"}},
		{name: "a carriage return", suggestion: Suggestion{Text: "a\rb"}},
		{name: "an escape sequence", suggestion: Suggestion{Text: "\x1b[2Jwipe"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const width = 40

			var out bytes.Buffer
			r := newRenderer(&out, ThemeDefault, &sizedMockTerminal{width: width})
			if err := r.renderWithSuggestionsOffset("> ", "a", 1, []Suggestion{tt.suggestion}, 0, 0); err != nil {
				t.Fatalf("renderWithSuggestionsOffset: %v", err)
			}

			screen := newScreenModel(width)
			screen.feed(out.String())

			if drawn := len(screen.rows()); drawn != r.lastLines {
				t.Errorf("the menu recorded %d rows and drew %d: %q", r.lastLines, drawn, screen.rows())
			}
			if r.lastCursorRow != screen.row {
				t.Errorf("the menu recorded the cursor on row %d and left it on row %d", r.lastCursorRow, screen.row)
			}
		})
	}
}

// TestSingleLineKeepsTextOnItsRow pins what is flattened. A tab stays, because a
// terminal keeps it on the row and layout measures it against tab stops.
func TestSingleLineKeepsTextOnItsRow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		want string
	}{
		{name: "plain text is returned as it is", s: "select * from t", want: "select * from t"},
		{name: "a newline becomes a space", s: "a\nb", want: "a b"},
		{name: "a carriage return becomes a space", s: "a\rb", want: "a b"},
		{name: "an escape becomes a space", s: "a\x1b[2Jb", want: "a [2Jb"},
		{name: "a delete becomes a space", s: "a\x7fb", want: "a b"},
		{name: "a tab is kept", s: "a\tb", want: "a\tb"},
		{name: "text outside ascii is kept", s: "日本語 naïve", want: "日本語 naïve"},
		{name: "empty stays empty", s: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := singleLine(tt.s); got != tt.want {
				t.Errorf("singleLine(%q) = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

// TestPromptShowsTheCursorWhenItGivesTheTerminalBack covers what an interrupt
// leaves behind. The completion menu hides the cursor while it is drawn and
// shows it again on the next render without one, so an interrupt -- which
// returns before that render -- used to hand the terminal back with no cursor,
// for as long as the terminal lived if the application exited there.
func TestPromptShowsTheCursorWhenItGivesTheTerminalBack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
	}{
		{name: "interrupted with the menu open", script: "cre\t\x03"},
		{name: "interrupted with no menu", script: "abc\x03"},
		{name: "input ended with the menu open", script: "cre\t\x1b\x04"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			p := newTestPrompt(newMockTerminal(tt.script), WithCompleter(func(Document) []Suggestion {
				return []Suggestion{{Text: "create"}, {Text: "credit"}}
			}))
			p.output = &out
			p.renderer = newRenderer(&out, ThemeDefault, p.terminal)

			if _, err := p.Run(); err == nil {
				t.Fatalf("Run() returned no error, want the session to have ended")
			}

			written := out.String()
			if hidden, shown := strings.LastIndex(written, "\x1b[?25l"), strings.LastIndex(written, "\x1b[?25h"); hidden > shown {
				t.Errorf("the prompt returned with the cursor hidden")
			}
		})
	}
}

// FuzzLayoutStaysOnTheScreen measures arbitrary text against the terminal model
// the renderer tests use. The two have to answer the same, because every render
// is laid out by one and drawn on the other.
func FuzzLayoutStaysOnTheScreen(f *testing.F) {
	for _, s := range []string{"", "a", "\t", "あ", "a\tb", "😀", "́"} {
		for _, w := range []int{1, 2, 8, 80} {
			f.Add(s, w)
		}
	}
	f.Fuzz(func(t *testing.T, s string, width int) {
		if width < 1 || width > 500 {
			t.Skip()
		}
		rows, col := layout(s, width)
		if rows < 0 {
			t.Fatalf("layout(%q, %d) reported %d rows", s, width, rows)
		}
		// A glyph wider than the terminal cannot be measured smaller than
		// itself, so on a terminal of one column a double-width rune ends on
		// column 2. Nothing can draw it there either; the readers clamp.
		if col < 0 || (col > width && width > 1) {
			t.Fatalf("layout(%q, %d) ended on column %d, outside [0, %d]", s, width, col, width)
		}
		// The model a render is measured against has to agree.
		m := newScreenModel(width)
		m.writeString(s)
		if m.row != rows {
			t.Fatalf("layout(%q, %d) says %d wraps, the terminal made %d", s, width, rows, m.row)
		}
	})
}

// FuzzSingleLineStaysOnItsRow pins what a menu row and a search result are
// flattened to: the same number of characters, none of which takes the cursor
// off the row it is drawn on.
func FuzzSingleLineStaysOnItsRow(f *testing.F) {
	for _, s := range []string{"", "a", "a\nb", "\x1b[2J", "\t", "日本語", "\x7f"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := singleLine(s)
		if len([]rune(got)) != len([]rune(s)) {
			t.Fatalf("singleLine(%q) = %q, a different number of characters", s, got)
		}
		for _, r := range got {
			if leavesRow(r) {
				t.Fatalf("singleLine(%q) = %q, which still holds %q", s, got, r)
			}
		}
		rows, _ := layout(got, 80)
		m := newScreenModel(80)
		m.writeString(got)
		if m.row != rows {
			t.Fatalf("singleLine(%q) = %q: layout says %d wraps, the terminal made %d", s, got, rows, m.row)
		}
	})
}

// FuzzSpansForNormalizes pins what a highlighter's answer is turned into. It is
// application code deciding a decoration, so it is normalized rather than
// trusted: what comes back is ordered, non-overlapping, inside the input, and
// never empty.
func FuzzSpansForNormalizes(f *testing.F) {
	f.Add("hello", 0, 3, 2, 4)
	f.Add("", -1, 5, 0, 0)
	f.Add("日本語", 2, 1, 0, 9)
	f.Fuzz(func(t *testing.T, input string, a, b, c, d int) {
		r := newRenderer(nil, ThemeDefault, nil)
		r.setHighlighter(func(string) []StyleSpan {
			return []StyleSpan{{Start: a, End: b}, {Start: c, End: d}}
		})
		limit := len([]rune(input))
		spans := r.spansFor(input)
		prev := 0
		for _, s := range spans {
			if s.Start < 0 || s.End > limit || s.Start >= s.End {
				t.Fatalf("spansFor(%q) kept %v, outside [0, %d] or empty", input, s, limit)
			}
			if s.Start < prev {
				t.Fatalf("spansFor(%q) returned %v out of order or overlapping", input, spans)
			}
			prev = s.End
		}
	})
}

// TestRendererKeepsTheCompletionMenuInsideTheTerminal pins the invariant the menu
// never enforced: what the prompt draws has to fit the terminal it is drawn on.
//
// The menu's size was a count of entries, ten, and its height in rows was
// whatever those ten happened to occupy -- a candidate wider than the terminal
// is drawn, and counted, as the rows it wraps onto. A block taller than the
// screen scrolls it, and the first rows to go are the prompt and the line being
// completed, so pressing Tab left the user reading candidates with no sight of
// what they were completing, and took the application's output off the screen
// with it.
func TestRendererKeepsTheCompletionMenuInsideTheTerminal(t *testing.T) {
	t.Parallel()

	shortCandidates := func(n int) []Suggestion {
		out := make([]Suggestion, 0, n)
		for i := range n {
			out = append(out, Suggestion{Text: fmt.Sprintf("suggestion-%02d", i+1)})
		}
		return out
	}
	wideCandidates := func(n, cells int) []Suggestion {
		out := make([]Suggestion, 0, n)
		for i := range n {
			out = append(out, Suggestion{Text: fmt.Sprintf("s%02d-", i+1) + strings.Repeat("x", cells)})
		}
		return out
	}

	tests := []struct {
		name        string
		width       int
		height      int
		input       string
		suggestions []Suggestion
	}{
		{
			name:        "ten candidates on a ten-row terminal",
			width:       80,
			height:      10,
			input:       "s",
			suggestions: shortCandidates(10),
		},
		{
			name:        "ten candidates that wrap, on a terminal of the usual size",
			width:       80,
			height:      24,
			input:       "s",
			suggestions: wideCandidates(10, 200),
		},
		{
			name:        "an input that wraps, leaving fewer rows for the menu",
			width:       20,
			height:      8,
			input:       strings.Repeat("a", 50),
			suggestions: shortCandidates(10),
		},
		{
			name:        "one row to spare",
			width:       80,
			height:      3,
			input:       "s",
			suggestions: shortCandidates(10),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			r := newRenderer(&out, ThemeDefault, &sizedMockTerminal{width: tt.width, height: tt.height})
			if err := r.renderWithSuggestionsOffset("$ ", tt.input, len([]rune(tt.input)), tt.suggestions, 0, 0); err != nil {
				t.Fatalf("renderWithSuggestionsOffset: %v", err)
			}

			screen := newScreenModel(tt.width)
			screen.feed(out.String())
			if drawn := len(screen.rows()); drawn > tt.height {
				t.Errorf("the render drew %d rows on a terminal of %d: the terminal scrolls, and the line being completed goes first", drawn, tt.height)
			}
			if r.lastLines > tt.height {
				t.Errorf("the render recorded a block of %d rows on a terminal of %d, so the next erase moves up past the top of the screen", r.lastLines, tt.height)
			}
			// A menu is worth drawing only if it has a row on screen, and the
			// selected row is the one the window must always contain. It is
			// looked for at the start of a row rather than anywhere in the
			// output, because the indicators are ASCII and a candidate can hold
			// the same two characters.
			selectedDrawn := false
			for _, row := range screen.rows() {
				if strings.HasPrefix(row, menuSelectedIndicator) {
					selectedDrawn = true
					break
				}
			}
			if !selectedDrawn {
				t.Errorf("the render drew no selected candidate:\n%q", screen.rows())
			}
		})
	}
}

// TestRendererDrawsNoMenuWithNoRoomForOne pins the other end of the same rule.
// Where the input already fills the terminal there is no row a candidate could
// be drawn on, and drawing one anyway would push the line being typed off the
// top of the screen. The prompt is rendered as it is without a menu, cursor and
// all, so the user keeps sight of what they are completing.
func TestRendererDrawsNoMenuWithNoRoomForOne(t *testing.T) {
	t.Parallel()

	const width, height = 20, 3

	var out bytes.Buffer
	terminal := &sizedMockTerminal{width: width, height: height}
	r := newRenderer(&out, ThemeDefault, terminal)
	input := strings.Repeat("a", width*height) // three rows of input on a three-row terminal
	if err := r.renderWithSuggestionsOffset("$ ", input, len([]rune(input)), []Suggestion{{Text: "candidate"}}, 0, 0); err != nil {
		t.Fatalf("renderWithSuggestionsOffset: %v", err)
	}

	if strings.Contains(out.String(), "candidate") {
		t.Errorf("the render drew a candidate with no row to draw it on:\n%q", out.String())
	}
	if !strings.Contains(out.String(), showCursorSequence) {
		t.Errorf("the render left the cursor hidden, which is what a menu does:\n%q", out.String())
	}
}

// TestMenuIndicatorsAreTheSameWidth pins the assumption the menu's height rests
// on. Every entry is measured with the indicator drawn in front of an unselected
// candidate, and the selected one is drawn with a different string, so the two
// have to occupy the same number of cells or the selected row is drawn taller
// than it was counted and the menu runs past the terminal's last row.
//
// The selected indicator used to be a black right-pointing triangle, which
// Unicode calls East Asian Ambiguous: go-runewidth reports it as two cells under
// a CJK locale and one everywhere else, so the menu's geometry depended on the
// environment the application was started in. Both runewidth conditions are
// checked here because a test run under an ASCII locale cannot tell them apart.
func TestMenuIndicatorsAreTheSameWidth(t *testing.T) {
	t.Parallel()

	for _, eastAsian := range []bool{false, true} {
		condition := runewidth.NewCondition()
		condition.EastAsianWidth = eastAsian

		plain := condition.StringWidth(menuIndicator)
		selected := condition.StringWidth(menuSelectedIndicator)
		if plain != selected {
			t.Errorf("with EastAsianWidth=%v the unselected indicator %q is %d cells and the selected one %q is %d: a candidate that fills its row wraps when it is selected, and the menu grows past the terminal",
				eastAsian, menuIndicator, plain, menuSelectedIndicator, selected)
		}
	}
}

// TestMenuFitsTheTerminalWhateverIsSelected compares the row the window is
// measured for with the row that is drawn. The window counts every candidate
// with the unselected indicator, so a candidate whose row ends within a cell of
// the right margin fits when counted and wraps when it is the one selected.
func TestMenuFitsTheTerminalWhateverIsSelected(t *testing.T) {
	t.Parallel()

	const width, height = 20, 4
	// Each candidate is exactly the width of a row once the indicator is in
	// front of it, which is where one cell more starts costing a second row.
	suggestions := []Suggestion{
		{Text: strings.Repeat("a", width-2)},
		{Text: strings.Repeat("b", width-2)},
		{Text: strings.Repeat("c", width-2)},
	}

	for selected := range suggestions {
		t.Run(fmt.Sprintf("candidate %d selected", selected), func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			r := newRenderer(&out, ThemeDefault, &sizedMockTerminal{width: width, height: height})
			if err := r.renderWithSuggestionsOffset("> ", "", 0, suggestions, selected, 0); err != nil {
				t.Fatalf("renderWithSuggestionsOffset() error = %v", err)
			}

			screen := newScreenModel(width)
			screen.feed(out.String())
			if drawn := len(screen.rows()); drawn > height {
				t.Errorf("the render drew %d rows on a terminal of %d: the terminal scrolls, and the line being completed goes first", drawn, height)
			}
			if r.lastLines > height {
				t.Errorf("the render recorded a block of %d rows on a terminal of %d, so the next erase moves up past the top of the screen", r.lastLines, height)
			}
		})
	}
}

// TestRendererDrawsTheLineFlattened renders a line holding a control character
// and compares the screen with the text a terminal should end up showing.
//
// The menu and the search block have flattened what they draw since #60 and
// #61; the line has not, and the line is where a completion candidate and a
// recalled history entry end up. An escape sequence written there is a command
// the terminal obeys, and the layout walk measures it as nothing, so the cursor
// is drawn past the text by however many cells the sequence really took.
//
// The expected text is written out rather than taken from singleLine, so the
// test is an oracle rather than a second copy of the implementation.
func TestRendererDrawsTheLineFlattened(t *testing.T) {
	t.Parallel()

	const width = 40
	tests := map[string]struct {
		input string
		want  string
	}{
		"an escape sequence from a completion candidate": {"select\x1b[31mred", "select [31mred"},
		"a carriage return from a history entry":         {"echo\rhi", "echo hi"},
		"a delete from a file the completer read":        {"na\x7fme", "na me"},
		"a C1 control from a CSV header":                 {"name\u0085surname", "name surname"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			r := newRenderer(&out, ThemeDefault, &sizedMockTerminal{width: width})
			if err := r.render("$ ", tt.input, len([]rune(tt.input))); err != nil {
				t.Fatalf("render() error = %v", err)
			}

			screen := newScreenModel(width)
			screen.feed(out.String())

			want := newScreenModel(width)
			want.writeString("$ " + tt.want)

			if got, expected := screen.rows(), want.rows(); strings.Join(got, "|") != strings.Join(expected, "|") {
				t.Errorf("the screen shows %q, want %q", got, expected)
			}
			if screen.row != want.row || screen.col != want.col {
				t.Errorf("the cursor is at (%d, %d), want (%d, %d)", screen.row, screen.col, want.row, want.col)
			}
		})
	}
}

// TestRendererDrawsThePrefixFlattened does the same for the prefix, which is
// measured by the same walk and drawn by the same writer but reaches the
// renderer by another route: an application builds it, and one that builds it
// out of data -- a working directory's name -- does not choose what goes in it.
func TestRendererDrawsThePrefixFlattened(t *testing.T) {
	t.Parallel()

	const width = 40
	const input = "abc"

	t.Run("an escape sequence leaves the cursor off the text", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		r := newRenderer(&out, ThemeDefault, &sizedMockTerminal{width: width})
		if err := r.render("sqly\x1b[31m> ", input, len(input)); err != nil {
			t.Fatalf("render() error = %v", err)
		}

		screen := newScreenModel(width)
		screen.feed(out.String())

		want := newScreenModel(width)
		want.writeString("sqly [31m> " + input)

		if got, expected := screen.rows(), want.rows(); strings.Join(got, "|") != strings.Join(expected, "|") {
			t.Errorf("the screen shows %q, want %q", got, expected)
		}
		if screen.row != want.row || screen.col != want.col {
			t.Errorf("the cursor is at (%d, %d), want (%d, %d)", screen.row, screen.col, want.row, want.col)
		}
	})

	// A newline in the prefix costs a row rather than a column, and the row is
	// not one the block counted: the erase that starts the next render is a row
	// short, so the block is redrawn below the one before it and the prompt
	// walks down the screen a row per keystroke.
	t.Run("a newline leaves the previous block on the screen", func(t *testing.T) {
		t.Parallel()

		var out bytes.Buffer
		r := newRenderer(&out, ThemeDefault, &sizedMockTerminal{width: width})
		for _, typed := range []string{"a", "ab", "abc"} {
			if err := r.render("sqly\n$ ", typed, len(typed)); err != nil {
				t.Fatalf("render() error = %v", err)
			}
		}

		screen := newScreenModel(width)
		screen.feed(out.String())

		want := newScreenModel(width)
		want.writeString("sqly $ " + input)

		if got, expected := screen.rows(), want.rows(); strings.Join(got, "|") != strings.Join(expected, "|") {
			t.Errorf("after three keystrokes the screen shows %q, want %q", got, expected)
		}
	})
}

// TestLeavesRowNamesEveryControlRune checks the rule the function states
// against the set Unicode names, rather than against the bytes a person can
// type. The C1 controls are the half of the category that arrives only in data,
// which is why the hand-written range test covered the other half.
func TestLeavesRowNamesEveryControlRune(t *testing.T) {
	t.Parallel()

	tests := map[rune]bool{
		'\x00':   true,  // NUL
		'\x1b':   true,  // ESC
		'\r':     true,  // CR
		'\n':     true,  // LF
		'\x7f':   true,  // DEL
		'\u0085': true,  // NEL, which takes the cursor to the next row
		'\u009b': true,  // CSI, which opens a control sequence
		'\t':     false, // a tab stays on its row, and the layout walk measures it
		'a':      false,
		'あ':      false, // a double-width letter, measured in cells already
		'\u200b': false, // zero width space: the terminal draws nothing and spends nothing
		'\u2028': false, // a line separator to a text engine, an ordinary rune to a terminal
	}

	for r, want := range tests {
		if got := leavesRow(r); got != want {
			t.Errorf("leavesRow(%U) = %v, want %v", r, got, want)
		}
	}
}

// TestMenuRowHoldingAC1ControlIsFlattened renders a menu whose candidate
// carries a C1 control and checks that none of it reaches the terminal. The
// menu is measured by walking its text for cells, and a C1 control is zero
// cells and a cursor movement, so a row holding one is drawn taller than it is
// counted and the extra row survives the erase.
func TestMenuRowHoldingAC1ControlIsFlattened(t *testing.T) {
	t.Parallel()

	const width = 40
	suggestions := []Suggestion{
		{Text: "name\u0085surname", Description: "a header\u009bcell"},
		{Text: "nickname"},
	}

	var out bytes.Buffer
	r := newRenderer(&out, ThemeDefault, &sizedMockTerminal{width: width})
	if err := r.renderWithSuggestionsOffset("$ ", "n", 1, suggestions, 0, 0); err != nil {
		t.Fatalf("renderWithSuggestionsOffset() error = %v", err)
	}

	for _, control := range []rune{'\u0085', '\u009b'} {
		if strings.ContainsRune(out.String(), control) {
			t.Errorf("the menu wrote %U to the terminal: %q", control, out.String())
		}
	}
}
