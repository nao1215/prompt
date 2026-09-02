package prompt

import (
	"context"
	"strings"
	"testing"
)

// TestAutoIndentOpensEachNewLine drives the read loop: a newline the user
// causes is followed by whatever the indenter returns, so a continuation line
// starts where the last one did rather than back at the margin.
func TestAutoIndentOpensEachNewLine(t *testing.T) {
	t.Parallel()

	// Two spaces per line, cumulative, so the test can tell which newline each
	// indent came from rather than only that some indent happened.
	indent := func(before string) string {
		return strings.Repeat(" ", 2*(strings.Count(before, "\n")+1))
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "a line continued because the input is incomplete opens indented",
			input: "a\rb\r;\r",
			want:  "a\n  b\n    ;",
		},
		{
			name:  "the indent is not added to a line that submits",
			input: ";\r",
			want:  ";",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPrompt(newMockTerminal(tt.input),
				WithMultiline(),
				WithIsComplete(func(in string) bool { return strings.HasSuffix(in, ";") }),
				WithAutoIndent(indent),
			)

			got, err := p.Run(context.Background())
			if err != nil {
				t.Fatalf("RunWithContext: %v", err)
			}
			if got != tt.want {
				t.Errorf("submitted input = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAutoIndentSeesTheTextBeforeTheNewLine pins what the indenter is given: the
// input up to where the line breaks, and not the newline itself. An indenter
// decides from the line it is continuing, so it has to be handed that line.
func TestAutoIndentSeesTheTextBeforeTheNewLine(t *testing.T) {
	t.Parallel()

	var seen []string
	p := newTestPrompt(newMockTerminal("ab\rcd\r;\r"),
		WithMultiline(),
		WithIsComplete(func(in string) bool { return strings.HasSuffix(in, ";") }),
		WithAutoIndent(func(before string) string {
			seen = append(seen, before)
			return ""
		}),
	)

	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("RunWithContext: %v", err)
	}

	want := []string{"ab", "ab\ncd"}
	if len(seen) != len(want) {
		t.Fatalf("the indenter was called %d time(s) with %q, want %d", len(seen), seen, len(want))
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("call %d was given %q, want %q", i, seen[i], want[i])
		}
	}
}

// TestAutoIndentSplitsALineAtTheCursor covers Enter pressed in the middle of a
// line: the indenter is given what is behind the cursor, and the text ahead of
// it moves to the new line after the indent.
func TestAutoIndentSplitsALineAtTheCursor(t *testing.T) {
	t.Parallel()

	var seen string
	// "abcd", then two left arrows, then Enter, then ";" and Enter to submit.
	// Completeness is "holds a ;" rather than "ends with one", because the ";"
	// is typed into the middle of the line here.
	p := newTestPrompt(newMockTerminal("abcd\x1b[D\x1b[D\r;\r"),
		WithMultiline(),
		WithIsComplete(func(in string) bool { return strings.Contains(in, ";") }),
		WithAutoIndent(func(before string) string {
			seen = before
			return ">>"
		}),
	)

	got, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("RunWithContext: %v", err)
	}
	if want := "ab"; seen != want {
		t.Errorf("the indenter was given %q, want %q", seen, want)
	}
	if want := "ab\n>>;cd"; got != want {
		t.Errorf("submitted input = %q, want %q", got, want)
	}
}

// TestWithoutAutoIndentNothingIsInserted is the other half of the contract: a
// prompt built without the option behaves exactly as it did before it existed.
func TestWithoutAutoIndentNothingIsInserted(t *testing.T) {
	t.Parallel()

	p := newTestPrompt(newMockTerminal("a\rb\r;\r"),
		WithMultiline(),
		WithIsComplete(func(in string) bool { return strings.HasSuffix(in, ";") }),
	)

	got, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("RunWithContext: %v", err)
	}
	if want := "a\nb\n;"; got != want {
		t.Errorf("submitted input = %q, want %q", got, want)
	}
}

// TestAutoIndentAppliesToEveryWayALineBreaks covers the other two: the explicit
// newline key, and the trailing backslash a line can be continued with. A
// continuation is a continuation however it was asked for.
func TestAutoIndentAppliesToEveryWayALineBreaks(t *testing.T) {
	t.Parallel()

	t.Run("a bound newline key indents the line it opens", func(t *testing.T) {
		t.Parallel()

		keyMap := NewDefaultKeyMap()
		keyMap.Bind('\x0e', ActionNewLine) // Ctrl+N, so the mock can send it

		p := newTestPrompt(newMockTerminal("a\x0eb\r"),
			WithMultiline(),
			WithKeyMap(keyMap),
			WithAutoIndent(func(string) string { return "--" }),
		)

		got, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext: %v", err)
		}
		if want := "a\n--b"; got != want {
			t.Errorf("submitted input = %q, want %q", got, want)
		}
	})

	t.Run("a trailing backslash indents the line it opens", func(t *testing.T) {
		t.Parallel()

		p := newTestPrompt(newMockTerminal("a\\\rb\r"),
			WithMultiline(),
			WithAutoIndent(func(string) string { return "--" }),
		)

		got, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext: %v", err)
		}
		if want := "a\n--b"; got != want {
			t.Errorf("submitted input = %q, want %q", got, want)
		}
	})
}
