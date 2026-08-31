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
		// A query whose characters are out of order still scores for the ones
		// found before the walk runs out, which is what it does in ASCII too.
		{name: "out of order scores partially", input: "語日", candidate: "日本語", want: true},
		{name: "out of order scores partially in ascii", input: "ca", candidate: "abc", want: true},
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
