package prompt

import (
	"io"
	"os"
	"runtime"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-tty"
	"golang.org/x/term"
)

// terminalInterface abstracts terminal operations for testability and cross-platform compatibility.
//
// This interface provides a clean abstraction over platform-specific terminal operations,
// allowing the prompt to work with both real terminals (via go-tty) and mock terminals
// for testing. It handles raw mode switching, size detection, input reading, and resource cleanup.
//
// Implementations:
//   - realTerminal: Uses go-tty for actual terminal interaction
//   - mockTerminal: Provides deterministic behavior for testing
//
// The interface addresses common terminal issues from the original go-prompt:
//   - Prevents file descriptor leaks through proper Close() implementation
//   - Provides safe fallback sizes to prevent divide-by-zero panics
//   - Supports cross-platform raw mode handling
type terminalInterface interface {
	SetRaw() error                        // Enter raw mode for immediate key processing
	Restore() error                       // Restore original terminal settings
	Size() (width, height int, err error) // Get terminal dimensions with safe fallbacks
	ReadRune() (rune, int, error)         // Read a single Unicode character from input
	Close() error                         // Clean up resources and prevent fd leaks
}

// realTerminal implements terminalInterface using external libraries for production use.
//
// This implementation leverages go-tty for cross-platform terminal handling and
// go-colorable for Windows ANSI color support. It addresses several critical issues
// from the original go-prompt implementation:
//
//   - Double-close protection: The 'closed' flag prevents Windows panics on double Close()
//   - Safe size fallbacks: Returns 80x24 if terminal size detection fails
//   - Color support: Uses go-colorable for Windows ANSI escape sequence processing
//   - Resource management: Properly closes TTY to prevent file descriptor leaks
//   - Proper raw mode handling: Uses golang.org/x/term for reliable terminal state management
//
// The terminal properly manages raw mode state to ensure terminal restoration
// even when interrupted by Ctrl-C or other signals.
type realTerminal struct {
	tty           *tty.TTY    // TTY handle from go-tty for cross-platform terminal operations
	output        io.Writer   // Color-capable output writer (colorable on Windows, stdout elsewhere)
	closed        bool        // Track if terminal is already closed to prevent double-close panic on Windows
	stdinFd       int         // File descriptor for stdin for raw mode management
	originalState *term.State // Original terminal state to restore on exit
}

// newRealTerminal creates a new terminal instance following simplified design
func newRealTerminal() (*realTerminal, error) {
	// Use go-tty for cross-platform terminal handling
	t, err := tty.Open()
	if err != nil {
		return nil, err
	}

	// Setup color-capable output
	var output io.Writer = os.Stdout
	if runtime.GOOS == "windows" {
		// Use colorable for Windows ANSI color support
		output = colorable.NewColorableStdout()
	}

	// Get stdin file descriptor for raw mode management
	stdinFd := int(os.Stdin.Fd())

	return &realTerminal{
		tty:     t,
		output:  output,
		stdinFd: stdinFd,
	}, nil
}

func (t *realTerminal) SetRaw() error {
	// Always capture current terminal state before entering raw mode
	// This ensures proper restoration regardless of how many times we enter/exit raw mode
	if term.IsTerminal(t.stdinFd) {
		state, err := term.GetState(t.stdinFd)
		if err != nil {
			return err
		}
		t.originalState = state

		// Use golang.org/x/term to enter raw mode instead of relying solely on go-tty
		// This gives us better control over terminal state management
		_, err = term.MakeRaw(t.stdinFd)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *realTerminal) Restore() error {
	// Restore original terminal state to fix cursor position and input visibility
	if t.originalState != nil && term.IsTerminal(t.stdinFd) {
		err := term.Restore(t.stdinFd, t.originalState)
		// Reset the state so that SetRaw can capture a fresh baseline next time
		t.originalState = nil
		return err
	}
	return nil
}

func (t *realTerminal) Size() (width, height int, err error) {
	w, h, err := t.tty.Size()
	if err != nil || w <= 0 || h <= 0 {
		// Safe fallback to prevent divide by zero (addresses go-prompt issue #277)
		return 80, 24, err
	}
	return w, h, nil
}

func (t *realTerminal) ReadRune() (rune, int, error) {
	r, err := t.tty.ReadRune()
	if err != nil {
		return 0, 0, err
	}
	// Return size as 1 for single rune (compatible with io.RuneReader)
	return r, 1, nil
}

func (t *realTerminal) Close() error {
	// Prevent double-close which causes panic on Windows
	if t.closed {
		return nil
	}
	if t.tty != nil {
		err := t.tty.Close()
		t.closed = true
		return err
	}
	return nil
}
