package prompt

import (
	"fmt"
	"os"
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

// NewFileCompleter returns a completer that offers the files and directories the
// word before the cursor names the start of. A directory is offered with a
// trailing separator -- the one the path was written with -- so the next Tab
// continues inside it, and a name beginning with a dot is offered only once the
// word begins with one.
//
// It completes the word rather than the line, so it can be used on a line that
// holds more than the path -- a command and its arguments, which is what a shell
// line is. A cursor that follows a space is not in the middle of a name, and
// what is offered there is the directory's contents.
//
// What it does not do: expand "~", understand quoting or backslash escapes, or
// know which argument of which command is being completed. A path holding a
// space is therefore two words to it. A completer that needs any of that is a
// function of your own -- example/shell has one -- and Document says where the
// cursor is and what the line holds.
func NewFileCompleter() func(Document) []Suggestion {
	return func(d Document) []Suggestion {
		// The word before the cursor, not the line before it. A line is a
		// command and its arguments, and handing the whole of it to the path
		// walk named a directory nothing is called: completion after "cat /et"
		// offered nothing at all. The word is empty when the cursor follows a
		// space, which is where the directory's contents belong rather than
		// nothing, and completeFilePath reads an empty path as the current
		// directory.
		//
		// It is also the word the prompt measures a candidate against when the
		// completer does not name the span it replaces, so what is offered here
		// and what is applied there are the same string.
		return completeFilePath(d.GetWordBeforeCursor())
	}
}

// completeFilePath returns the files and directories whose names start with the
// last component of path, offered as the path the user typed with that component
// completed.
//
// The candidate is built from what was typed rather than from the parts the path
// was taken apart into. filepath.Join cleans what it builds -- "./" goes, a
// doubled separator goes -- and a candidate that comes back tidier than the word
// it completes does not start with that word, which is what the prompt measures
// a suggestion against when the completer does not name the span it replaces. It
// was then dropped before it reached the screen, so the key appeared to do
// nothing.
func completeFilePath(path string) []Suggestion {
	// What was typed in front of the name being completed, kept exactly as it
	// was, and the name itself. Nothing typed at all is the current directory
	// with no name to filter by: reading that as the path "." made "." the name
	// as well, and only a dot file starts with one, so the case that should list
	// everything listed the dot files alone.
	cut := strings.LastIndexAny(path, `/\`)
	prefix, base := path[:cut+1], path[cut+1:]

	dir := prefix
	if dir == "" {
		dir = "."
	}

	// A directory is offered with the separator the path was written with, so a
	// path typed the way Windows writes them stays one. A path with no separator
	// in it yet has no style to follow, and both platforms take the slash.
	separator := "/"
	if cut >= 0 {
		separator = path[cut : cut+1]
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	suggestions := make([]Suggestion, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()

		// A name beginning with a dot is offered only once the word begins with
		// one, the way a shell hides them until they are asked for.
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		if base != "" && !strings.HasPrefix(name, base) {
			continue
		}

		// A directory is offered with the separator after it, so the next Tab
		// continues inside it rather than starting again on the same word.
		description := "file"
		if entry.IsDir() {
			name += separator
			description = "directory"
		}

		suggestions = append(suggestions, Suggestion{
			Text:        prefix + name,
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
// list given here, matched against the input before the cursor. Nor does it stop
// at a line break -- in a multiline entry the input before the cursor takes in
// the earlier lines, and a candidate that matches it replaces them. A completer
// that needs to answer differently in different parts of an entry is a function
// of your own, and Document says where the cursor is.
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
	// so a position outside the text is answered the way TextBeforeCursor answers
	// it -- with the whole text -- rather than with a span that would insert the
	// candidate in front of what it matched.
	end := d.CursorPosition
	if runes := len([]rune(d.Text)); end < 0 || end > runes {
		end = runes
	}
	replace := &Range{Start: 0, End: end}

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
