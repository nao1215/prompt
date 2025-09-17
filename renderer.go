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
	output      io.Writer    // Target output writer (typically stdout or colorable wrapper)
	colorScheme *ColorScheme // Color configuration for themed rendering
	lastLines   int          // Track number of lines rendered for efficient cleanup
}

// newRenderer creates a new renderer with the given output and color scheme.
func newRenderer(output io.Writer, colorScheme *ColorScheme) *renderer {
	return &renderer{
		output:      output,
		colorScheme: colorScheme,
	}
}

// render displays the prompt with the current input.
func (r *renderer) render(prefix, input string, cursor int) error {
	return r.renderWithSuggestions(prefix, input, cursor, nil, 0)
}

// renderWithSuggestions displays the prompt with completion suggestions.
func (r *renderer) renderWithSuggestions(prefix, input string, cursor int, suggestions []Suggestion, selected int) error {
	// Clear previous output
	r.clearPreviousLines()

	// Render main prompt line
	if err := r.renderMainLine(prefix, input, cursor); err != nil {
		return err
	}

	// Count lines in the input
	inputLines := len(r.splitIntoLines(input))

	// Render suggestions if any
	if len(suggestions) > 0 {
		if err := r.renderSuggestions(suggestions, selected); err != nil {
			return err
		}
	}

	r.lastLines = inputLines + len(suggestions) // Input lines + suggestion lines
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

// renderSuggestions renders the completion suggestions below the main line.
func (r *renderer) renderSuggestions(suggestions []Suggestion, selected int) error {
	if _, err := fmt.Fprint(r.output, "\r\n"); err != nil { // Move to next line with proper line ending
		return err
	}

	maxSuggestions := 10 // Limit number of displayed suggestions
	if len(suggestions) > maxSuggestions {
		suggestions = suggestions[:maxSuggestions]
	}

	for i, suggestion := range suggestions {
		// Clear line and move to beginning
		if _, err := fmt.Fprint(r.output, "\r\x1b[K"); err != nil {
			return err
		}

		// Render selection indicator and suggestion
		if i == selected {
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
		if i < len(suggestions)-1 {
			if _, err := fmt.Fprint(r.output, "\r\n"); err != nil {
				return err
			}
		}
	}

	// Move cursor back to main input line
	if len(suggestions) > 0 {
		if _, err := fmt.Fprintf(r.output, "\x1b[%dA", len(suggestions)); err != nil {
			return err
		}
	}

	return nil
}

// clearPreviousLines clears the previously rendered lines.
func (r *renderer) clearPreviousLines() {
	if r.lastLines <= 1 {
		return
	}

	// Move to beginning of the block and clear all lines
	for range r.lastLines - 1 {
		fmt.Fprint(r.output, "\x1b[E") // Move to beginning of next line
		fmt.Fprint(r.output, "\x1b[K") // Clear line
	}

	// Move back to the first line
	fmt.Fprintf(r.output, "\x1b[%dA", r.lastLines-1)
	fmt.Fprint(r.output, "\r") // Move to beginning of line
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
