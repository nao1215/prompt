package prompt

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"
)

// layout walks s a cell at a time and reports how many rows it advances from
// the start of a row, and the column it ends on. That column can equal width,
// which is the state a terminal is in once it has filled its last cell: the
// cursor stays there until another character arrives, rather than moving to a
// row that does not exist yet. Only a written cell reaches that state; a tab
// stops one column short of it.
//
// Everything here is measured in cells rather than runes, because a rune is not
// a cell: "データ> " is 5 runes and 8 columns, an emoji is 1 rune and 2 columns,
// and a combining mark is a rune that occupies none.
//
// Walking is not the same calculation as dividing the total width by the
// terminal's, in two ways that each cost a cell. A terminal never splits a
// glyph across the right margin, so a double-width rune that does not fit moves
// whole to the next row and leaves the last cell blank; and a filled row is
// still one row. The division misses both, and the drift shows up twice: in the
// column the cursor is drawn at, and in the height the redraw erases.
//
// A tab is measured against tab stops rather than by rune width: runewidth has
// no notion of stops and reports zero, while a terminal advances a tab up to
// eight columns, so a tab measured as nothing drew the cursor that far left of
// the text. A tab reaches the buffer through a bracketed paste — pasted SQL, a
// TSV row — since a typed Tab runs completion instead.
func layout(s string, width int) (rows, col int) {
	for _, r := range s {
		if r == '\t' {
			if col >= width {
				rows++
				col = 0
			}
			// A terminal stops a tab at the last column instead of carrying it
			// onto the next row, and stopping there is not the same as filling
			// the row: no cell was written, so the cursor sits on the last
			// column with no wrap owed and the next character prints beside it.
			// Reporting width here claimed a wrap the terminal never made.
			col = min(col+tabWidth-col%tabWidth, width-1)
			continue
		}
		w := runewidth.RuneWidth(r)
		if w == 0 {
			continue
		}
		if col+w > width {
			rows++
			col = 0
		}
		col += w
	}
	return rows, col
}

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
	// highlighter colors runs of the input as it is drawn. Nil draws the whole
	// input in the scheme's color.
	highlighter       func(string) []StyleSpan
	output            io.Writer         // Target output writer (typically stdout or colorable wrapper)
	colorScheme       *ColorScheme      // Color configuration for themed rendering
	lastLines         int               // Track number of lines rendered for efficient cleanup
	suggestionsActive bool              // Track if suggestions are currently displayed
	terminal          terminalInterface // Terminal interface for getting size information
	// lastCursorRow is the row, counted from the first row of the last rendered
	// block, where that render left the terminal cursor. The next render erases
	// from there, so it has to be remembered rather than assumed: a cursor moved
	// onto an earlier row (a left arrow crossing a line break) is not on the
	// block's last row, and erasing as if it were walked the prompt up the screen
	// one row per keystroke, taking a line of scrollback with it each time.
	lastCursorRow int
	// width is the terminal's width in cells, read once at the start of each
	// render. Every piece of the wrap arithmetic needs it, and asking the
	// terminal per question is both wasted work and a chance for the answer to
	// change halfway through a redraw. Zero until the first render, which is why
	// the readers fall back rather than trusting it.
	width int
	// continuationPrefix is drawn in front of every line after the first, so a
	// multiline entry shows that the prompt is still collecting input. Empty
	// (the default) keeps continuation lines flush against the left margin.
	continuationPrefix string
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

// setContinuationPrefix sets the string drawn in front of every line after the
// first. It is applied on the next render.
// setHighlighter sets what colors the input on the next render. A nil one
// draws the whole input in the scheme's color, which is what a prompt without
// WithHighlighter does.
func (r *renderer) setHighlighter(highlighter func(string) []StyleSpan) {
	r.highlighter = highlighter
}

func (r *renderer) setContinuationPrefix(prefix string) {
	r.continuationPrefix = prefix
}

// render displays the prompt with the current input.
func (r *renderer) render(prefix, input string, cursor int) error {
	return r.renderWithSuggestionsOffset(prefix, input, cursor, nil, 0, 0)
}

// renderWithSuggestionsOffset displays the prompt with completion suggestions and scrolling support.
func (r *renderer) renderWithSuggestionsOffset(prefix, input string, cursor int, suggestions []Suggestion, selected int, offset int) error {
	// One measurement for the whole render: what is cleared, what is drawn, and
	// where the cursor lands are three answers to the same question and have to
	// agree.
	r.measureTerminal()

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
		suggestionRows, err := r.renderSuggestionsWithOffset(prefix, input, cursor, suggestions, selected, offset)
		if err != nil {
			return err
		}

		// Update state AFTER rendering. The menu's height is the rows it drew, not
		// the suggestions it holds: a suggestion wider than the terminal wraps onto
		// more than one, and counting entries left those rows out of the erase.
		r.lastLines = inputLines + suggestionRows
		r.suggestionsActive = true
		// The cursor is left wherever the last suggestion ended, on the block's
		// last row.
		r.lastCursorRow = r.lastLines - 1
	} else {
		// No suggestions - render normally with cursor
		cursorRow, err := r.renderMainLine(prefix, input, cursor)
		if err != nil {
			return err
		}

		// Show cursor
		if _, err := fmt.Fprint(r.output, "\x1b[?25h"); err != nil {
			return err
		}

		// Update lastLines to match the actual number of lines rendered
		r.lastLines = inputLines
		r.lastCursorRow = cursorRow
		r.suggestionsActive = false
	}

	return nil
}

// renderMainLine renders the main prompt line with prefix and input. It returns
// the row, counted from the block's first row, where it left the cursor.
func (r *renderer) renderMainLine(prefix, input string, cursor int) (int, error) {
	if err := r.renderLines(prefix, input); err != nil {
		return 0, err
	}

	// Position cursor correctly
	lines := r.splitIntoLines(input)
	inputRunes := []rune(input)
	cursorLine, cursorCol := r.findCursorPosition(inputRunes, cursor)
	return r.positionCursor(lines, cursorLine, cursorCol, prefix), nil
}

// renderMainLineWithoutCursor renders the main prompt line without cursor positioning (for suggestions)
func (r *renderer) renderMainLineWithoutCursor(prefix, input string) error {
	return r.renderLines(prefix, input)
}

// renderLines renders the prompt lines without cursor positioning (shared logic)
func (r *renderer) renderLines(prefix, input string) error {
	// Move to beginning of line and clear it
	if _, err := fmt.Fprint(r.output, "\r\x1b[K"); err != nil {
		return err
	}

	// Split input into lines
	lines := r.splitIntoLines(input)
	spans := r.spansFor(input)
	lineStart := 0 // the first rune of the current line, in the whole input

	// Render each line
	for lineIndex, line := range lines {
		if lineIndex > 0 {
			// Continuation lines: ensure we start from line beginning
			if _, err := fmt.Fprint(r.output, "\r\x1b[K"); err != nil {
				return err
			}
		}

		// The first line carries the prompt prefix; the rest carry the
		// continuation prefix, which is empty unless the caller set one.
		linePrefix := r.continuationPrefix
		if lineIndex == 0 {
			linePrefix = prefix
		}
		if linePrefix != "" {
			if _, err := fmt.Fprint(r.output, r.colorScheme.Prefix.ToANSI()); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, linePrefix); err != nil {
				return err
			}
			if _, err := fmt.Fprint(r.output, ansiReset()); err != nil {
				return err
			}
		}

		// Render line content with color
		if err := r.renderLineContent(line, lineStart, spans); err != nil {
			return err
		}

		// Move to next line if not the last line
		if lineIndex < len(lines)-1 {
			if _, err := fmt.Fprint(r.output, "\n"); err != nil {
				return err
			}
		}
		// The next line starts after this one and the newline between them.
		lineStart += len([]rune(line)) + 1
	}

	return nil
}

// renderLineContent writes one line, coloring the runs the highlighter named.
// lineStart is the line's first rune offset in the whole input, because that is
// what the spans are measured in.
//
// Only what is written changes here. The prompt measures its layout from the
// plain text, so the escape sequences added between runs cost no columns and
// cannot move the cursor away from the character under it.
func (r *renderer) renderLineContent(line string, lineStart int, spans []StyleSpan) error {
	runes := []rune(line)
	base := r.colorScheme.Input.ToANSI()

	write := func(color, text string) error {
		if _, err := fmt.Fprint(r.output, color, text, ansiReset()); err != nil {
			return err
		}
		return nil
	}

	pos := 0 // the first rune of this line not yet written
	for _, span := range spans {
		start, end := span.Start-lineStart, span.End-lineStart
		if end <= pos {
			continue // entirely behind us, on an earlier line
		}
		if start >= len(runes) {
			break // and the rest are on later lines, because spans are ordered
		}
		start = max(start, pos)
		end = min(end, len(runes))
		if start > pos {
			if err := write(base, string(runes[pos:start])); err != nil {
				return err
			}
		}
		if err := write(span.Color.ToANSI(), string(runes[start:end])); err != nil {
			return err
		}
		pos = end
	}
	// The tail, and the whole of a line no span touched. An empty line still
	// writes its color, which is what it did before spans existed.
	return write(base, string(runes[pos:]))
}

// spansFor asks the highlighter about input and returns what it said in the
// order and shape the renderer can walk: sorted by start, clamped to the input,
// with empty runs dropped and overlaps trimmed so the earlier run keeps what it
// claimed.
//
// A highlighter is application code deciding a decoration. It is normalized
// rather than trusted, and never rejected, because getting a color wrong must
// not cost the user the line they are typing.
func (r *renderer) spansFor(input string) []StyleSpan {
	if r.highlighter == nil {
		return nil
	}
	reported := r.highlighter(input)
	if len(reported) == 0 {
		return nil
	}

	limit := len([]rune(input))
	spans := make([]StyleSpan, 0, len(reported))
	for _, span := range reported {
		span.Start = min(max(span.Start, 0), limit)
		span.End = min(max(span.End, 0), limit)
		if span.Start >= span.End {
			continue // empty, or inverted and therefore meaningless
		}
		spans = append(spans, span)
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })

	out := spans[:0]
	end := 0
	for _, span := range spans {
		span.Start = max(span.Start, end)
		if span.Start >= span.End {
			continue // wholly covered by the run before it
		}
		out = append(out, span)
		end = span.End
	}
	return out
}

// renderSuggestionsWithOffset renders the completion suggestions with scrolling
// support. It returns how many terminal rows the menu occupies, which the next
// erase moves up by: the visible range is decided here, so the count is too.
func (r *renderer) renderSuggestionsWithOffset(_, _ string, _ int, suggestions []Suggestion, selected int, offset int) (int, error) {
	// Start rendering suggestions
	if _, err := fmt.Fprint(r.output, "\r\n"); err != nil {
		return 0, err
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

	rows := 0
	for i, suggestion := range visibleSuggestions {
		// Clear line and move to beginning
		if _, err := fmt.Fprint(r.output, "\r\x1b[K"); err != nil {
			return 0, err
		}

		indicator := "  "
		textColor := r.colorScheme.Suggestion.Text.ToANSI()
		if i == visibleSelected {
			indicator = "\u25b6 "
			textColor = r.colorScheme.Selected.ToANSI()
		}
		if _, err := fmt.Fprint(r.output, textColor, indicator, suggestion.Text, ansiReset()); err != nil {
			return 0, err
		}

		if suggestion.Description != "" {
			if _, err := fmt.Fprint(r.output, " ", r.colorScheme.Suggestion.Description.ToANSI(), "- ", suggestion.Description, ansiReset()); err != nil {
				return 0, err
			}
		}

		// The erase is counted in rows, so a suggestion too wide for the
		// terminal contributes the rows it wraps onto, not one.
		wrapped, _ := layout(suggestionCells(indicator, suggestion), r.terminalWidth())
		rows += wrapped + 1

		// Move to next line (except for last suggestion) with proper line ending
		if i < len(visibleSuggestions)-1 {
			if _, err := fmt.Fprint(r.output, "\r\n"); err != nil {
				return 0, err
			}
		}
	}

	// Leave cursor at the end of suggestions
	// Parent function will handle final cursor positioning
	return rows, nil
}

// suggestionCells returns the printable text of one menu row: what the loop in
// renderSuggestionsWithOffset prints, minus the color escapes, which occupy no
// cells. The two have to stay in step, or the height is measured against text
// that was never drawn.
func suggestionCells(indicator string, s Suggestion) string {
	line := indicator + s.Text
	if s.Description != "" {
		line += " - " + s.Description
	}
	return line
}

// clearPreviousLines clears the previously rendered lines.
// clearScreen clears the entire terminal screen and scrollback and homes the
// cursor, then resets the line-tracking state so the next render draws the
// prompt at the top. It implements the Ctrl+L clear-screen behavior.
func (r *renderer) clearScreen() {
	fmt.Fprint(r.output, "\x1b[H\x1b[2J\x1b[3J")
	r.forgetBlock()
}

// forgetBlock drops what the renderer remembers about the block on screen, so
// the next render erases only the line it is on.
//
// It is called when a line ends and when the screen is cleared. What was drawn
// belongs to the finished line then, and the application prints its own output
// underneath it before the next prompt appears. A render that still moved up to
// erase "its" block would erase that output instead: after an entry that
// occupied two rows, the first row erased was the last row the application had
// printed, so a result table lost its bottom border.
func (r *renderer) forgetBlock() {
	r.lastLines = 1
	r.lastCursorRow = 0
	r.suggestionsActive = false
}

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

	// Move up by the rows between the cursor and the top of the block — not by
	// the block's height, which is the same number only while the cursor is on
	// the last row. See lastCursorRow.
	if r.lastCursorRow > 0 {
		fmt.Fprintf(r.output, "\x1b[%dA", r.lastCursorRow)
	}

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
// Simplified approach for multiline:
//   - Single-line input: Normal positioning with prefix
//   - Multi-line input: Continuation lines always start from line beginning (column 0)
//   - No complex calculations - just move to target line and position from start
//
// Uses standard ANSI escape codes:
//   - \x1b[<n>A: Move cursor up n lines
//   - \x1b[<n>C: Move cursor right n characters
//   - \r: Move cursor to beginning of line
//
// cursorCol is a rune index within its line; every distance written here is the
// display width of the text it spans, so a wide or zero-width character lands the
// cursor on the cell the user sees.
func (r *renderer) positionCursor(lines []string, cursorLine, cursorCol int, prefix string) int {
	row, col := r.cursorRowCol(lines, cursorLine, cursorCol, prefix)

	// Rendering ends at the foot of the block, so the move is from there up to
	// the cursor's row and then across to its column. It is done in absolute
	// terms — up, to column 0, then right — rather than by moving back the width
	// of the text after the cursor: a backward move stops at the left margin of
	// the row it is on, so on a line long enough to wrap it could never reach the
	// row above.
	if up := r.blockRows(lines, prefix) - 1 - row; up > 0 {
		fmt.Fprintf(r.output, "\x1b[%dA", up)
	}
	fmt.Fprint(r.output, "\r")
	if col > 0 {
		fmt.Fprintf(r.output, "\x1b[%dC", col)
	}
	return row
}

// cursorRowCol returns where the cursor sits inside the rendered block: the row
// counted from the block's first row, and the column on that row. Both are in
// terminal cells and both account for wrapping, so a logical line wide enough to
// occupy three rows contributes three.
func (r *renderer) cursorRowCol(lines []string, cursorLine, cursorCol int, prefix string) (row, col int) {
	width := r.terminalWidth()
	if cursorLine >= len(lines) {
		cursorLine = len(lines) - 1
	}
	for i := range cursorLine {
		row += r.lineRows(i, lines[i], prefix)
	}

	lineRunes := []rune(lines[cursorLine])
	if cursorCol > len(lineRunes) {
		cursorCol = len(lineRunes)
	}
	rows, col := layout(r.linePrefix(cursorLine, prefix)+string(lineRunes[:cursorCol]), width)
	if col >= width {
		// The row is full, and the terminal is holding the cursor on its last
		// cell. Reporting the row below it — which the text has not reached —
		// left the next redraw erasing one row too high, taking the line above
		// the prompt with it.
		col = width - 1
	}
	return row + rows, col
}

// blockRows returns how many terminal rows the rendered block occupies.
func (r *renderer) blockRows(lines []string, prefix string) int {
	return r.calculateRenderedLines(prefix, strings.Join(lines, "\n"))
}

// lineRows returns how many terminal rows one logical line occupies, including
// whatever prefix is drawn in front of it.
func (r *renderer) lineRows(lineIndex int, line, prefix string) int {
	rows, _ := layout(r.linePrefix(lineIndex, prefix)+line, r.terminalWidth())
	return rows + 1
}

// linePrefix returns what is drawn in front of a line: the prompt prefix on the
// first, the continuation prefix on the rest.
func (r *renderer) linePrefix(lineIndex int, prefix string) string {
	if lineIndex == 0 {
		return prefix
	}
	return r.continuationPrefix
}

// terminalWidth returns the width this render is measuring against, falling back
// to 80 when the terminal has not been asked yet or could not say. The fallback
// keeps the wrap arithmetic away from a division by zero.
func (r *renderer) terminalWidth() int {
	if r.width > 0 {
		return r.width
	}
	return defaultTerminalWidth
}

// measureTerminal reads the terminal's width for the render about to happen.
func (r *renderer) measureTerminal() {
	r.width = 0
	if r.terminal == nil {
		return
	}
	if width, _, err := r.terminal.Size(); err == nil && width > 0 {
		r.width = width
	}
}

// defaultTerminalWidth is the width assumed when the terminal cannot report one.
const defaultTerminalWidth = 80

// tabWidth is the spacing between tab stops. Eight is the terminal default, and
// nothing here can read the stops a session has actually set.
const tabWidth = 8

// calculateRenderedLines calculates the actual number of lines that will be rendered,
// accounting for both explicit newlines and terminal wrapping.
//
// Every line is measured the same way, including one holding nothing. A prompt
// waiting for its first keystroke still draws its prefix, and a prefix wider
// than the terminal occupies as many rows as it needs — recording that block as
// one row left the first keystroke redrawing the prefix underneath the rows
// already on screen, once per line the user entered.
func (r *renderer) calculateRenderedLines(prefix, input string) int {
	totalLines := 0
	for i, line := range strings.Split(input, "\n") {
		totalLines += r.lineRows(i, line, prefix)
	}
	return totalLines
}
