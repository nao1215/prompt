package prompt

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

// TestFuzzyCompleterSpanFollowsTheTextItMatched covers a Document built by hand
// rather than by the prompt. TextBeforeCursor answers a position outside the
// text with the whole text, so the span the suggestion names has to say the same
// thing: a span ending at zero would have inserted the candidate in front of
// what it was matched against instead of replacing it.
func TestFuzzyCompleterSpanFollowsTheTextItMatched(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		candidates []string
		doc        Document
		wantEnd    int
	}{
		{name: "the cursor inside the text", candidates: []string{"select"}, doc: Document{Text: "select", CursorPosition: 3}, wantEnd: 3},
		{name: "the cursor at the end", candidates: []string{"select"}, doc: Document{Text: "select", CursorPosition: 6}, wantEnd: 6},
		{name: "a negative cursor", candidates: []string{"select"}, doc: Document{Text: "select", CursorPosition: -1}, wantEnd: 6},
		{name: "a cursor past the end", candidates: []string{"select"}, doc: Document{Text: "select", CursorPosition: 99}, wantEnd: 6},
		{name: "counted in runes", candidates: []string{"選択する"}, doc: Document{Text: "選択", CursorPosition: 99}, wantEnd: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			suggestions := NewFuzzyCompleter(tt.candidates)(tt.doc)
			if len(suggestions) == 0 {
				t.Fatalf("completer(%+v) returned nothing", tt.doc)
			}
			for _, s := range suggestions {
				if s.Replace == nil {
					t.Fatalf("suggestion %q names no span, so the prompt filters it against the word before the cursor", s.Text)
				}
				if s.Replace.Start != 0 || s.Replace.End != tt.wantEnd {
					t.Errorf("suggestion %q replaces %+v, want {Start:0 End:%d}", s.Text, *s.Replace, tt.wantEnd)
				}
			}
		})
	}
}

// TestFileCompleterCompletesTheWordBeforeTheCursor asks the completer for the
// paths a line ends in, with something in front of it. It read everything to the
// left of the cursor and handed the whole of it to the path walk, so a line that
// held anything but the path -- a command, which is what a shell line starts
// with -- named a directory that does not exist and completed nothing.
func TestFileCompleterCompletesTheWordBeforeTheCursor(t *testing.T) {
	// Not parallel: one of the cases is a relative path, which is answered
	// against the working directory.
	dir := t.TempDir()
	for _, name := range []string{"alpha.txt", "beta.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatalf("writing the fixture: %v", err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "gamma"), 0o750); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), nil, 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	// A relative path is completed against the working directory, so the test
	// works from the one it filled.
	t.Chdir(dir)

	completer := NewFileCompleter()
	alpha := filepath.Join(dir, "alpha.txt")

	tests := map[string]struct {
		text string
		want []string
	}{
		"a path alone": {
			text: filepath.Join(dir, "al"),
			want: []string{alpha},
		},
		"a path after a command": {
			text: "cat " + filepath.Join(dir, "al"),
			want: []string{alpha},
		},
		"a path after a command and a flag": {
			text: "cat -n " + filepath.Join(dir, "al"),
			want: []string{alpha},
		},
		"a directory, which is listed with the separator it was written with": {
			text: "cd " + filepath.Join(dir, "ga"),
			want: []string{filepath.Join(dir, "gamma") + string(filepath.Separator)},
		},
		"a path written with a leading ./": {
			text: "cat ./al",
			want: []string{"./alpha.txt"},
		},
		"a path written with a doubled separator": {
			text: "cat " + dir + "//al",
			want: []string{dir + "//alpha.txt"},
		},
		"a name beginning with a dot, once the word does": {
			text: "cat " + filepath.Join(dir, ".hid"),
			want: []string{filepath.Join(dir, ".hidden")},
		},
		"everything in a directory named in full": {
			// Written with a forward slash, which every platform takes, so
			// that is the separator the directory comes back with.
			text: "ls " + dir + "/",
			want: []string{dir + "/alpha.txt", dir + "/beta.txt", dir + "/gamma/"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := completer(Document{Text: tt.text, CursorPosition: len([]rune(tt.text))})
			texts := make([]string, 0, len(got))
			for _, suggestion := range got {
				texts = append(texts, suggestion.Text)
			}
			if !slices.Equal(texts, tt.want) {
				t.Errorf("completing %q offered %q, want %q", tt.text, texts, tt.want)
			}
		})
	}
}

// TestFileCompleterListsTheDirectoryAfterASpace pins the empty word. A cursor
// that follows a space is not typing a path yet, and what belongs there is the
// directory's contents rather than nothing.
func TestFileCompleterListsTheDirectoryAfterASpace(t *testing.T) {
	// Not parallel: it works from a directory of its own, which is a property of
	// the process rather than of the test.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), nil, 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	t.Chdir(dir)

	got := NewFileCompleter()(Document{Text: "cat ", CursorPosition: 4})
	if len(got) != 1 || got[0].Text != "alpha.txt" {
		t.Errorf("completing %q offered %v, want the contents of the directory", "cat ", got)
	}
}

// TestFileCompleterCompletesInsideAPrompt runs the helper the way an application
// does: handed to WithCompleter, with Tab pressed on a line that holds a command
// in front of the path.
//
// It is the chain rather than the helper. A completer that does not name the
// span its candidate replaces has that candidate measured against the word
// before the cursor, both to decide whether to offer it and to decide what to
// insert, so a candidate carrying anything the word does not start with is
// dropped without a word to the user.
func TestFileCompleterCompletesInsideAPrompt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), nil, 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	var out bytes.Buffer
	terminal := &sizedMockTerminal{width: 120, height: 24}
	terminal.mockTerminal = *newMockTerminal("cat " + filepath.Join(dir, "al") + "\t\r")
	p := newTestPromptOn(terminal, WithCompleter(NewFileCompleter()))
	p.output = &out
	p.renderer = newRenderer(&out, ThemeDefault, terminal)

	line, err := p.Run()
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if want := "cat " + filepath.Join(dir, "alpha.txt"); line != want {
		t.Errorf("Tab completed to %q, want %q", line, want)
	}
}
