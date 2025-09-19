package prompt

import (
	"fmt"
	"io"
	"strings"
)

// renderer handles the display of the prompt and suggestions with advanced terminal control.
//
// The renderer manages all visual aspects of the prompt including:
//   - Multi-line input rendering with proper cursor positioning
//   - Color-coded output using ANSI escape sequences
//   - Completion suggestion display with selection highlighting
//   - Efficient screen updates by tracking and clearing previous output
//   - Cross-platform terminal control for consistent appearance
//
// Key features addressing original go-prompt issues:
//   - Safe cursor positioning to prevent divide-by-zero panics (issue #277)
//   - Proper line tracking for clean screen updates
//   - Unicode-aware text handling for international characters
//   - Efficient rendering that minimizes terminal flicker
//
// The renderer coordinates with the color scheme system to provide themed
// visual output and handles complex scenarios like suggestion menus and
// multi-line editing with proper text wrapping.
type renderer struct {
	output            io.Writer         // Target output writer (typically stdout or colorable wrapper)
	colorScheme       *ColorScheme      // Color configuration for themed rendering
	lastLines         int               // Track number of lines rendered for efficient cleanup
	suggestionsActive bool              // Track if suggestions are currently displayed
	terminal          terminalInterface // Terminal interface for getting size information
}

// newRenderer creates a new renderer with the given output and color scheme.
func newRenderer(output io.Writer, colorScheme *ColorScheme, terminal terminalInterface) *renderer {
	return &renderer{
		output:            output,
		colorScheme:       colorScheme,
		lastLines:         1, // Initialize with 1 to handle initial clear correctly
		suggestionsActive: false,
		terminal:          terminal,
	}
}

// render displays the prompt with the current input.
func (r *renderer) render(prefix, input string, cursor int) error {
	return r.renderWithSuggestionsOffset(prefix, input, cursor, nil, 0, 0)
}

// renderWithSuggestionsOffset displays the prompt with completion suggestions and scrolling support.
func (r *renderer) renderWithSuggestionsOffset(prefix, input string, cursor int, suggestions []Suggestion, selected int, offset int) error {
	// Clear previous output using the CURRENT lastLines value
	r.clearPreviousLines()

	// Calculate the actual number of lines that will be rendered
	// This accounts for both explicit newlines and terminal wrapping
	inputLines := r.calculateRenderedLines(prefix, input)
	if inputLines == 0 {
		inputLines = 1
	}

	if len(suggestions) > 0 {
		// Hide cursor during suggestion rendering
		if _, err := fmt.Fprint(r.output, "\x1b[?25l"); err != nil {
			return err
		}

		// Render the main prompt line without cursor
		if err := r.renderMainLineWithoutCursor(prefix, input); err != nil {
			return err
		}

		// Render suggestions
		if err := r.renderSuggestionsWithOffset(prefix, input, cursor, suggestions, selected, offset); err != nil {
			return err
		}

		// Update state AFTER rendering
		visibleCount := min(len(suggestions), 10)
		r.lastLines = inputLines + visibleCount
		r.suggestionsActive = true
	} else {
		// No suggestions - render normally with cursor
		if err := r.renderMainLine(prefix, input, cursor); err != nil {
			return err
		}

		// Show cursor
		if _, err := fmt.Fprint(r.output, "\x1b[?25h"); err != nil {
			return err
		}

		// Update lastLines to match the actual number of lines rendered
		r.lastLines = inputLines
		r.suggestionsActive = false
	}

	return nil
}

// renderMainLine renders the main prompt line with prefix and input.
func (r *renderer) renderMainLine(prefix, input string, cursor int) error {
	// Move to beginning of line and clear it
	if _, err := fmt.Fprint(r.output, "\r\x1b[K"); err != nil {
		return err
	}

	// Split input into lines
	lines := r.splitIntoLines(input)
	inputRunes := []rune(input)

	// Calculate cursor position in terms of line and column
	cursorLine, cursorCol := r.findCursorPosition(inputRunes, cursor)

	// Render each line
	for lineIndex, line := range lines {
		// Clear current line
		if lineIndex > 0 {
			if _, err := fmt.Fprint(r.output, "\x1b[K"); err != nil {
				return err
			}
		}

		if lineIndex == 0 {
			// First line: render prefix
			if _, err := fmt.Fprint(r.output, r.colorScheme.Prefix.ToANSI()); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, prefix); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, Reset()); err != nil {
				return err
			}
		} else {
			// Continuation lines: add appropriate indentation
			if _, err := fmt.Fprint(r.output, strings.Repeat(" ", len([]rune(prefix)))); err != nil {
				return err
			}
		}

		// Render line content with color
		if _, err := fmt.Fprint(r.output, r.colorScheme.Input.ToANSI()); err != nil {
			return err
		}
		if _, err := fmt.Fprint(r.output, line); err != nil {
			return err
		}
		if _, err := fmt.Fprint(r.output, Reset()); err != nil {
			return err
		}

		// Move to next line if not the last line
		if lineIndex < len(lines)-1 {
			if _, err := fmt.Fprint(r.output, "\n"); err != nil {
				return err
			}
		}
	}

	// Position cursor correctly
	r.positionCursor(lines, cursorLine, cursorCol, len([]rune(prefix)))

	return nil
}

// renderMainLineWithoutCursor renders the main prompt line without cursor positioning (for suggestions)
func (r *renderer) renderMainLineWithoutCursor(prefix, input string) error {
	// Move to beginning of line and clear it
	if _, err := fmt.Fprint(r.output, "\r\x1b[K"); err != nil {
		return err
	}

	// Split input into lines
	lines := r.splitIntoLines(input)

	// Render each line
	for lineIndex, line := range lines {
		// Clear current line
		if lineIndex > 0 {
			if _, err := fmt.Fprint(r.output, "\x1b[K"); err != nil {
				return err
			}
		}

		if lineIndex == 0 {
			// First line: render prefix
			if _, err := fmt.Fprint(r.output, r.colorScheme.Prefix.ToANSI()); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, prefix); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, Reset()); err != nil {
				return err
			}
		} else {
			// Continuation lines: add appropriate indentation
			if _, err := fmt.Fprint(r.output, strings.Repeat(" ", len([]rune(prefix)))); err != nil {
				return err
			}
		}

		// Render line content with color
		if _, err := fmt.Fprint(r.output, r.colorScheme.Input.ToANSI()); err != nil {
			return err
		}
		if _, err := fmt.Fprint(r.output, line); err != nil {
			return err
		}
		if _, err := fmt.Fprint(r.output, Reset()); err != nil {
			return err
		}

		// Move to next line if not the last line
		if lineIndex < len(lines)-1 {
			if _, err := fmt.Fprint(r.output, "\n"); err != nil {
				return err
			}
		}
	}

	return nil
}

// renderSuggestionsWithOffset renders the completion suggestions with scrolling support.
func (r *renderer) renderSuggestionsWithOffset(_, _ string, _ int, suggestions []Suggestion, selected int, offset int) error {
	// Start rendering suggestions
	if _, err := fmt.Fprint(r.output, "\r\n"); err != nil {
		return err
	}

	maxSuggestions := 10 // Limit number of displayed suggestions

	// Clamp offset to valid range for all suggestion counts
	maxOffset := max(0, len(suggestions)-maxSuggestions)
	offset = max(0, min(offset, maxOffset))

	// Calculate visible range with offset
	visibleSuggestions := suggestions
	if len(suggestions) > maxSuggestions {
		visibleSuggestions = suggestions[offset:min(offset+maxSuggestions, len(suggestions))]
	}

	// Adjust selected index for visible range
	visibleSelected := selected - offset
	if selected < offset || selected >= offset+len(visibleSuggestions) {
		visibleSelected = -1 // Selected item is not visible
	}

	for i, suggestion := range visibleSuggestions {
		// Clear line and move to beginning
		if _, err := fmt.Fprint(r.output, "\r\x1b[K"); err != nil {
			return err
		}

		// Render selection indicator and suggestion
		if i == visibleSelected {
			// Selected suggestion
			if _, err := fmt.Fprint(r.output, r.colorScheme.Selected.ToANSI()); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, "▶ "); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, suggestion.Text); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, Reset()); err != nil {
				return err
			}
		} else {
			// Normal suggestion
			if _, err := fmt.Fprint(r.output, r.colorScheme.Suggestion.Text.ToANSI()); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, "  "); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, suggestion.Text); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, Reset()); err != nil {
				return err
			}
		}

		// Render description if available
		if suggestion.Description != "" {
			if _, err := fmt.Fprint(r.output, " "); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, r.colorScheme.Suggestion.Description.ToANSI()); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, "- "); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, suggestion.Description); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, Reset()); err != nil {
				return err
			}
		}

		// Move to next line (except for last suggestion) with proper line ending
		if i < len(visibleSuggestions)-1 {
			if _, err := fmt.Fprint(r.output, "\r\n"); err != nil {
				return err
			}
		}
	}

	// Leave cursor at the end of suggestions
	// Parent function will handle final cursor positioning
	return nil
}

// clearPreviousLines clears the previously rendered lines.
func (r *renderer) clearPreviousLines() {
	if r.lastLines <= 1 {
		// Just clear the current line
		fmt.Fprint(r.output, "\r\x1b[K")
		return
	}

	// For multi-line content, we need to:
	// 1. Move cursor up to the beginning of the rendered content
	// 2. Clear from cursor position to end of screen
	// This ensures all previously rendered lines are cleared properly

	// Move cursor up to the first line of the previously rendered content
	fmt.Fprintf(r.output, "\x1b[%dA", r.lastLines-1)

	// Move to beginning of line and clear from cursor to end of screen
	// \x1b[0J clears from cursor position to end of screen
	fmt.Fprint(r.output, "\r\x1b[0J")
}

// splitIntoLines splits the input string into individual lines for multi-line rendering.
//
// This function properly handles various line ending scenarios:
//   - Empty input returns a single empty line for consistent rendering
//   - Single line input without newlines returns one line
//   - Multi-line input with \n separators returns properly split lines
//   - Preserves empty lines within the input for accurate display
//
// Used internally for calculating cursor positions and rendering multi-line prompts.
func (r *renderer) splitIntoLines(input string) []string {
	if input == "" {
		return []string{""}
	}
	lines := strings.Split(input, "\n")
	return lines
}

// findCursorPosition calculates which line and column the cursor is at within multi-line input.
//
// This algorithm handles cursor positioning for complex multi-line scenarios:
//   - Counts newline characters to determine the current line number
//   - Calculates column position relative to the start of the current line
//   - Handles edge cases like cursor at start (0,0) or end of input
//   - Provides safe bounds checking to prevent index out of range errors
//
// Returns (line, col) where both are 0-indexed. Used for proper cursor
// positioning in terminal output and multi-line editing operations.
//
// Critical for preventing cursor positioning bugs that caused crashes
// in the original go-prompt implementation.
func (r *renderer) findCursorPosition(inputRunes []rune, cursor int) (line, col int) {
	if cursor <= 0 {
		return 0, 0
	}
	if cursor >= len(inputRunes) {
		// Find the last line
		lineCount := 0
		lastLineStart := 0
		for i, r := range inputRunes {
			if r == '\n' {
				lineCount++
				lastLineStart = i + 1
			}
		}
		return lineCount, len(inputRunes) - lastLineStart
	}

	line = 0
	col = cursor
	for i := range cursor {
		if inputRunes[i] == '\n' {
			line++
			col = cursor - i - 1
		}
	}
	return line, col
}

// positionCursor moves the terminal cursor to the correct position using ANSI escape sequences.
//
// This function handles the complex task of positioning the cursor accurately in multi-line
// terminal output. It addresses several critical positioning scenarios:
//
//   - Single-line input: Positions cursor by moving left from end of line
//   - Multi-line input: Moves up to correct line, then positions horizontally
//   - Prefix handling: Accounts for prompt prefix length on the first line
//   - Continuation lines: Accounts for indentation on wrapped lines
//
// The algorithm prevents cursor positioning errors that caused visual glitches
// and crashes in the original go-prompt. Uses standard ANSI escape codes:
//   - \x1b[<n>A: Move cursor up n lines
//   - \x1b[<n>C: Move cursor right n characters
//   - \x1b[<n>D: Move cursor left n characters
//   - \r: Move cursor to beginning of line
//
// Critical for proper visual feedback during editing operations.
func (r *renderer) positionCursor(lines []string, cursorLine, cursorCol, prefixLen int) {
	// Calculate how many lines we need to move up from the last line
	totalLines := len(lines)
	if totalLines <= 1 {
		// Single line - move cursor back from end of line
		lineLen := len([]rune(lines[0]))
		if cursorCol < lineLen {
			runesAfterCursor := lineLen - cursorCol
			if runesAfterCursor > 0 {
				fmt.Fprintf(r.output, "\x1b[%dD", runesAfterCursor)
			}
		}
		return
	}

	// Multi-line - move up to correct line, then position horizontally
	linesToMoveUp := totalLines - 1 - cursorLine
	if linesToMoveUp > 0 {
		fmt.Fprintf(r.output, "\x1b[%dA", linesToMoveUp)
	}

	// Move to beginning of line and then to correct column
	fmt.Fprint(r.output, "\r")

	// Calculate total column position including prefix on first line
	totalCol := cursorCol
	if cursorLine == 0 {
		totalCol += prefixLen
	} else {
		totalCol += prefixLen // Indentation for continuation lines
	}

	if totalCol > 0 {
		fmt.Fprintf(r.output, "\x1b[%dC", totalCol)
	}
}

// calculateRenderedLines calculates the actual number of lines that will be rendered,
// accounting for both explicit newlines and terminal wrapping.
func (r *renderer) calculateRenderedLines(prefix, input string) int {
	// Get terminal width
	termWidth := 80 // Default fallback
	if r.terminal != nil {
		if width, _, err := r.terminal.Size(); err == nil && width > 0 {
			termWidth = width
		}
	}

	// If input is empty, we still have one line with just the prefix
	if input == "" {
		return 1
	}

	// Split by explicit newlines
	lines := strings.Split(input, "\n")

	totalLines := 0
	prefixLen := len([]rune(prefix))

	for i, line := range lines {
		lineRunes := []rune(line)

		// Calculate the actual length including prefix/indentation
		var actualLength int
		if i == 0 {
			// First line includes the actual prefix
			actualLength = prefixLen + len(lineRunes)
		} else {
			// Continuation lines have indentation (spaces) equal to prefix length
			actualLength = prefixLen + len(lineRunes)
		}

		// Calculate how many terminal lines this will take
		if actualLength == 0 || (i == 0 && actualLength == prefixLen) {
			// Empty line or just prefix
			totalLines++
		} else if termWidth > 0 {
			// Calculate wrapped lines based on terminal width
			// Use ceiling division: (actualLength + termWidth - 1) / termWidth
			wrappedLines := (actualLength + termWidth - 1) / termWidth
			if wrappedLines == 0 {
				wrappedLines = 1
			}
			totalLines += wrappedLines
		} else {
			totalLines++
		}
	}

	return totalLines
}
