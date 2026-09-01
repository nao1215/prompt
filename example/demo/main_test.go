package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/prompt"
)

// The animation in the README is this program being driven by demo.tape, so
// what it appears to show is only true while these hold. They are also what
// makes the program worth reading as an example: a completer that decides from
// context, a highlighter that colors runs without moving anything, and a
// multiline rule that knows when a statement has ended.

func documentAt(text string) prompt.Document {
	return prompt.Document{Text: text, CursorPosition: len([]rune(text))}
}

func suggestionTexts(suggestions []prompt.Suggestion) []string {
	out := make([]string, 0, len(suggestions))
	for _, s := range suggestions {
		out = append(out, s.Text)
	}
	return out
}

func TestCompleterAnswersFromContext(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		before   string
		contains string
		absent   string
	}{
		"an empty line offers keywords":        {"", "select", "users"},
		"a partial keyword still offers them":  {"sel", "select", "users"},
		"after FROM comes a table":             {"select * from ", "users", "select"},
		"after a table name comes a keyword":   {"select * from users ", "where", "users"},
		"after SELECT comes a column":          {"select ", "email", "select"},
		"after WHERE comes a column":           {"select * from users where ", "created_at", "users"},
		"mid-word after FROM still has tables": {"select * from us", "users", "select"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := suggestionTexts(completer(documentAt(tt.before)))
			if !slices.Contains(got, tt.contains) {
				t.Errorf("after %q the completer offers %v, which does not include %q", tt.before, got, tt.contains)
			}
			if slices.Contains(got, tt.absent) {
				t.Errorf("after %q the completer offers %q, which belongs to another position", tt.before, tt.absent)
			}
		})
	}
}

func TestEverySuggestionCarriesADescription(t *testing.T) {
	t.Parallel()

	// The menu's second column is what tells the user which candidate they
	// want, and the animation shows it. A suggestion without one draws a blank.
	sets := map[string][]prompt.Suggestion{
		"keywords": keywords,
		"tables":   tableSuggestions(),
		"columns":  columnSuggestions(),
	}
	for name, set := range sets {
		for _, suggestion := range set {
			if suggestion.Description == "" {
				t.Errorf("the %s set offers %q with no description", name, suggestion.Text)
			}
		}
	}
}

func TestHighlightColorsKeywordsAndStrings(t *testing.T) {
	t.Parallel()

	const input = "select name from users where name = 'alice';"
	spans := highlight(input)

	runes := []rune(input)
	colored := make(map[string]prompt.Color)
	for _, span := range spans {
		if span.Start < 0 || span.End > len(runes) || span.Start >= span.End {
			t.Fatalf("the span %d..%d is not inside a line of %d runes", span.Start, span.End, len(runes))
		}
		colored[string(runes[span.Start:span.End])] = span.Color
	}

	for _, keyword := range []string{"select", "from", "where"} {
		if colored[keyword] != colorKeyword {
			t.Errorf("%q is not colored as a keyword", keyword)
		}
	}
	if colored["'alice'"] != colorString {
		t.Errorf("the quoted string is not colored; the spans cover %v", colored)
	}
	if _, ok := colored["name"]; ok {
		t.Error("a column name is colored as though it were a keyword")
	}
}

func TestHighlightSpansDoNotOverlap(t *testing.T) {
	t.Parallel()

	// Spans are rune offsets into the whole input and the renderer walks them in
	// order, so one that starts before the last one ended would draw a run
	// twice.
	for _, input := range []string{
		"select * from users;",
		"select 'a' from 'b' where 'unclosed",
		"'opens", "''", "", "     ", "select",
	} {
		previousEnd := 0
		for _, span := range highlight(input) {
			if span.Start < previousEnd {
				t.Errorf("on %q the span at %d starts before the one before it ended at %d", input, span.Start, previousEnd)
			}
			previousEnd = span.End
		}
		if previousEnd > len([]rune(input)) {
			t.Errorf("on %q a span reaches %d, past the end of the input", input, previousEnd)
		}
	}
}

func TestStatementIsCompleteWaitsForTheSemicolon(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"select * from users;":    true,
		"select * from users":     false,
		"select *":                false,
		"select * from users;   ": true,
		`\q`:                      true,
		"":                        true,
		"   ":                     true,
	}
	for input, want := range tests {
		if got := statementIsComplete(input); got != want {
			t.Errorf("statementIsComplete(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestExecuteAnswersTheStatementsTheDemoRuns(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		statement string
		contains  []string
	}{
		"the statement the animation types first": {
			statement: "select name, email from users;",
			contains:  []string{"name", "email", "alice@example.com", "(3 rows)"},
		},
		"the multiline statement it types next": {
			statement: "select total, placed_at\n  from orders\n  where total > '20';",
			contains:  []string{"total", "placed_at", "42.00", "(2 rows)"},
		},
		"every column": {
			statement: "select * from users;",
			contains:  []string{"created_at", "carol", "(3 rows)"},
		},
		"a table that is not there": {
			statement: "select * from nowhere;",
			contains:  []string{"no table named nowhere"},
		},
		"a statement the demo does not implement": {
			statement: "delete from users;",
			contains:  []string{"only SELECT"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := execute(tt.statement)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("executing %q answered %q, which does not contain %q", tt.statement, got, want)
				}
			}
		})
	}
}
