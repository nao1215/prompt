package prompt

import (
	"reflect"
	"testing"
)

// Test only public APIs - internal functions are tested indirectly through public APIs

func TestNewFileCompleter(t *testing.T) {
	completer := NewFileCompleter()
	if completer == nil {
		t.Error("NewFileCompleter() returned nil")
	}

	// Test that the completer function works
	doc := Document{Text: ".", CursorPosition: 1}
	suggestions := completer(doc)
	// Should return at least something for current directory
	if suggestions == nil {
		t.Error("File completer returned nil suggestions")
	}
}

func TestNewFuzzyCompleter(t *testing.T) {
	candidates := []string{"apple", "banana", "cherry"}
	completer := NewFuzzyCompleter(candidates)

	if completer == nil {
		t.Error("NewFuzzyCompleter() returned nil")
	}

	// Test empty input returns all candidates
	doc := Document{Text: "", CursorPosition: 0}
	suggestions := completer(doc)

	if len(suggestions) != len(candidates) {
		t.Errorf("Expected %d suggestions for empty input, got %d", len(candidates), len(suggestions))
	}

	// Test prefix matching
	doc = Document{Text: "ap", CursorPosition: 2}
	suggestions = completer(doc)

	if len(suggestions) == 0 {
		t.Error("Expected at least one suggestion for 'ap'")
	}

	// Should include "apple"
	found := false
	for _, s := range suggestions {
		if s.Text == "apple" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'apple' in suggestions for 'ap'")
	}
}

func TestNewHistorySearcher(t *testing.T) {
	history := []string{"git status", "git commit", "ls -la"}
	search := newHistorySearcher(history)

	if search == nil {
		t.Error("newHistorySearcher() returned nil")
	}

	// Test empty query returns all history
	results := search("")
	if !reflect.DeepEqual(results, history) {
		t.Errorf("Expected %v for empty query, got %v", history, results)
	}

	// Test searching for "git"
	results = search("git")
	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'git', got %d", len(results))
	}
}

// TestFuzzyScoreWalksTheCandidateByRune covers matching outside ASCII. The
// candidate was walked a byte at a time and each byte compared to a rune of the
// input, so one byte of a UTF-8 sequence was tested against the character it is
// part of and never matched: Ctrl+R and NewFuzzyCompleter could not find a
// command written in Japanese, or a word with an accent, by its characters.
func TestFuzzyScoreWalksTheCandidateByRune(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		candidate string
		want      bool // whether the characters are all there, in order
	}{
		{name: "ascii scattered", input: "st", candidate: "select * from t", want: true},
		{name: "japanese scattered", input: "日語", candidate: "日本語テキスト", want: true},
		{name: "japanese across a word", input: "名テ", candidate: "名前テーブル", want: true},
		{name: "an accented letter", input: "éo", candidate: "école", want: true},
		{name: "a character that is not there scores nothing", input: "猫", candidate: "日本語", want: false},
		{name: "a character that is not there scores nothing in ascii", input: "z", candidate: "abc", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := calculateFuzzyScore(tt.input, tt.candidate)
			if (got > 0) != tt.want {
				t.Errorf("calculateFuzzyScore(%q, %q) = %d, want a score %s zero", tt.input, tt.candidate, got, map[bool]string{true: "above", false: "of"}[tt.want])
			}
		})
	}
}

// TestFuzzyScoreKeepsItsAsciiScores pins that widening the walk changed nothing
// for a query and a candidate that were already reachable.
func TestFuzzyScoreKeepsItsAsciiScores(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input     string
		candidate string
		want      int
	}{
		{input: "", candidate: "anything", want: 1},
		{input: "abc", candidate: "", want: 0},
		{input: "abc", candidate: "abc", want: 1000},
		{input: "ab", candidate: "abc", want: 820},
		{input: "bc", candidate: "abcd", want: 510},
		{input: "ac", candidate: "abc", want: 20},
	}

	for _, tt := range tests {
		t.Run(tt.input+"/"+tt.candidate, func(t *testing.T) {
			t.Parallel()

			if got := calculateFuzzyScore(tt.input, tt.candidate); got != tt.want {
				t.Errorf("calculateFuzzyScore(%q, %q) = %d, want %d", tt.input, tt.candidate, got, tt.want)
			}
		})
	}
}

// TestHistorySearchFindsANonAsciiEntry is the same measurement seen through
// Ctrl+R, which ranks the history with that score.
func TestHistorySearchFindsANonAsciiEntry(t *testing.T) {
	t.Parallel()

	search := newHistorySearcher([]string{"日本語テキスト", "select * from t", "école normale"})
	for _, query := range []string{"日語", "st", "éo"} {
		if got := search(query); len(got) == 0 {
			t.Errorf("searching %q found nothing", query)
		}
	}
}

func TestFuzzyCompleter(t *testing.T) {
	t.Parallel()

	candidates := []string{
		"git status",
		"git commit",
		"git push",
		"docker build",
		"docker run",
		"kubectl get",
		"kubectl apply",
	}

	completer := NewFuzzyCompleter(candidates)

	tests := []struct {
		name     string
		input    string
		expected int // expected number of results
	}{
		{
			name:     "empty input returns all",
			input:    "",
			expected: 7,
		},
		{
			name:     "git prefix",
			input:    "git",
			expected: 3, // the three candidates starting with it; nothing else holds g-i-t in order
		},
		{
			name:     "docker prefix",
			input:    "docker",
			expected: 2,
		},
		{
			name:     "fuzzy match",
			input:    "gst",
			expected: 1, // only "git status" holds g, then s, then t
		},
		{
			name:     "no matches",
			input:    "xyz",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			doc := Document{Text: tt.input, CursorPosition: len(tt.input)}
			suggestions := completer(doc)
			if len(suggestions) != tt.expected {
				t.Errorf("Complete(%q) returned %d suggestions, want %d",
					tt.input, len(suggestions), tt.expected)
			}

			// Verify all suggestions contain the text field
			for _, s := range suggestions {
				if s.Text == "" {
					t.Error("Suggestion with empty Text field")
				}
			}
		})
	}
}

func TestFuzzyScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		candidate string
		minScore  int
	}{
		{
			name:      "exact match",
			input:     "git",
			candidate: "git",
			minScore:  1000,
		},
		{
			name:      "prefix match",
			input:     "git",
			candidate: "git status",
			minScore:  800,
		},
		{
			name:      "contains match",
			input:     "status",
			candidate: "git status",
			minScore:  500,
		},
		{
			name:      "fuzzy match",
			input:     "gst",
			candidate: "git status",
			minScore:  10,
		},
		{
			name:      "no match",
			input:     "xyz",
			candidate: "git status",
			minScore:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			score := calculateFuzzyScore(tt.input, tt.candidate)
			if tt.minScore == 0 {
				if score != 0 {
					t.Errorf("Expected no match (score 0), got %d", score)
				}
			} else {
				if score < tt.minScore {
					t.Errorf("Score %d is less than expected minimum %d", score, tt.minScore)
				}
			}
		})
	}
}

func TestHistorySearcher(t *testing.T) {
	t.Parallel()

	history := []string{
		"git status",
		"git commit -m 'initial'",
		"docker build .",
		"kubectl get pods",
		"git push origin main",
	}

	searcher := newHistorySearcher(history)

	tests := []struct {
		name     string
		query    string
		expected int
	}{
		{
			name:     "empty query returns all",
			query:    "",
			expected: 5,
		},
		{
			name:     "git query",
			query:    "git",
			expected: 3, // the three entries starting with it; "kubectl get pods" has no i after its g
		},
		{
			name:     "docker query",
			query:    "docker",
			expected: 1, // the one entry starting with it
		},
		{
			name:     "no matches",
			query:    "xyz",
			expected: 0,
		},
		{
			name:     "fuzzy match",
			query:    "gst",
			expected: 1, // only "git status" holds g, then s, then t
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			results := searcher(tt.query)
			if len(results) != tt.expected {
				t.Errorf("Search(%q) returned %d results, want %d",
					tt.query, len(results), tt.expected)
			}
		})
	}
}

// TestFuzzyScoreRequiresEveryCharacter pins the subsequence pass to an
// all-or-nothing answer. It added ten per character it found and returned
// whatever it had when the candidate ran out, so a query that got one character
// in scored above zero, and every caller reads a score above zero as a match:
// reverse search for "sql" listed an entry holding no "q".
func TestFuzzyScoreRequiresEveryCharacter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		candidate string
		want      bool
	}{
		{name: "every character, in order", input: "gst", candidate: "git status", want: true},
		{name: "the first character only", input: "sql", candidate: "select * from users", want: false},
		{name: "some of the characters", input: "git st", candidate: "kubectl get", want: false},
		{name: "in the wrong order", input: "ts", candidate: "set", want: false},
		{name: "out of order, outside ascii", input: "語日", candidate: "日本語", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := calculateFuzzyScore(tt.input, tt.candidate) > 0
			if got != tt.want {
				t.Errorf("calculateFuzzyScore(%q, %q) > 0 = %v, want %v", tt.input, tt.candidate, got, tt.want)
			}
		})
	}
}

// TestFuzzySearchListsOnlyWhatMatches is the same rule where the user meets it:
// reverse search and NewFuzzyCompleter both read this list.
func TestFuzzySearchListsOnlyWhatMatches(t *testing.T) {
	t.Parallel()

	matcher := &fuzzyMatcher{items: []string{
		"select * from users",
		"drop table t",
		"insert into t values (1)",
	}}

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "not a subsequence of any entry", query: "sql", want: nil},
		{name: "a prefix", query: "select", want: []string{"select * from users"}},
		{name: "a substring", query: "table", want: []string{"drop table t"}},
		{name: "a real subsequence", query: "dtt", want: []string{"drop table t"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := matcher.searchFunc(tt.query)
			if len(got) != len(tt.want) {
				t.Fatalf("searchFunc(%q) = %q, want %q", tt.query, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("searchFunc(%q) = %q, want %q", tt.query, got, tt.want)
					break
				}
			}
		})
	}
}
