package prompt

import (
	"context"
	"math/rand"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAcceptSuggestionWithReplaceRange covers the explicit-range primitive: a
// suggestion that names the input it overwrites is applied literally, whatever
// the word before the cursor happens to look like.
func TestAcceptSuggestionWithReplaceRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		initialText    string
		cursorPos      int
		suggestion     Suggestion
		expectedText   string
		expectedCursor int
	}{
		{
			name:           "a case-insensitive match replaces the typed word instead of being appended",
			initialText:    "sel",
			cursorPos:      3,
			suggestion:     Suggestion{Text: "SELECT", Replace: &Range{Start: 0, End: 3}},
			expectedText:   "SELECT",
			expectedCursor: 6,
		},
		{
			name:           "an empty range at the cursor inserts without touching the word before it",
			initialText:    "a.",
			cursorPos:      2,
			suggestion:     Suggestion{Text: "name", Replace: &Range{Start: 2, End: 2}},
			expectedText:   "a.name",
			expectedCursor: 6,
		},
		{
			name:           "a range ending before the cursor keeps the text after it",
			initialText:    "SELECT xx FROM t",
			cursorPos:      9,
			suggestion:     Suggestion{Text: "id", Replace: &Range{Start: 7, End: 9}},
			expectedText:   "SELECT id FROM t",
			expectedCursor: 9,
		},
		{
			name:           "a range is measured in runes, not bytes",
			initialText:    "名前 xxx",
			cursorPos:      6,
			suggestion:     Suggestion{Text: "yy", Replace: &Range{Start: 3, End: 5}},
			expectedText:   "名前 yyx",
			expectedCursor: 5,
		},
		{
			name:           "a range covering the whole buffer replaces all of it",
			initialText:    "abc",
			cursorPos:      1,
			suggestion:     Suggestion{Text: "z", Replace: &Range{Start: 0, End: 3}},
			expectedText:   "z",
			expectedCursor: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &Prompt{
				buffer: []rune(tt.initialText),
				cursor: tt.cursorPos,
			}
			p.acceptSuggestion(tt.suggestion)

			assert.Equal(t, tt.expectedText, string(p.buffer))
			assert.Equal(t, tt.expectedCursor, p.cursor)
		})
	}
}

// TestAcceptSuggestionClampsOutOfBoundsReplaceRange keeps a completer's
// arithmetic mistake from panicking the line editor: an out-of-range or
// inverted range is clamped to the buffer rather than slicing outside it.
func TestAcceptSuggestionClampsOutOfBoundsReplaceRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		initialText  string
		cursorPos    int
		replace      Range
		expectedText string
	}{
		{
			name:         "an end past the buffer is clamped to its length",
			initialText:  "abc",
			cursorPos:    3,
			replace:      Range{Start: 1, End: 99},
			expectedText: "aZ",
		},
		{
			name:         "a negative start is clamped to zero",
			initialText:  "abc",
			cursorPos:    3,
			replace:      Range{Start: -5, End: 2},
			expectedText: "Zc",
		},
		{
			name:         "an inverted range collapses to an insertion at its start",
			initialText:  "abc",
			cursorPos:    3,
			replace:      Range{Start: 2, End: 1},
			expectedText: "abZc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &Prompt{
				buffer: []rune(tt.initialText),
				cursor: tt.cursorPos,
			}
			p.acceptSuggestion(Suggestion{Text: "Z", Replace: &tt.replace})

			assert.Equal(t, tt.expectedText, string(p.buffer))
		})
	}
}

// TestCompleterOwnsMatchingWhenItNamesTheReplacedSpan drives the whole read
// loop. The prompt filters the completer's suggestions by a case-sensitive
// prefix test against the word before the cursor, so a completer matching by
// its own rule — case-insensitively, or on a qualified name the prompt would
// not split there — had its answer thrown away and TAB did nothing at all. A
// suggestion that names the span it replaces skips that filter.
func TestCompleterOwnsMatchingWhenItNamesTheReplacedSpan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		suggestion func(Document) []Suggestion
		want       string
	}{
		{
			name:  "a case-insensitive keyword completes over the lowercase word",
			input: "sel\t\r",
			suggestion: func(d Document) []Suggestion {
				return []Suggestion{{Text: "SELECT", Replace: &Range{Start: 0, End: d.CursorPosition}}}
			},
			want: "SELECT",
		},
		{
			name:  "a qualified name completes after the dot the prompt keeps in the word",
			input: "t.na\t\r",
			suggestion: func(d Document) []Suggestion {
				return []Suggestion{{Text: "name", Replace: &Range{Start: 2, End: d.CursorPosition}}}
			},
			want: "t.name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPrompt(newMockTerminal(tt.input), WithCompleter(tt.suggestion))
			got, err := p.RunWithContext(context.Background())
			if err != nil {
				t.Fatalf("RunWithContext returned error: %v", err)
			}
			if got != tt.want {
				t.Errorf("submitted line = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCompleterWithoutReplaceSpanKeepsPromptFiltering is the other half of the
// contract: a completer that names no span still gets the prompt's built-in
// prefix filter, so an existing one behaves exactly as it did.
func TestCompleterWithoutReplaceSpanKeepsPromptFiltering(t *testing.T) {
	t.Parallel()

	// "SELECT" does not prefix-match the typed "sel", so the filter drops it and
	// TAB leaves the line alone.
	p := newTestPrompt(newMockTerminal("sel\t\r"), WithCompleter(func(Document) []Suggestion {
		return []Suggestion{{Text: "SELECT"}}
	}))

	got, err := p.RunWithContext(context.Background())
	if err != nil {
		t.Fatalf("RunWithContext returned error: %v", err)
	}
	if want := "sel"; got != want {
		t.Errorf("submitted line = %q, want %q", got, want)
	}
}

// TestReplaceRangeInvariants is a property check over random spans: whatever a
// completer asks for, the edit must leave a buffer the line editor can keep
// rendering — a cursor inside it, and the runes outside the (clamped) span
// untouched.
func TestReplaceRangeInvariants(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(20260831)) //nolint:gosec // reproducible test input, not security
	alphabet := []rune("ab 日\t")

	for i := range 2000 {
		buf := make([]rune, rng.Intn(8))
		for j := range buf {
			buf[j] = alphabet[rng.Intn(len(alphabet))]
		}
		text := string(alphabet[:rng.Intn(len(alphabet))])
		// Spans reach past both ends of the buffer so clamping is exercised.
		span := Range{Start: rng.Intn(12) - 2, End: rng.Intn(12) - 2}

		p := &Prompt{buffer: slices.Clone(buf), cursor: len(buf)}
		p.replaceRange(span, text)

		start := min(max(span.Start, 0), len(buf))
		end := min(max(span.End, start), len(buf))
		want := string(buf[:start]) + text + string(buf[end:])

		if got := string(p.buffer); got != want {
			t.Fatalf("case %d: replaceRange(%v, %q) over %q = %q, want %q", i, span, text, string(buf), got, want)
		}
		if p.cursor < 0 || p.cursor > len(p.buffer) {
			t.Fatalf("case %d: cursor %d outside buffer of %d runes", i, p.cursor, len(p.buffer))
		}
		if p.cursor != start+len([]rune(text)) {
			t.Fatalf("case %d: cursor = %d, want %d (end of the inserted text)", i, p.cursor, start+len([]rune(text)))
		}
	}
}
