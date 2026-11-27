package prompt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// errWrite is what failAfterWriter returns once its budget is spent.
var errWrite = errors.New("terminal write failed")

// failAfterWriter accepts its first ok writes and fails every one after them,
// so a test can make each write of a session the one that fails in turn.
type failAfterWriter struct {
	ok     int
	writes int
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.writes >= w.ok {
		return 0, errWrite
	}
	w.writes++
	return len(p), nil
}

// writeFailureScenario is a scripted session: the prompt it runs on and the
// line the session returns when every write succeeds.
type writeFailureScenario struct {
	name  string
	build func(w *failAfterWriter) *Prompt
	want  string
}

func writeFailureScenarios() []writeFailureScenario {
	on := func(mock *mockTerminal, w *failAfterWriter, opts ...Option) *Prompt {
		p := newTestPrompt(mock, opts...)
		p.output = w
		p.renderer.output = w
		p.renderer.setHighlighter(p.config.Highlighter)
		return p
	}
	completer := func(Document) []Suggestion {
		suggestions := make([]Suggestion, 12)
		for i := range suggestions {
			suggestions[i] = Suggestion{Text: fmt.Sprintf("select%d", i), Description: "a statement"}
		}
		return suggestions
	}
	highlight := func(s string) []StyleSpan {
		if len(s) < 2 {
			return nil
		}
		return []StyleSpan{{Start: 1, End: 2, Color: Color{R: 200}}}
	}
	return []writeFailureScenario{
		{
			name: "typing with a highlighter",
			build: func(w *failAfterWriter) *Prompt {
				return on(newMockTerminal("abc\x1b[D\x7fd\r"), w, WithHighlighter(highlight))
			},
			want: "adc",
		},
		{
			name: "completion menu moved both ways",
			build: func(w *failAfterWriter) *Prompt {
				return on(newMockTerminal("s\t\x1b[B\x1b[B\x1b[A\t\r"), w, WithCompleter(completer))
			},
			want: "select1",
		},
		{
			name: "history search",
			build: func(w *failAfterWriter) *Prompt {
				p := on(newMockTerminal("\x12sel\x1b[A\x1b[B\r\r"), w)
				p.history = []string{"select 1", "select 2", "update"}
				return p
			},
			want: "select 1",
		},
		{
			name: "long paste",
			build: func(w *failAfterWriter) *Prompt {
				return on(newMockTerminal("\x1b[200~"+strings.Repeat("x", 2*pasteDrawInterval)+"\x1b[201~\r"), w)
			},
			want: strings.Repeat("x", 2*pasteDrawInterval),
		},
		{
			name: "multiline input taller than the terminal",
			build: func(w *failAfterWriter) *Prompt {
				mock := newMockTerminal("one\\\rtwo\\\rthree\\\rfour\\\rfive" + strings.Repeat("\x1b[A", 4) + "\r")
				mock.terminalSize = [2]int{12, 3}
				return on(mock, w, WithMultiline(), WithHighlighter(highlight))
			},
			want: "one\ntwo\nthree\nfour\nfive",
		},
	}
}

// TestEveryFailedWriteEndsTheSession makes each write of a scripted session
// fail in turn. A prompt that cannot draw leaves the screen and the buffer out
// of step, so until the line is accepted a failed write must end the session
// with its error. Once Enter has accepted the line, the writes left (the final
// redraw, the line break, leaving raw mode) only tidy the screen, and the
// session still returns the line when they fail. The writes therefore split at
// one point: every failure before it is an error, and every failure after it
// returns the line.
func TestEveryFailedWriteEndsTheSession(t *testing.T) {
	t.Parallel()

	for _, sc := range writeFailureScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()

			counter := &failAfterWriter{ok: 1 << 30}
			got, err := sc.build(counter).Run(context.Background())
			if err != nil || got != sc.want {
				t.Fatalf("unfailing session = %q, %v; want %q, nil", got, err, sc.want)
			}
			total := counter.writes

			accepted := -1 // the first write whose failure no longer ends the session
			for ok := range total {
				got, err := sc.build(&failAfterWriter{ok: ok}).Run(context.Background())
				switch {
				case errors.Is(err, errWrite) && accepted < 0:
				case err == nil && got == sc.want:
					if accepted < 0 {
						accepted = ok
					}
				default:
					t.Errorf("write %d of %d failed: Run = %q, %v; want the write's error before the line is accepted and the line after", ok+1, total, got, err)
				}
			}
			// The tidying after Enter is a handful of writes. A boundary far
			// before the end would mean a failure during editing was swallowed.
			if accepted < 0 || total-accepted > 20 {
				t.Errorf("failures stop ending the session at write %d of %d", accepted+1, total)
			}
		})
	}
}
