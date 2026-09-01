// Package main is the toy SQL shell recorded for the animation in the README.
//
// It is a real program built on this library rather than a script: what the
// animation shows is what the code below does, so a change that breaks the
// prompt breaks the recording's source too. Everything it queries is held in
// this file, so a run depends on nothing outside it -- no database, no working
// directory, no files -- and the same keystrokes produce the same screen on any
// machine.
//
// Run it with:
//
//	go run ./example/demo
package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/nao1215/prompt"
)

func main() {
	p, err := prompt.New("demo=# ",
		prompt.WithCompleter(completer),
		prompt.WithHighlighter(highlight),
		prompt.WithMemoryHistory(100),
		prompt.WithMultiline(true),
		prompt.WithIsComplete(statementIsComplete),
		prompt.WithContinuationPrefix("demo-# "),
		prompt.WithTheme(prompt.ThemeDracula),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	fmt.Print("prompt demo -- a toy SQL shell. Type \\q to leave.\n\n")

	for {
		statement, err := p.Run()
		if err != nil {
			if errors.Is(err, prompt.ErrEOF) || errors.Is(err, prompt.ErrInterrupted) {
				fmt.Println("bye")
				return
			}
			log.Fatal(err)
		}
		if strings.TrimSpace(statement) == `\q` {
			fmt.Println("bye")
			return
		}
		fmt.Println(execute(statement))
	}
}

// schema is what the shell knows about: one table, and what its columns hold.
// The completer and the query engine both read it, so the names offered are the
// names that answer.
var schema = []struct {
	table   string
	columns []string
	rows    [][]string
}{
	{
		table:   "users",
		columns: []string{"id", "name", "email", "created_at"},
		rows: [][]string{
			{"1", "alice", "alice@example.com", "2026-01-14"},
			{"2", "bob", "bob@example.com", "2026-02-02"},
			{"3", "carol", "carol@example.com", "2026-03-30"},
		},
	},
	{
		table:   "orders",
		columns: []string{"id", "user_id", "total", "placed_at"},
		rows: [][]string{
			{"1", "1", "42.00", "2026-04-01"},
			{"2", "3", "17.50", "2026-04-06"},
		},
	},
}

// The keywords the shell reacts to, named once so the completer, the
// highlighter and the query engine cannot drift apart on their spelling.
const (
	kwSelect = "select"
	kwFrom   = "from"
	kwWhere  = "where"
)

// keywords are offered with a description, which is the half of a completion
// menu that tells the user which candidate they want.
var keywords = []prompt.Suggestion{
	{Text: kwSelect, Description: "read rows from a table"},
	{Text: kwFrom, Description: "name the table to read"},
	{Text: kwWhere, Description: "keep only the rows that match"},
	{Text: "order", Description: "sort the rows"},
	{Text: "limit", Description: "stop after this many rows"},
}

// completer answers with the words that can follow what has been typed: table
// names after FROM, column names after SELECT or WHERE, and keywords otherwise.
//
// It is deliberately not a SQL parser. The point it makes is that a completer
// is given the whole Document and can decide from context, which is what
// separates a useful menu from an alphabetical list of everything.
func completer(d prompt.Document) []prompt.Suggestion {
	before := strings.ToLower(d.TextBeforeCursor())
	fields := strings.Fields(before)

	var previous string
	if len(fields) > 0 && !strings.HasSuffix(before, " ") {
		fields = fields[:len(fields)-1]
	}
	if len(fields) > 0 {
		previous = fields[len(fields)-1]
	}

	switch previous {
	case kwFrom, "join", "into", "update":
		return tableSuggestions()
	case kwSelect, kwWhere, "and", "by", ",":
		return columnSuggestions()
	default:
		return keywords
	}
}

func tableSuggestions() []prompt.Suggestion {
	out := make([]prompt.Suggestion, 0, len(schema))
	for _, table := range schema {
		out = append(out, prompt.Suggestion{
			Text:        table.table,
			Description: fmt.Sprintf("%d rows, %d columns", len(table.rows), len(table.columns)),
		})
	}
	return out
}

func columnSuggestions() []prompt.Suggestion {
	out := make([]prompt.Suggestion, 0, 8)
	out = append(out, prompt.Suggestion{Text: "*", Description: "every column"})
	for _, table := range schema {
		for _, column := range table.columns {
			out = append(out, prompt.Suggestion{
				Text:        column,
				Description: table.table,
			})
		}
	}
	return out
}

// highlight colors the keywords and the quoted strings of the line as it is
// typed.
//
// Spans are rune offsets into the whole input, end exclusive, and they only
// choose colors: the layout is measured from the plain text, so a highlighter
// cannot move the cursor away from the character under it however it colors
// the line.
func highlight(input string) []prompt.StyleSpan {
	var spans []prompt.StyleSpan
	runes := []rune(input)

	for start := 0; start < len(runes); {
		switch {
		case runes[start] == '\'':
			end := start + 1
			for end < len(runes) && runes[end] != '\'' {
				end++
			}
			if end < len(runes) {
				end++ // the closing quote belongs to the string
			}
			spans = append(spans, prompt.StyleSpan{Start: start, End: end, Color: colorString})
			start = end
		case isWordRune(runes[start]):
			end := start
			for end < len(runes) && isWordRune(runes[end]) {
				end++
			}
			if isKeyword(string(runes[start:end])) {
				spans = append(spans, prompt.StyleSpan{Start: start, End: end, Color: colorKeyword})
			}
			start = end
		default:
			start++
		}
	}
	return spans
}

var (
	colorKeyword = prompt.Color{R: 0xff, G: 0x79, B: 0xc6, Bold: true}
	colorString  = prompt.Color{R: 0xf1, G: 0xfa, B: 0x8c}
)

func isWordRune(r rune) bool {
	return r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func isKeyword(word string) bool {
	switch strings.ToLower(word) {
	case kwSelect, kwFrom, kwWhere, "order", "by", "limit", "and", "or", "not", "insert", "into", "values", "update", "set", "delete":
		return true
	default:
		return false
	}
}

// statementIsComplete decides whether Enter submits or opens another line. A
// statement ends at a semicolon, so anything else is still being typed and the
// prompt draws the continuation prefix instead of running it.
//
// A line that is only whitespace, and the shell's own \q, submit as they are:
// waiting for a semicolon that is never coming is how a REPL traps its user.
func statementIsComplete(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || strings.HasPrefix(trimmed, `\`) {
		return true
	}
	return strings.HasSuffix(trimmed, ";")
}

// execute answers a statement the way the demo's shell does: it recognizes
// "select <columns> from <table>" and prints the rows, and says so plainly for
// anything else.
func execute(statement string) string {
	fields := strings.Fields(strings.TrimSuffix(strings.TrimSpace(statement), ";"))
	if len(fields) < 4 || !strings.EqualFold(fields[0], kwSelect) {
		return "only SELECT is implemented in this demo"
	}

	var from int
	for i, field := range fields {
		if strings.EqualFold(field, kwFrom) {
			from = i
			break
		}
	}
	if from == 0 || from+1 >= len(fields) {
		return "the statement names no table"
	}

	name := fields[from+1]
	for _, table := range schema {
		if !strings.EqualFold(table.table, name) {
			continue
		}
		columns := selectedColumns(table.columns, fields[1:from])
		if len(columns) == 0 {
			return "the statement names no column of " + table.table
		}
		return renderTable(table.columns, columns, table.rows)
	}
	return "no table named " + name
}

// selectedColumns returns the indexes of the columns the statement asked for,
// in the order it asked for them. A "*" asks for all of them.
func selectedColumns(available []string, asked []string) []int {
	joined := strings.Join(asked, "")
	if strings.Contains(joined, "*") {
		indexes := make([]int, len(available))
		for i := range available {
			indexes[i] = i
		}
		return indexes
	}

	var indexes []int
	for want := range strings.SplitSeq(joined, ",") {
		want = strings.TrimSpace(want)
		for i, column := range available {
			if strings.EqualFold(column, want) {
				indexes = append(indexes, i)
			}
		}
	}
	return indexes
}

// renderTable draws the selected columns as a bordered table, sized to its
// widest cell.
func renderTable(names []string, columns []int, rows [][]string) string {
	widths := make([]int, len(columns))
	for i, column := range columns {
		widths[i] = len(names[column])
		for _, row := range rows {
			widths[i] = max(widths[i], len(row[column]))
		}
	}

	var b strings.Builder
	writeRow := func(cells []string) {
		for i := range columns {
			fmt.Fprintf(&b, " %-*s |", widths[i], cells[i])
		}
		b.WriteString("\n")
	}

	header := make([]string, len(columns))
	for i, column := range columns {
		header[i] = names[column]
	}
	writeRow(header)
	for i := range columns {
		b.WriteString(strings.Repeat("-", widths[i]+2))
		b.WriteString("+")
	}
	b.WriteString("\n")

	cells := make([]string, len(columns))
	for _, row := range rows {
		for i, column := range columns {
			cells[i] = row[column]
		}
		writeRow(cells)
	}
	fmt.Fprintf(&b, "(%d rows)", len(rows))
	return b.String()
}
