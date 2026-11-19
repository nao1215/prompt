package prompt

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// The tests in this file draw their inputs with rapid, which shrinks a failure
// to a small counterexample. Their names say which kind they are:
// TestProperty_ checks a rule every input has to keep, TestMetamorphic_ checks
// how the answer changes when the input is changed in a known way, and
// TestDifferential_ checks the answer against a second, independent way of
// computing it.

// editRunes are the characters the line-editing code treats specially, plus a
// few ordinary ones in and outside ASCII, so that a drawn line keeps meeting
// the cases that matter rather than mostly holding letters.
var editRunes = []rune{' ', '\t', '\n', '\\', 'a', 'b', 'G', 'é', '語', '🙂'}

func genText(t *rapid.T, label string) string {
	return rapid.StringOf(rapid.SampledFrom(editRunes)).Draw(t, label)
}

// genDocument draws a Document whose cursor is usually inside the text and
// sometimes outside it on either side, which a completer building its own
// Document can hand over.
func genDocument(t *rapid.T) Document {
	text := genText(t, "text")
	n := len([]rune(text))
	return Document{Text: text, CursorPosition: rapid.IntRange(-2, n+2).Draw(t, "cursor")}
}

func TestProperty_DocumentSplitsAtTheCursor(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		d := genDocument(t)
		before, after := d.TextBeforeCursor(), d.TextAfterCursor()
		if before+after != d.Text {
			t.Fatalf("TextBeforeCursor()+TextAfterCursor() = %q+%q, want %q", before, after, d.Text)
		}
		runes := []rune(d.Text)
		if d.CursorPosition < 0 || d.CursorPosition > len(runes) {
			// A position outside the text is answered with the whole text.
			if before != d.Text || after != "" {
				t.Fatalf("cursor %d outside %q: before %q, after %q", d.CursorPosition, d.Text, before, after)
			}
			return
		}
		if got := len([]rune(before)); got != d.CursorPosition {
			t.Fatalf("TextBeforeCursor() holds %d runes, want the cursor's %d", got, d.CursorPosition)
		}
	})
}

// TestDifferential_WordBeforeCursorIsTheLastField checks WordBeforeCursor
// against strings.FieldsFunc: the word is the last field of the text before the
// cursor, and nothing when that text ends in a separator.
func TestDifferential_WordBeforeCursorIsTheLastField(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		d := genDocument(t)
		before := d.TextBeforeCursor()
		want := ""
		if r := []rune(before); len(r) > 0 && !isWordSeparator(r[len(r)-1]) {
			fields := strings.FieldsFunc(before, isWordSeparator)
			want = fields[len(fields)-1]
		}
		if got := d.WordBeforeCursor(); got != want {
			t.Fatalf("WordBeforeCursor() of %q = %q, want %q", before, got, want)
		}
	})
}

// TestDifferential_EscapedWordAgreesWithoutBackslashes checks the two word
// functions against each other: they differ only in what a backslash does, so
// on a line holding none they have to give the same word.
func TestDifferential_EscapedWordAgreesWithoutBackslashes(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		d := genDocument(t)
		d.Text = strings.ReplaceAll(d.Text, `\`, "x")
		if plain, escaped := d.WordBeforeCursor(), d.WordBeforeCursorEscaped(); plain != escaped {
			t.Fatalf("on %q at %d: WordBeforeCursor() = %q, WordBeforeCursorEscaped() = %q", d.Text, d.CursorPosition, plain, escaped)
		}
	})
}

// TestDifferential_CurrentLineIsTheCursorsLine checks CurrentLine against
// splitting the text at its line breaks: the cursor's line is the one whose
// index is the number of breaks before the cursor.
func TestDifferential_CurrentLineIsTheCursorsLine(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		d := genDocument(t)
		lines := strings.Split(d.Text, "\n")
		want := lines[strings.Count(d.TextBeforeCursor(), "\n")]
		if got := d.CurrentLine(); got != want {
			t.Fatalf("CurrentLine() of %q at %d = %q, want %q", d.Text, d.CursorPosition, got, want)
		}
	})
}

// TestDifferential_FuzzyScoreMatchesASubsequence checks calculateFuzzyScore
// against a regular expression: a candidate matches when the input's characters
// appear in it in order, which is what the pattern a.*b.*c says.
func TestDifferential_FuzzyScoreMatchesASubsequence(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		input := genText(t, "input")
		candidate := genText(t, "candidate")
		parts := make([]string, 0, len(input))
		for _, r := range input {
			parts = append(parts, regexp.QuoteMeta(string(r)))
		}
		want := regexp.MustCompile(`(?s)` + strings.Join(parts, ".*")).MatchString(candidate)
		if got := calculateFuzzyScore(input, candidate) > 0; got != want {
			t.Fatalf("calculateFuzzyScore(%q, %q) > 0 = %v, want %v", input, candidate, got, want)
		}
	})
}

// genCandidates draws a short candidate list over a small alphabet, so that
// queries match some of it and ties on the score are common.
func genCandidates(t *rapid.T) []string {
	word := rapid.StringOfN(rapid.SampledFrom([]rune("gitsa ")), 1, 6, -1)
	return rapid.SliceOfN(word, 0, 8).Draw(t, "candidates")
}

// TestMetamorphic_FuzzySearchIgnoresTheRest checks how the list of matches
// changes when the candidates do: removing one candidate removes that match and
// leaves the others in their order, and adding a candidate the query does not
// match changes nothing.
func TestMetamorphic_FuzzySearchIgnoresTheRest(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		items := genCandidates(t)
		query := rapid.StringOfN(rapid.SampledFrom([]rune("gits")), 1, 3, -1).Draw(t, "query")
		base := (&fuzzyMatcher{items: items}).searchFunc(query)

		if len(items) > 0 {
			k := rapid.IntRange(0, len(items)-1).Draw(t, "removed")
			fewer := slices.Delete(slices.Clone(items), k, k+1)
			got := (&fuzzyMatcher{items: fewer}).searchFunc(query)
			want := slices.Clone(base)
			if i := slices.Index(want, items[k]); i >= 0 {
				want = slices.Delete(want, i, i+1)
			}
			// A duplicate of the removed candidate stays, and the first copy is
			// the one removed above; the comparison is of the multiset in order.
			if !slices.Equal(got, want) {
				t.Fatalf("removing %q from %q: searchFunc(%q) = %q, want %q", items[k], items, query, got, want)
			}
		}

		unmatched := strings.Repeat("z", len(query))
		pos := rapid.IntRange(0, len(items)).Draw(t, "inserted")
		more := slices.Insert(slices.Clone(items), pos, unmatched)
		if got := (&fuzzyMatcher{items: more}).searchFunc(query); !slices.Equal(got, base) {
			t.Fatalf("adding %q to %q: searchFunc(%q) = %q, want %q", unmatched, items, query, got, base)
		}
	})
}

// TestMetamorphic_FuzzySearchIgnoresCase checks that the case of the query
// does not change what matches or in which order.
func TestMetamorphic_FuzzySearchIgnoresCase(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		items := genCandidates(t)
		query := rapid.StringOfN(rapid.SampledFrom([]rune("gits")), 1, 3, -1).Draw(t, "query")
		m := &fuzzyMatcher{items: items}
		if lower, upper := m.searchFunc(query), m.searchFunc(strings.ToUpper(query)); !slices.Equal(lower, upper) {
			t.Fatalf("searchFunc(%q) = %q but searchFunc(%q) = %q", query, lower, strings.ToUpper(query), upper)
		}
	})
}

// TestProperty_HistoryFollowsItsModel drives a history manager with a random
// sequence of entries and compares it after every step with a plain model:
// an empty entry and a repeat of the last one are not added, and only the
// newest MaxEntries are kept.
func TestProperty_HistoryFollowsItsModel(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		limit := rapid.IntRange(1, 5).Draw(t, "MaxEntries")
		hm := newHistoryManager(&historyConfig{Enabled: true, MaxEntries: limit})
		var model []string
		entry := rapid.SampledFrom([]string{"", "a", "b", "a b", "a\nb"})
		for step := range rapid.IntRange(0, 20).Draw(t, "steps") {
			e := entry.Draw(t, fmt.Sprintf("entry %d", step))
			hm.addEntry(e)
			if e != "" && (len(model) == 0 || model[len(model)-1] != e) {
				model = append(model, e)
				if len(model) > limit {
					model = model[len(model)-limit:]
				}
			}
			if got := hm.getHistory(); !slices.Equal(got, model) {
				t.Fatalf("after adding %q: history %q, want %q", e, got, model)
			}
		}
	})
}

// TestProperty_HistorySurvivesTheFile saves a drawn history and loads it into
// a second manager: every entry comes back as it was, whatever characters it
// holds, and in its order.
func TestProperty_HistorySurvivesTheFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	run := 0
	rapid.Check(t, func(t *rapid.T) {
		run++
		file := filepath.Join(dir, fmt.Sprintf("history-%d", run))
		entries := rapid.SliceOfN(rapid.StringOfN(rapid.SampledFrom(append(editRunes, '\r', 'n')), 1, 8, -1), 0, 10).Draw(t, "entries")

		saver := newHistoryManager(&historyConfig{Enabled: true, File: file, MaxEntries: 100})
		saver.setHistory(entries)
		if err := saver.saveHistory(); err != nil {
			t.Fatal(err)
		}
		loader := newHistoryManager(&historyConfig{Enabled: true, File: file, MaxEntries: 100})
		if err := loader.loadHistory(); err != nil {
			t.Fatal(err)
		}
		if got := loader.getHistory(); !slices.Equal(got, entries) {
			t.Fatalf("saved %q, loaded %q", entries, got)
		}
	})
}
