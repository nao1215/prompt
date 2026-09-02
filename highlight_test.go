package prompt

import (
	"bytes"
	"strings"
	"testing"
)

// renderedInput renders input through a renderer wired to the given highlighter
// and returns what was written, so a test can look at the escape sequences the
// prompt emitted rather than only at the text.
func renderedInput(t *testing.T, input string, highlight func(string) []StyleSpan) string {
	t.Helper()

	var out bytes.Buffer
	r := newRenderer(&out, ThemeDefault, newMockTerminal(""))
	r.setHighlighter(highlight)
	if err := r.renderLines("", input); err != nil {
		t.Fatalf("renderLines(%q): %v", input, err)
	}
	return out.String()
}

// visibleText strips every escape sequence, leaving what the terminal shows.
// Highlighting must never change that: it decides colors, not content.
func visibleText(rendered string) string {
	var b strings.Builder
	for i := 0; i < len(rendered); {
		if rendered[i] == '\x1b' {
			for i < len(rendered) && rendered[i] != 'm' && rendered[i] != 'K' {
				i++
			}
			i++ // the terminating byte
			continue
		}
		if rendered[i] == '\r' {
			i++
			continue
		}
		b.WriteByte(rendered[i])
		i++
	}
	return b.String()
}

var (
	red  = Color{R: 255, G: 0, B: 0}
	blue = Color{R: 0, G: 0, B: 255}
)

// TestHighlighterColorsTheSpansItNames covers the whole point: a run the
// highlighter names is drawn in its color, and everything else in the scheme's
// input color.
func TestHighlighterColorsTheSpansItNames(t *testing.T) {
	t.Parallel()

	rendered := renderedInput(t, "SELECT a", func(string) []StyleSpan {
		return []StyleSpan{{Start: 0, End: 6, Color: red}}
	})

	if !strings.Contains(rendered, red.ToANSI()+"SELECT") {
		t.Errorf("the named run was not drawn in its own color:\n%q", rendered)
	}
	if !strings.Contains(rendered, ThemeDefault.Input.ToANSI()+" a") {
		t.Errorf("the rest was not drawn in the scheme's input color:\n%q", rendered)
	}
}

// TestHighlightingLeavesTheTextAlone is the invariant that matters most: what
// the terminal shows is the input, whatever the highlighter says about it. The
// prompt measures its own layout from the plain text, so a highlighter that
// changed the text would move the cursor away from the character under it.
func TestHighlightingLeavesTheTextAlone(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"", "a", "SELECT 1", "SELECT\n  a\nFROM t", "日本語 AS name", "  leading", "trailing  ",
	}
	for _, input := range inputs {
		// A highlighter that names a run in every other position, including ones
		// that reach past the end and overlap each other.
		spans := func(in string) []StyleSpan {
			var out []StyleSpan
			for i := 0; i < len([]rune(in))+3; i += 2 {
				out = append(out, StyleSpan{Start: i, End: i + 3, Color: red})
			}
			return out
		}
		got := visibleText(renderedInput(t, input, spans))
		if got != input {
			t.Errorf("highlighting %q changed what is shown to %q", input, got)
		}
	}
}

// TestHighlightingSpansEachLineOfAMultilineInput covers a span that reaches
// across a line break, and one that starts on a later line: the offsets a
// highlighter reports are into the whole input, not into a line of it.
func TestHighlightingSpansEachLineOfAMultilineInput(t *testing.T) {
	t.Parallel()

	// "ab\ncd\nef": rune 3 is 'c', rune 6 is 'e'.
	rendered := renderedInput(t, "ab\ncd\nef", func(string) []StyleSpan {
		return []StyleSpan{
			{Start: 1, End: 4, Color: red},  // "b", the break, and "c"
			{Start: 6, End: 8, Color: blue}, // "ef" on the last line
		}
	})

	for _, want := range []string{
		red.ToANSI() + "b", // the first line's tail
		red.ToANSI() + "c", // and its continuation on the next
		blue.ToANSI() + "ef",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered output does not contain %q:\n%q", want, rendered)
		}
	}
	if got := visibleText(rendered); got != "ab\ncd\nef" {
		t.Errorf("visible text = %q, want the input unchanged", got)
	}
}

// TestHighlightingNormalizesWhatItIsGiven covers a highlighter that reports
// nonsense. The prompt draws over it rather than panicking or cutting a line
// short: a color is a decoration, and getting one wrong must not cost the user
// the line they are typing.
func TestHighlightingNormalizesWhatItIsGiven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		spans []StyleSpan
	}{
		{name: "a span reaching past the end", spans: []StyleSpan{{Start: 2, End: 99, Color: red}}},
		{name: "a span starting before the beginning", spans: []StyleSpan{{Start: -5, End: 3, Color: red}}},
		{name: "an inverted span", spans: []StyleSpan{{Start: 5, End: 2, Color: red}}},
		{name: "an empty span", spans: []StyleSpan{{Start: 3, End: 3, Color: red}}},
		{name: "spans out of order", spans: []StyleSpan{{Start: 5, End: 7, Color: red}, {Start: 0, End: 2, Color: blue}}},
		{name: "overlapping spans", spans: []StyleSpan{{Start: 0, End: 5, Color: red}, {Start: 3, End: 7, Color: blue}}},
		{name: "a span entirely past the end", spans: []StyleSpan{{Start: 50, End: 60, Color: red}}},
		{name: "no spans at all", spans: nil},
	}

	const input = "SELECT 1"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := visibleText(renderedInput(t, input, func(string) []StyleSpan { return tt.spans }))
			if got != input {
				t.Errorf("visible text = %q, want %q", got, input)
			}
		})
	}
}

// TestWithoutAHighlighterTheInputIsOneColor is the other half of the contract:
// a prompt built without one renders exactly as it did before highlighting
// existed.
func TestWithoutAHighlighterTheInputIsOneColor(t *testing.T) {
	t.Parallel()

	rendered := renderedInput(t, "SELECT 1", nil)
	want := ThemeDefault.Input.ToANSI() + "SELECT 1" + ansiReset()
	if !strings.Contains(rendered, want) {
		t.Errorf("rendered output does not contain %q:\n%q", want, rendered)
	}
}

// TestHighlightingDoesNotMoveTheCursor is the layout half of the contract. The
// prompt measures rows and columns from the plain input, so the escape
// sequences a highlighter causes must cost no columns: with and without one,
// the cursor has to land in the same place.
func TestHighlightingDoesNotMoveTheCursor(t *testing.T) {
	t.Parallel()

	inputs := []string{"SELECT 1", "SELECT\n  a\nFROM t", "日本語 AS name", ""}
	for _, input := range inputs {
		for cursor := range len([]rune(input)) + 1 {
			plain := renderCursorRow(t, input, cursor, nil)
			colored := renderCursorRow(t, input, cursor, func(in string) []StyleSpan {
				var out []StyleSpan
				for i := 0; i+1 < len([]rune(in)); i += 3 {
					out = append(out, StyleSpan{Start: i, End: i + 2, Color: red})
				}
				return out
			})
			if plain != colored {
				t.Errorf("input %q cursor %d: the cursor landed on row %d with highlighting and %d without",
					input, cursor, colored, plain)
			}
		}
	}
}

// renderCursorRow renders input with the cursor at the given position and
// reports the row the renderer left the cursor on.
func renderCursorRow(t *testing.T, input string, cursor int, highlight func(string) []StyleSpan) int {
	t.Helper()

	var out bytes.Buffer
	r := newRenderer(&out, ThemeDefault, newMockTerminal(""))
	r.setHighlighter(highlight)
	_, row, err := r.renderMainLine("$ ", input, cursor)
	if err != nil {
		t.Fatalf("renderMainLine(%q, %d): %v", input, cursor, err)
	}
	return row
}
