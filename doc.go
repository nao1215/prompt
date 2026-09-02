// Package prompt provides a modern, robust terminal prompt library for Go.
//
// This library is designed as a replacement for the unmaintained go-prompt
// library, addressing critical issues like divide-by-zero panics, memory leaks,
// and limited cross-platform support while providing enhanced functionality.
//
// Key Features:
//
//   - Interactive terminal prompts with rich editing capabilities
//   - Multi-line input support with proper cursor navigation
//   - Fuzzy auto-completion with intelligent ranking
//   - Command history with reverse search (Ctrl+R)
//   - Configurable key bindings and shortcuts
//   - Cross-platform compatibility (Windows, macOS, Linux)
//   - Context support for timeouts and cancellation
//   - Comprehensive error handling and resource management
//
// Quick Start:
//
// The simplest way to create a prompt:
//
//	package main
//
//	import (
//		"fmt"
//		"log"
//		"github.com/nao1215/prompt"
//	)
//
//	func main() {
//		p, err := prompt.New("Enter command: ")
//		if err != nil {
//			log.Fatal(err)
//		}
//		defer p.Close()
//
//		result, err := p.Run()
//		if err != nil {
//			log.Fatal(err)
//		}
//		fmt.Printf("You entered: %s\n", result)
//	}
//
// Advanced Usage with Completion:
//
//	completer := prompt.NewFuzzyCompleter([]string{
//		"git status", "git commit", "docker run", "kubectl get",
//	})
//
//	p, err := prompt.New("$ ",
//		prompt.WithCompleter(completer),
//		prompt.WithMemoryHistory(100),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer p.Close()
//
//	result, err := p.Run()
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Command: %s\n", result)
//
// Choosing What a Suggestion Replaces:
//
// By default the prompt decides what a suggestion replaces: it takes the word
// before the cursor and keeps a suggestion only when the word is a
// case-sensitive prefix of it. A completer that matches by another rule sets
// Suggestion.Replace to the span it stands for, counted in runes like
// Document.CursorPosition, and the prompt applies that span literally instead:
//
//	func completer(d prompt.Document) []prompt.Suggestion {
//		word := d.GetWordBeforeCursor()
//		start := d.CursorPosition - len([]rune(word))
//
//		var out []prompt.Suggestion
//		for _, kw := range []string{"SELECT", "INSERT", "UPDATE"} {
//			// Match case-insensitively, which the built-in filter cannot do.
//			if strings.HasPrefix(strings.ToLower(kw), strings.ToLower(word)) {
//				out = append(out, prompt.Suggestion{
//					Text:    kw,
//					Replace: &prompt.Range{Start: start, End: d.CursorPosition},
//				})
//			}
//		}
//		return out
//	}
//
// Key Bindings:
//
// The library supports comprehensive key bindings out of the box:
//
//   - Enter: Submit input (Shift+Enter for multi-line in appropriate contexts)
//   - Ctrl+C: Discard the current line and return ErrInterrupted
//   - Ctrl+D: EOF when buffer is empty
//   - Esc: Close the completion popup
//   - Arrow keys: Navigate history (up/down) and move cursor (left/right)
//   - Ctrl+A / Home: Move to beginning of line
//   - Ctrl+E / End: Move to end of line
//   - Ctrl+K: Delete from cursor to end of line
//   - Ctrl+U: Delete entire line
//   - Ctrl+W: Delete word backwards
//   - Ctrl+R: Reverse history search (like bash). Tab and the arrow keys move
//     through the matches, Enter accepts the one the search names, Escape
//     cancels. A query that matches nothing has nothing to accept, so Enter
//     leaves the line as the search found it.
//   - Tab: Auto-completion
//   - Backspace: Delete character backwards
//   - Delete: Delete character forwards
//   - Ctrl+Left/Right: Move by word boundaries
//
// A completion menu stands for the word before the cursor, so editing the line
// or moving the cursor off that word ends it and the next Tab asks again. It
// lists at most ten candidates, and fewer when the terminal has fewer rows to
// spare under the line being typed -- none at all when it has none, which is
// what keeps that line on screen; Up and Down scroll a longer list.
//
// While an application runs work between prompts (a query, an import), no one is
// reading the terminal, so Ctrl+C either waits in the buffer as a byte or, once
// the prompt has given raw mode back, arrives as a SIGINT that would otherwise
// kill the application. WatchInterrupt watches for the byte and the signal
// during that gap: the key cancels the context it returns instead of ending the
// process, until the stop function is called.
//
// Custom Key Bindings:
//
// You can customize key bindings by creating a custom KeyMap:
//
//	keyMap := prompt.NewDefaultKeyMap()
//	// Reach the history from a multiline entry, the way a shell does
//	keyMap.Bind('\x10', prompt.ActionHistoryUp)   // Ctrl+P
//	keyMap.Bind('\x0E', prompt.ActionHistoryDown) // Ctrl+N
//	// Add F1 for completion (a terminal sends it as an escape sequence)
//	keyMap.BindSequence("OP", prompt.ActionComplete)
//
//	config := prompt.Config{
//		Prefix: "$ ",
//		KeyMap: keyMap,
//	}
//
// Context Support:
//
// Use RunWithContext for timeout or cancellation support:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	result, err := p.RunWithContext(ctx)
//	if err == context.DeadlineExceeded {
//		fmt.Println("Timeout reached")
//		return
//	}
//
// Error Handling:
//
// The library provides specific error types for different scenarios:
//
//   - prompt.ErrInterrupted: User pressed Ctrl+C
//   - prompt.ErrEOF: User pressed Ctrl+D with an empty buffer, or the input
//     reached its end. It matches io.EOF as well as itself.
//   - context.DeadlineExceeded: Timeout reached (when using context)
//   - context.Canceled: Context was canceled
//
// Multi-line Input:
//
// The prompt automatically detects and handles multi-line input. When the buffer
// contains newline characters, arrow keys navigate between lines instead of history,
// and Home/End keys move to line boundaries instead of buffer boundaries.
//
// An entry taller than the terminal is drawn as the rows around the cursor that
// the terminal has room for, redrawn in place. The rows outside that window are
// left undrawn rather than drawn and scrolled away, because what scrolls off the
// top of the screen is the application's output rather than the prompt's. The
// window moves only as far as the cursor makes it: an entry does not scroll
// under a cursor that is already on screen.
//
// A line ends at the foot of the entry, whichever line the cursor was on when
// Enter or Ctrl+C was pressed. The cursor is left after the entry's last
// character, so what an application prints next starts below the entry rather
// than on top of the rows the cursor was above.
//
// Thread Safety:
//
// Prompt instances are not thread-safe. Each prompt should be used from a single
// goroutine. Ending a session is the exception, because a prompt waiting for a
// key cannot end itself: canceling the context passed to RunWithContext returns
// context.Canceled, and Close returns ErrEOF.
//
// Resource Management:
//
// Always call Close() when done with a prompt to prevent resource leaks:
//
//	p, err := prompt.New(config)
//	if err != nil {
//		return err
//	}
//	defer p.Close() // Essential for cleanup
//
// The Close method is safe to call multiple times and should be called even if
// Run or RunWithContext returns an error.
package prompt
