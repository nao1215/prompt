package prompt

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// The benchmarks below cover the work done on every key press (layout,
// highlighting, the menu, fuzzy completion) and the history file read at
// start-up. The Benchmark workflow compares them between the base and the head
// of a pull request, so each one fixes its input and does no I/O but its own.

// BenchmarkFuzzyCompletion ranks 2,000 candidates for a three-letter word, the
// size of a shell's command list.
func BenchmarkFuzzyCompletion(b *testing.B) {
	candidates := make([]string, 2000)
	for i := range candidates {
		candidates[i] = fmt.Sprintf("command-%04d-%s", i, strings.Repeat("x", i%17))
	}
	complete := NewFuzzyCompleter(candidates)
	doc := Document{Text: "cmd", CursorPosition: 3}
	b.ReportAllocs()
	for b.Loop() {
		if len(complete(doc)) == 0 {
			b.Fatal("no suggestion")
		}
	}
}

// BenchmarkLoadHistory reads a 1,000-entry history file with escaped
// newlines, as New does when the prompt starts.
func BenchmarkLoadHistory(b *testing.B) {
	path := filepath.Join(b.TempDir(), "history")
	hm := newHistoryManager(&historyConfig{Enabled: true, File: path, MaxEntries: 1000})
	for i := range 1000 {
		hm.addEntry(fmt.Sprintf("SELECT %d\nFROM t WHERE c = '\\%d'", i, i))
	}
	if err := hm.saveHistory(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		entries, err := readHistoryFile(path)
		if err != nil || len(entries) != 1000 {
			b.Fatalf("read %d entries, err %v", len(entries), err)
		}
	}
}

// BenchmarkRenderMultiline redraws a highlighted 20-line input with an open
// menu of 30 suggestions on an 80x24 terminal.
func BenchmarkRenderMultiline(b *testing.B) {
	r := newRenderer(io.Discard, ThemeDefault, newMockTerminal(""))
	r.setHighlighter(func(s string) []StyleSpan {
		var spans []StyleSpan
		for i := 0; i+6 <= len(s); i += 12 {
			spans = append(spans, StyleSpan{Start: i, End: i + 6, Color: Color{R: 255}})
		}
		return spans
	})
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("SELECT column_%02d, other FROM table_%02d", i, i)
	}
	input := strings.Join(lines, "\n")
	suggestions := make([]Suggestion, 30)
	for i := range suggestions {
		suggestions[i] = Suggestion{Text: fmt.Sprintf("column_%02d", i), Description: "a column"}
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := r.renderWithSuggestionsOffset("> ", input, len(input)/2, suggestions, 5, 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLayoutWide wraps a 4,000-character line of mixed-width runes.
func BenchmarkLayoutWide(b *testing.B) {
	var sb bytes.Buffer
	for range 400 {
		sb.WriteString("abc漢字😀\tde")
	}
	s := sb.String()
	for b.Loop() {
		if rows, _ := layout(s, 80); rows == 0 {
			b.Fatal("no rows")
		}
	}
}
