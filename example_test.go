package prompt_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nao1215/prompt"
)

// The examples that open a terminal have no Output comment: they are compiled
// so they cannot rot, and not run, because a test has no terminal to give them.

func ExampleNew() {
	p, err := prompt.New("$ ",
		prompt.WithCompleter(prompt.NewFuzzyCompleter([]string{"git status", "git commit"})),
		prompt.WithMemoryHistory(100),
		prompt.WithTheme(prompt.ThemeDracula),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	for {
		line, err := p.Run(context.Background())
		if errors.Is(err, prompt.ErrEOF) {
			return // Ctrl+D on an empty line
		}
		if errors.Is(err, prompt.ErrInterrupted) {
			continue // Ctrl+C discarded the line
		}
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(line)
	}
}

func ExamplePrompt_Run_deadline() {
	p, err := prompt.New("> ")
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	line, err := p.Run(ctx)
	if errors.Is(err, context.DeadlineExceeded) {
		fmt.Println("no answer in thirty seconds")
		return
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(line)
}

func ExamplePrompt_WatchInterrupt() {
	p, err := prompt.New("sql> ")
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	for {
		line, err := p.Run(context.Background())
		if err != nil {
			return
		}

		// Nothing reads the terminal while the query runs, so Ctrl+C would
		// either wait in the buffer or kill the process. The watch turns it
		// into a canceled context instead, until stop is called.
		ctx, stop := p.WatchInterrupt(context.Background())
		err = runQuery(ctx, line)
		stop()

		if errors.Is(err, context.Canceled) {
			fmt.Print("canceled\r\n")
		}
	}
}

func runQuery(ctx context.Context, _ string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Second):
		return nil
	}
}

func ExampleWithMultiline() {
	p, err := prompt.New("sql> ",
		prompt.WithMultiline(),
		// Enter submits once the statement ends in a semicolon, and opens a
		// new line before that.
		prompt.WithIsComplete(func(in string) bool {
			return strings.HasSuffix(strings.TrimSpace(in), ";")
		}),
		prompt.WithContinuationPrefix(" ..> "),
		// A new line opens with the indentation of the line before it.
		prompt.WithAutoIndent(func(before string) string {
			line := before[strings.LastIndex(before, "\n")+1:]
			return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	statement, err := p.Run(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(statement)
}

func ExampleWithKeyMap() {
	keyMap := prompt.NewDefaultKeyMap()
	// On a multiline entry the arrow keys move between its lines, so the
	// history is reached through keys of its own.
	keyMap.Bind('\x10', prompt.ActionHistoryUp)      // Ctrl+P
	keyMap.Bind('\x0e', prompt.ActionHistoryDown)    // Ctrl+N
	keyMap.BindSequence("OP", prompt.ActionComplete) // F1, which the terminal sends as ESC O P

	p, err := prompt.New("$ ", prompt.WithKeyMap(keyMap), prompt.WithMultiline())
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()
}

func ExampleWithFileHistory() {
	dir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}

	// The last thousand entries outlive the process, in a file only its owner
	// can read.
	p, err := prompt.New("$ ", prompt.WithFileHistory(filepath.Join(dir, "myapp", "history"), 1000))
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()
}

func ExampleWithHighlighter() {
	keyword := prompt.Color{R: 198, G: 120, B: 221, Bold: true}

	p, err := prompt.New("sql> ",
		prompt.WithHighlighter(func(input string) []prompt.StyleSpan {
			// Color every SELECT, wherever it is in the input. Offsets are in
			// runes, so they are counted rather than taken from strings.Index.
			var spans []prompt.StyleSpan
			runes := []rune(input)
			for i := 0; i+6 <= len(runes); i++ {
				if strings.EqualFold(string(runes[i:i+6]), "select") {
					spans = append(spans, prompt.StyleSpan{Start: i, End: i + 6, Color: keyword})
				}
			}
			return spans
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()
}

func ExampleSuggestion_replace() {
	// A completer that matches case-insensitively names the span its
	// suggestion stands for, and the prompt applies it without the
	// case-sensitive prefix filter it runs otherwise.
	completer := func(d prompt.Document) []prompt.Suggestion {
		word := d.WordBeforeCursor()
		start := d.CursorPosition - len([]rune(word))

		var out []prompt.Suggestion
		for _, kw := range []string{"SELECT", "INSERT", "UPDATE"} {
			if strings.HasPrefix(strings.ToLower(kw), strings.ToLower(word)) {
				out = append(out, prompt.Suggestion{
					Text:    kw,
					Replace: &prompt.Range{Start: start, End: d.CursorPosition},
				})
			}
		}
		return out
	}

	for _, s := range completer(prompt.Document{Text: "sel", CursorPosition: 3}) {
		fmt.Printf("%s replaces [%d, %d)\n", s.Text, s.Replace.Start, s.Replace.End)
	}
	// Output:
	// SELECT replaces [0, 3)
}

func ExampleDocument() {
	// CursorPosition is counted in runes, so it is 12 here whatever the byte
	// length of the text before it.
	d := prompt.Document{Text: "SELECT 名前\nFROM users", CursorPosition: 12}

	fmt.Printf("%q\n", d.TextBeforeCursor())
	fmt.Printf("%q\n", d.TextAfterCursor())
	fmt.Printf("%q\n", d.CurrentLine())
	fmt.Printf("%q\n", d.WordBeforeCursor())
	// Output:
	// "SELECT 名前\nFR"
	// "OM users"
	// "FROM users"
	// "FR"
}

func ExampleDocument_WordBeforeCursorEscaped() {
	d := prompt.Document{Text: `cat my\ data.csv`, CursorPosition: 16}

	fmt.Println(d.WordBeforeCursor())
	fmt.Println(d.WordBeforeCursorEscaped())
	// Output:
	// data.csv
	// my\ data.csv
}

func ExampleNewFuzzyCompleter() {
	complete := prompt.NewFuzzyCompleter([]string{"git status", "git commit", "docker build", "docker ps"})

	for _, s := range complete(prompt.Document{Text: "dckrbld", CursorPosition: 7}) {
		fmt.Println(s.Text)
	}
	// Output:
	// docker build
}

func ExampleNewFileCompleter() {
	dir, err := os.MkdirTemp("", "prompt-example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	for _, name := range []string{"alpha.txt", "album.md", "beta.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			log.Fatal(err)
		}
	}

	complete := prompt.NewFileCompleter()
	// The completer looks at the word before the cursor, so the command in
	// front of the path does not get in its way.
	typed := "cat " + filepath.Join(dir, "al")
	for _, s := range complete(prompt.Document{Text: typed, CursorPosition: len([]rune(typed))}) {
		fmt.Println(filepath.Base(s.Text))
	}
	// Output:
	// album.md
	// alpha.txt
}

func ExampleColor_ToANSI() {
	fmt.Printf("%q\n", prompt.Color{R: 255, G: 0, B: 0}.ToANSI())
	fmt.Printf("%q\n", prompt.Color{Bold: true}.ToANSI())
	// The zero Color names no color: the terminal keeps its own foreground.
	fmt.Printf("%q\n", prompt.Color{}.ToANSI())
	// Output:
	// "\x1b[38;2;255;0;0m"
	// "\x1b[1m"
	// ""
}
