package prompt

import (
	"os"
	"path/filepath"
	"strings"
)

// calculateFuzzyScore calculates a fuzzy matching score between input and
// candidate. Returns 0 if no match, higher scores for better matches.
//
// It compares what it is given. Case is the caller's to decide, and the one
// caller that wants it ignored lowers both strings before asking.
func calculateFuzzyScore(input, candidate string) int {
	if input == "" {
		return 1
	}
	if candidate == "" {
		return 0
	}

	searchInput := input
	searchCandidate := candidate

	// Exact match gets highest score
	if searchInput == searchCandidate {
		return 1000
	}

	// Prefix match gets high score
	if strings.HasPrefix(searchCandidate, searchInput) {
		return 800 + len(searchInput)*10
	}

	// Contains match gets medium score
	if strings.Contains(searchCandidate, searchInput) {
		return 500 + len(searchInput)*5
	}

	// Character-by-character fuzzy matching. The candidate is walked as runes
	// because the input is: ranging over a string yields characters and indexing
	// one yields bytes, so comparing the two tested a byte of a UTF-8 sequence
	// against the character it belongs to. Nothing outside ASCII could match,
	// and every candidate written in another alphabet scored zero here.
	score := 0
	candidateRunes := []rune(searchCandidate)
	candidateIdx := 0

	for _, inputChar := range searchInput {
		for candidateIdx < len(candidateRunes) {
			if candidateRunes[candidateIdx] == inputChar {
				score += 10
				candidateIdx++
				break
			}
			candidateIdx++
		}
		if candidateIdx >= len(candidateRunes) {
			break
		}
	}

	return score
}

// NewFileCompleter creates a completer that provides file and directory suggestions
func NewFileCompleter() func(Document) []Suggestion {
	return func(d Document) []Suggestion {
		text := d.TextBeforeCursor()
		return completeFilePath(text)
	}
}

// completeFilePath provides file and directory completion for the given path (internal)
func completeFilePath(path string) []Suggestion {
	// Handle empty path - start from current directory
	if path == "" {
		path = "."
	}

	// Extract directory and filename parts
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	// If path ends with separator, we're completing in that directory
	if strings.HasSuffix(path, "/") || strings.HasSuffix(path, "\\") {
		dir = path
		base = ""
	}

	// Handle relative paths
	if dir == "." && !strings.HasPrefix(path, "./") {
		dir = "."
	}

	// Read directory contents
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	suggestions := make([]Suggestion, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files unless explicitly requested
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}

		// Filter by prefix
		if base != "" && !strings.HasPrefix(name, base) {
			continue
		}

		// Build full path
		fullPath := filepath.Join(dir, name)
		if dir == "." && !strings.HasPrefix(path, "./") {
			fullPath = name
		}

		// Add trailing slash for directories
		description := "file"
		if entry.IsDir() {
			fullPath += "/"
			description = "directory"
		}

		suggestions = append(suggestions, Suggestion{
			Text:        fullPath,
			Description: description,
		})
	}

	return suggestions
}
