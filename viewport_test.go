package prompt

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
)

// tallBlock builds an entry of the given number of lines, each one naming its
// own index so a row can be told from the row above it on screen.
func tallBlock(lines int) string {
	out := make([]string, 0, lines)
	for i := range lines {
		out = append(out, fmt.Sprintf("line%02d", i))
	}
	return strings.Join(out, "\n")
}

// TestRendererRedrawsATallBlockWithoutScrollingTheScreen types nothing and
// redraws an entry taller than the terminal five times. A redraw that changes
// nothing must change nothing on screen: the renderer draws the block from the
// row it starts on, so a block taller than the terminal pushes the difference
// off the top of the screen on every keystroke, and what goes is the scrollback
// the application printed before the prompt.
func TestRendererRedrawsATallBlockWithoutScrollingTheScreen(t *testing.T) {
	t.Parallel()

	const width, height, lines = 20, 8, 12

	var out bytes.Buffer
	r := newRenderer(&out, ThemeDefault, &sizedMockTerminal{width: width, height: height})
	r.setContinuationPrefix("...> ")
	input := tallBlock(lines)
	screen := newBoundedScreenModel(width, height)

	for redraw := range 5 {
		out.Reset()
		if err := r.render("sql> ", input, len([]rune(input))); err != nil {
			t.Fatalf("render: %v", err)
		}
		screen.feed(out.String())
		if screen.scrolled != 0 {
			t.Fatalf("redraw %d scrolled the screen by %d rows: a %d-row block on a %d-row terminal takes the session's scrollback with it",
				redraw, screen.scrolled, lines, height)
		}
		if r.lastLines > height {
			t.Fatalf("redraw %d recorded a block of %d rows on a terminal of %d, so the next erase moves up past the top of the screen",
				redraw, r.lastLines, height)
		}
	}
}

// TestRendererDrawsTheCaretOnTheRowItIsEditing puts the cursor on a line that a
// block taller than the terminal has pushed off the top of the screen. The row
// the caret is drawn on has to be the row holding the character it is in front
// of, whatever the block's height: the move up to it is clamped at the top of
// the screen, so a caret belonging to a row that is not on screen lands wherever
// the clamp left it.
func TestRendererDrawsTheCaretOnTheRowItIsEditing(t *testing.T) {
	t.Parallel()

	const width, height, lines = 20, 8, 12
	input := tallBlock(lines)

	tests := map[string]struct {
		line int // the line the cursor is put at the start of
	}{
		"the first line of the entry": {line: 0},
		"a line near the top":         {line: 1},
		"a line in the middle":        {line: 6},
		"the last line":               {line: lines - 1},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			r := newRenderer(&out, ThemeDefault, &sizedMockTerminal{width: width, height: height})
			r.setContinuationPrefix("...> ")
			screen := newBoundedScreenModel(width, height)

			// The caret is moved to the line under test on a second render, the
			// way a cursor key does: the first render is what puts the block on
			// screen.
			cursor := len([]rune(input))
			for _, at := range []int{cursor, tt.line * len("line00\n")} {
				out.Reset()
				if err := r.render("sql> ", input, at); err != nil {
					t.Fatalf("render: %v", err)
				}
				screen.feed(out.String())
			}

			want := fmt.Sprintf("line%02d", tt.line)
			rows := screen.rows()
			if screen.row >= len(rows) || !strings.Contains(rows[screen.row], want) {
				t.Errorf("the caret is on row %d, which holds %q, and the cursor is in front of %q\n%q",
					screen.row, rowAt(rows, screen.row), want, rows)
			}
		})
	}
}

func rowAt(rows []string, row int) string {
	if row < 0 || row >= len(rows) {
		return ""
	}
	return rows[row]
}

// TestViewportTopMovesAsLittleAsTheCaretAllows pins the window's arithmetic on
// its own. A window that moves when it does not have to is a block that jumps
// under the cursor keys; one that does not move when it has to is a caret drawn
// on a row that is not on screen.
func TestViewportTopMovesAsLittleAsTheCaretAllows(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		previous, caret, total, height int
		want                           int
	}{
		"a block that fits is drawn whole":            {previous: 0, caret: 3, total: 5, height: 8, want: 0},
		"a window forgets where it was once it fits":  {previous: 4, caret: 3, total: 5, height: 8, want: 0},
		"the caret above the window pulls it up":      {previous: 4, caret: 2, total: 12, height: 8, want: 2},
		"the caret below the window pulls it down":    {previous: 0, caret: 9, total: 12, height: 8, want: 2},
		"a caret already in the window moves nothing": {previous: 2, caret: 5, total: 12, height: 8, want: 2},
		"the window stops at the block's last row":    {previous: 99, caret: 11, total: 12, height: 8, want: 4},
		"a height of one still holds the caret":       {previous: 0, caret: 7, total: 12, height: 1, want: 7},
		"a terminal of no height draws from the top":  {previous: 3, caret: 7, total: 12, height: 0, want: 0},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := viewportTop(tt.previous, tt.caret, tt.total, tt.height)
			if got != tt.want {
				t.Errorf("viewportTop(%d, %d, %d, %d) = %d, want %d",
					tt.previous, tt.caret, tt.total, tt.height, got, tt.want)
			}
			if tt.height > 0 && tt.total > 0 {
				if tt.caret < got || tt.caret >= got+tt.height {
					t.Errorf("the window %d..%d leaves the caret at row %d off screen", got, got+tt.height, tt.caret)
				}
			}
		})
	}
}

// TestBlockRowsAgreeWithTheHeightTheRendererCounts asks the two answers to the
// same question to match. calculateRenderedLines says how tall the block is,
// which is what the erase moves up by; blockRowsOf says which rows are drawn.
// A block cut into more or fewer rows than it was counted as puts the caret on
// a row holding something else, and leaves rows behind on the next erase.
func TestBlockRowsAgreeWithTheHeightTheRendererCounts(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"",
		"\n",
		"select 1",
		"select 1\nfrom t\nwhere x = 1",
		strings.Repeat("a", 200),
		strings.Repeat("あ", 60),
		"a\tb\tc\td\te\tf",
		strings.Repeat("a", 19) + "\t" + strings.Repeat("b", 30),
		"é" + strings.Repeat("x", 40),
		"one\n\n\nfour",
		strings.Repeat("x", 40) + "\n" + strings.Repeat("あ", 40),
	}
	prefixes := []string{"", "> ", "データ> ", strings.Repeat("p", 45)}

	for _, width := range []int{1, 2, 3, 7, 20, 21, 80} {
		for _, prefix := range prefixes {
			for _, input := range inputs {
				r := newRenderer(io.Discard, ThemeDefault, &sizedMockTerminal{width: width, height: 24})
				r.setContinuationPrefix("...> ")
				r.measureTerminal()
				rows := r.blockRowsOf(prefix, input)
				if counted := r.calculateRenderedLines(prefix, input); len(rows) != counted {
					t.Errorf("width %d prefix %q input %q: the block is cut into %d rows and counted as %d",
						width, prefix, input, len(rows), counted)
				}
				if got, want := textOf(rows), r.linePrefixesOf(prefix, input); got != want {
					t.Errorf("width %d prefix %q input %q: the rows hold %q, the block is %q",
						width, prefix, input, got, want)
				}
			}
		}
	}
}

// textOf returns everything the rows hold, in the order it is drawn.
func textOf(rows []blockRow) string {
	var out strings.Builder
	for _, row := range rows {
		for _, segment := range row {
			out.WriteString(segment.text)
		}
	}
	return out.String()
}

// linePrefixesOf returns the text the block draws: every line with the prefix
// that goes in front of it, and no line breaks, because a break is a row rather
// than a character on one.
func (r *renderer) linePrefixesOf(prefix, input string) string {
	var out strings.Builder
	for i, line := range r.splitIntoLines(input) {
		out.WriteString(r.linePrefix(i, prefix))
		out.WriteString(line)
	}
	return out.String()
}

// TestAClippedBlockShowsTheSameRowsAsAScreenWithRoomForIt draws the same entry
// twice: once on a terminal tall enough to hold it, and once on one that has to
// clip it. The short screen has to show exactly the window the tall one shows at
// those rows.
//
// It is the check that the renderer's own wrapping matches the terminal's. A
// block that fits is written out and left to the terminal to wrap; a clipped one
// is cut into rows here, because a window starting in the middle of a wrapped
// line has no whole line to hand over. Two rules for one job is two chances to
// disagree, and a disagreement moves the text rather than only the caret.
func TestAClippedBlockShowsTheSameRowsAsAScreenWithRoomForIt(t *testing.T) {
	t.Parallel()

	const width, height = 20, 6

	tests := map[string]string{
		"lines of their own":     tallBlock(14),
		"a line that wraps":      "select " + strings.Repeat("x", 200),
		"wide runes":             strings.Repeat("あ", 60),
		"a tab across rows":      strings.Repeat("a\tb\t", 20),
		"empty lines among them": "one\n\n\n\n\n\n\nnine\n\n\n\n\ntwelve",
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for _, cursor := range []int{0, len([]rune(input)) / 2, len([]rune(input))} {
				var tall bytes.Buffer
				roomy := newRenderer(&tall, ThemeDefault, &sizedMockTerminal{width: width, height: 100})
				roomy.setContinuationPrefix("...> ")
				if err := roomy.render("sql> ", input, cursor); err != nil {
					t.Fatalf("render: %v", err)
				}
				whole := newScreenModel(width)
				whole.feed(tall.String())

				var short bytes.Buffer
				clipped := newRenderer(&short, ThemeDefault, &sizedMockTerminal{width: width, height: height})
				clipped.setContinuationPrefix("...> ")
				if err := clipped.render("sql> ", input, cursor); err != nil {
					t.Fatalf("render: %v", err)
				}
				window := newBoundedScreenModel(width, height)
				window.feed(short.String())

				rows := whole.rows()
				top := clipped.viewTop
				want := rows[min(top, len(rows)):min(top+height, len(rows))]
				if got := window.rows(); !equalRows(got, want) {
					t.Errorf("cursor %d: the clipped screen shows\n%q\nthe rows the tall one shows at %d..%d are\n%q",
						cursor, got, top, top+height, want)
				}
			}
		})
	}
}

func equalRows(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestAClippedBlockColorsTheRunsTheHighlighterNamed puts a highlighter on a
// block taller than the terminal. A span indexes the input, and a clipped row
// holds a run of it that starts wherever the row starts, so the run has to be
// colored by where its characters are in the input rather than by where they are
// on the row.
func TestAClippedBlockColorsTheRunsTheHighlighterNamed(t *testing.T) {
	t.Parallel()

	const width, height, lines = 20, 8, 12
	input := tallBlock(lines)
	red := Color{R: 255}

	var out bytes.Buffer
	r := newRenderer(&out, ThemeDefault, &sizedMockTerminal{width: width, height: height})
	r.setContinuationPrefix("...> ")
	// Every line's two digits, and nothing else.
	r.setHighlighter(func(in string) []StyleSpan {
		lines := strings.Split(in, "\n")
		spans := make([]StyleSpan, 0, len(lines))
		for i, line := range lines {
			start := i * (len([]rune(line)) + 1)
			spans = append(spans, StyleSpan{Start: start + 4, End: start + 6, Color: red})
		}
		return spans
	})
	if err := r.render("sql> ", input, len([]rune(input))); err != nil {
		t.Fatalf("render: %v", err)
	}

	drawn := out.String()
	for line := lines - height; line < lines; line++ {
		want := fmt.Sprintf("%s%02d%s", red.ToANSI(), line, ansiReset())
		if !strings.Contains(drawn, want) {
			t.Errorf("line %d is drawn without the color its span named: %q", line, drawn)
		}
	}
}
