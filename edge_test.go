package prompt

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// TestScrollToSelectionAbove checks that a selection above the menu's window
// scrolls the window up to start at it, and that an empty menu leaves the
// offset alone.
func TestScrollToSelectionAbove(t *testing.T) {
	t.Parallel()

	p := newTestPrompt(newMockTerminal(""))
	suggestions := make([]Suggestion, 10)
	if got := p.scrollToSelection(suggestions, 2, 6); got != 2 {
		t.Errorf("scrollToSelection(selected 2, offset 6) = %d, want 2", got)
	}
	if got := p.scrollToSelection(nil, 0, 3); got != 3 {
		t.Errorf("scrollToSelection of no suggestions = %d, want the offset 3", got)
	}
}

// TestWordRightFromAGap checks that Ctrl+Right from between two words crosses
// the gap and the next word, ending after it.
func TestWordRightFromAGap(t *testing.T) {
	t.Parallel()

	p := newTestPrompt(newMockTerminal(""))
	p.buffer = []rune("ab  cd ef")
	p.cursor = 2
	if got := p.findWordBoundary(1); got != 6 {
		t.Errorf("findWordBoundary(+1) from 2 in %q = %d, want 6", string(p.buffer), got)
	}
}

// TestFileCompletionOfAMissingDirectory checks that a path into a directory
// that does not exist completes to nothing.
func TestFileCompletionOfAMissingDirectory(t *testing.T) {
	t.Parallel()

	if got := completeFilePath(filepath.Join(t.TempDir(), "missing", "x")); got != nil {
		t.Errorf("completeFilePath in a missing directory = %v, want nil", got)
	}
}

// TestDecodeKeepsWhatItCannotDecode checks that a history line with an escape
// this encoding does not define, or a backslash with nothing after it, comes
// back with those characters as they were written.
func TestDecodeKeepsWhatItCannotDecode(t *testing.T) {
	t.Parallel()

	for _, line := range []string{`a\qb`, `ab\`} {
		if got, ok := decodeHistoryLine(line); !ok || got != line {
			t.Errorf("decodeHistoryLine(%q) = %q, %v; want it unchanged", line, got, ok)
		}
	}
}

// TestSearchSelectionWithNoMatches checks that moving the reverse-search
// selection with nothing to select leaves it where it is.
func TestSearchSelectionWithNoMatches(t *testing.T) {
	t.Parallel()

	if got := moveSearchSelection(3, 1, 0); got != 3 {
		t.Errorf("moveSearchSelection(3, 1, 0) = %d, want 3", got)
	}
}

// TestSessionsThatScroll drives the paths that only a long session reaches: a
// paste long enough to be drawn while it arrives, a menu scrolled down past
// its window and back up, a block taller than the terminal walked to its top,
// a Tab that is bound to nothing, and input held back for a cancelable Run.
func TestSessionsThatScroll(t *testing.T) {
	t.Parallel()

	suggest := func(Document) []Suggestion {
		suggestions := make([]Suggestion, 12)
		for i := range suggestions {
			suggestions[i] = Suggestion{Text: fmt.Sprintf("s%02d", i)}
		}
		return suggestions
	}
	pasted := strings.Repeat("x", 3*pasteDrawInterval)
	unboundTab := NewDefaultKeyMap()
	unboundTab.Bind('\t', ActionNone)

	tests := []struct {
		name   string
		script string
		size   [2]int
		opts   []Option
		ctx    func() (context.Context, context.CancelFunc)
		want   string
	}{
		{name: "long paste", script: "\x1b[200~" + pasted + "\x1b[201~\r", want: pasted},
		{
			name:   "menu down past its window and back",
			script: "s\t" + strings.Repeat("\x1b[B", 9) + strings.Repeat("\x1b[A", 9) + "\t\r",
			size:   [2]int{40, 5},
			opts:   []Option{WithCompleter(suggest)},
			want:   "s00",
		},
		{
			name:   "tall block walked to its top",
			script: "one\\\rtwo\\\rthree\\\rfour" + strings.Repeat("\x1b[A", 4) + "\r",
			size:   [2]int{20, 2},
			opts:   []Option{WithMultiline()},
			want:   "one\ntwo\nthree\nfour",
		},
		{name: "unbound tab", script: "a\tb\r", opts: []Option{WithKeyMap(unboundTab)}, want: "ab"},
		{
			name:   "held-back input under a cancelable context",
			script: "ab\x1bO\r",
			ctx:    func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			want:   "ab",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := newMockTerminal(tt.script)
			if tt.size != [2]int{} {
				mock.terminalSize = tt.size
			}
			p := newTestPrompt(mock, tt.opts...)
			ctx, cancel := context.Background(), context.CancelFunc(func() {})
			if tt.ctx != nil {
				ctx, cancel = tt.ctx()
			}
			defer cancel()
			got, err := p.Run(ctx)
			if err != nil || got != tt.want {
				t.Fatalf("Run = %q, %v; want %q, nil", got, err, tt.want)
			}
		})
	}
}

// TestInputEndingAfterEscape checks that input running out right after ESC,
// with the sequence it began unfinished, ends the session as the end of input.
func TestInputEndingAfterEscape(t *testing.T) {
	t.Parallel()

	for _, script := range []string{"a\x1b", "a\x1b[1"} {
		p := newTestPrompt(newMockTerminal(script))
		if _, err := p.Run(context.Background()); !errors.Is(err, ErrEOF) {
			t.Errorf("Run over %q = %v, want ErrEOF", script, err)
		}
	}
}
