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
// Key Bindings:
//
// The library supports comprehensive key bindings out of the box:
//
//   - Enter: Submit input (Shift+Enter for multi-line in appropriate contexts)
//   - Ctrl+C: Cancel and return ErrInterrupted
//   - Ctrl+D: EOF when buffer is empty
//   - Arrow keys: Navigate history (up/down) and move cursor (left/right)
//   - Ctrl+A / Home: Move to beginning of line
//   - Ctrl+E / End: Move to end of line
//   - Ctrl+K: Delete from cursor to end of line
//   - Ctrl+U: Delete entire line
//   - Ctrl+W: Delete word backwards
//   - Ctrl+R: Reverse history search (like bash)
//   - Tab: Auto-completion
//   - Backspace: Delete character backwards
//   - Delete: Delete character forwards
//   - Ctrl+Left/Right: Move by word boundaries
//
// Custom Key Bindings:
//
// You can customize key bindings by creating a custom KeyMap:
//
//	keyMap := prompt.NewDefaultKeyMap()
//	// Add Ctrl+L to clear the line
//	keyMap.Bind('\x0C', prompt.ActionDeleteLine)
//	// Add F1 key for help (escape sequence)
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
//   - io.EOF: User pressed Ctrl+D with empty buffer
//   - context.DeadlineExceeded: Timeout reached (when using context)
//   - context.Canceled: Context was cancelled
//
// Multi-line Input:
//
// The prompt automatically detects and handles multi-line input. When the buffer
// contains newline characters, arrow keys navigate between lines instead of history,
// and Home/End keys move to line boundaries instead of buffer boundaries.
//
// Thread Safety:
//
// Prompt instances are not thread-safe. Each prompt should be used from a single
// goroutine. However, you can safely cancel a prompt from another goroutine using
// context cancellation.
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
