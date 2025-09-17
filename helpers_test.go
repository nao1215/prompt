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
	search := NewHistorySearcher(history)

	if search == nil {
		t.Error("NewHistorySearcher() returned nil")
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
