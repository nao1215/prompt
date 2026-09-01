package prompt

import (
	"fmt"
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
	//
	// A match is the whole input or nothing. The walk used to return whatever it
	// had accumulated when the candidate ran out, which is how far the input got
	// rather than whether it arrived, and every caller reads a score above zero
	// as a match: an entry sharing the first character of a six-character query
	// was offered as one.
	score := 0
	candidateRunes := []rune(searchCandidate)
	candidateIdx := 0

	for _, inputChar := range searchInput {
		found := false
		for candidateIdx < len(candidateRunes) {
			if candidateRunes[candidateIdx] == inputChar {
				score += 10
				candidateIdx++
				found = true
				break
			}
			candidateIdx++
		}
		if !found {
			return 0
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

// fuzzyMatcher provides reusable fuzzy matching logic for completions and history search
type fuzzyMatcher struct {
	items []string
}

// NewFuzzyCompleter creates a new fuzzy completer with the given candidates.
//
// It matches the input before the cursor, not the word before it, and ignores
// case. A candidate matches when the input is a prefix of it, a substring of it,
// or a subsequence of it -- its characters in order, with anything between, so
// "dckrbld" finds "docker build" -- and nothing else matches. Candidates are
// ordered by how they matched, an exact match first and a subsequence last. An
// empty input matches every candidate.
//
// Each suggestion replaces the input before the cursor, which is what was
// matched against. That is why a candidate holding a space completes: the prompt
// applies a suggestion that names its span literally and does not filter it
// against the word before the cursor. See Suggestion.Replace.
//
// Each suggestion's Description holds the score it matched with, which the menu
// draws beside the candidate.
//
// It does not read the filesystem, know anything about the shape of a command,
// or narrow the list by where in the line the cursor is: the candidates are the
// list given here, matched against the input before the cursor. A completer that
// needs to answer differently in different parts of a line is a function of your
// own, and Document says where the cursor is.
//
// Example:
//
//	candidates := []string{
//		"git status", "git commit", "git push", "git pull",
//		"docker run", "docker build", "docker ps",
//		"kubectl get", "kubectl apply", "kubectl delete",
//	}
//
//	config := prompt.Config{
//		Prefix: "$ ",
//		Completer: prompt.NewFuzzyCompleter(candidates),
//	}
//
//	p, _ := prompt.New(config)
//	defer p.Close()
//	result, _ := p.Run()
func NewFuzzyCompleter(candidates []string) func(Document) []Suggestion {
	fm := &fuzzyMatcher{
		items: candidates,
	}
	return fm.completionFunc
}

// completionFunc returns fuzzy-matched suggestions for the given document context.
//
// Every suggestion names the span it stands for: the input before the cursor,
// which is what was matched against. Without it the prompt keeps only the
// candidates that start with the word before the cursor, case-sensitively, which
// does none of the three things that set this completer apart from a prefix
// completer -- match the input rather than the word, ignore case, match a
// subsequence -- so it threw this completer's answer away.
func (f *fuzzyMatcher) completionFunc(d Document) []Suggestion {
	// The span is in runes, because that is what the prompt's buffer is indexed
	// in and what CursorPosition counts. A Document is whatever the caller built,
	// so the start is clamped rather than trusted; the prompt clamps the span
	// again when it applies it.
	replace := &Range{Start: 0, End: max(d.CursorPosition, 0)}

	input := d.TextBeforeCursor()
	if input == "" {
		// Return all items if no input
		suggestions := make([]Suggestion, len(f.items))
		for i, item := range f.items {
			suggestions[i] = Suggestion{
				Text:        item,
				Description: "",
				Replace:     replace,
			}
		}
		return suggestions
	}

	matches := f.fuzzySearch(input)
	// Convert to suggestions
	suggestions := make([]Suggestion, len(matches))
	for i, match := range matches {
		suggestions[i] = Suggestion{
			Text:        match.text,
			Description: fmt.Sprintf("score: %d", match.score),
			Replace:     replace,
		}
	}
	return suggestions
}

type fuzzyMatch struct {
	text  string
	score int
}

// fuzzySearch performs fuzzy matching against items and returns sorted matches
func (f *fuzzyMatcher) fuzzySearch(query string) []fuzzyMatch {
	if query == "" {
		return nil
	}

	var matches []fuzzyMatch
	queryLower := strings.ToLower(query)

	for _, item := range f.items {
		if score := calculateFuzzyScore(queryLower, strings.ToLower(item)); score > 0 {
			matches = append(matches, fuzzyMatch{
				text:  item,
				score: score,
			})
		}
	}

	// Sort by score (descending)
	for i := range len(matches) - 1 {
		for j := i + 1; j < len(matches); j++ {
			if matches[i].score < matches[j].score {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	return matches
}

// searchFunc returns items that match the query using fuzzy matching
func (f *fuzzyMatcher) searchFunc(query string) []string {
	if query == "" {
		return f.items
	}

	matches := f.fuzzySearch(query)
	// Convert to string slice
	results := make([]string, len(matches))
	for i, match := range matches {
		results[i] = match.text
	}
	return results
}
