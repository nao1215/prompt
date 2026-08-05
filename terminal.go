package prompt

import (
	"io"
	"os"
	"runtime"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-tty"
	"golang.org/x/term"
)

// rawEnter is defined per platform (terminal_unix.go, terminal_windows.go). It
// puts the terminal into raw mode and returns a function that restores the
// pre-raw state. When there is nothing to make raw (input is not a terminal, or
// there is no tty), it returns a nil restore func and a nil error so SetRaw
// becomes a no-op. See each file for why the platforms differ.

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
//   - Raw mode on the read handle: raw mode is applied to whatever handle input is
//     read from, per platform, so a re-rendered prompt cannot outrun raw mode
//
// The terminal properly manages raw mode state to ensure terminal restoration
// even when interrupted by Ctrl-C or other signals.
type realTerminal struct {
	tty        *tty.TTY     // TTY handle from go-tty for cross-platform terminal operations
	output     io.Writer    // Color-capable output writer (colorable on Windows, stdout elsewhere)
	closed     bool         // Track if terminal is already closed to prevent double-close panic on Windows
	restoreRaw func() error // Restores the pre-raw terminal state; nil when not in raw mode
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
	if runtime.GOOS == windowsOS {
		// Use colorable for Windows ANSI color support
		output = colorable.NewColorableStdout()
	}

	return &realTerminal{
		tty:    t,
		output: output,
	}, nil
}

// SetRaw enters raw mode via the platform-specific rawEnter, which applies raw
// mode to the handle input is actually read from. It is idempotent: once a
// restore hook is held it does nothing, so a persistent session enters raw mode
// once and Restore stays balanced.
func (t *realTerminal) SetRaw() error {
	if t.restoreRaw != nil {
		return nil
	}
	restore, err := t.rawEnter()
	if err != nil {
		return err
	}
	t.restoreRaw = restore
	return nil
}

// Restore returns the terminal to the state captured when SetRaw entered raw
// mode. It is idempotent: when not in raw mode it does nothing.
func (t *realTerminal) Restore() error {
	if t.restoreRaw == nil {
		return nil
	}
	restore := t.restoreRaw
	t.restoreRaw = nil
	return restore()
}

// Size reports the terminal's size in character cells.
//
// It asks the operating system, through the terminal's own descriptor, rather
// than go-tty's Size. go-tty's falls back to asking the terminal for its size in
// pixels — it writes "\x1b[14t" and then reads the reply back from the input
// handle — whenever the kernel reports no pixel dimensions, which is the normal
// case under tmux and most terminals. Anything the user typed while that read was
// waiting was consumed with the reply and never reached the prompt, so a
// keystroke during a redraw disappeared. Nothing here writes to the terminal or
// reads from it.
func (t *realTerminal) Size() (width, height int, err error) {
	if t.tty != nil {
		if out := t.tty.Output(); out != nil {
			w, h, err := term.GetSize(int(out.Fd()))
			if err == nil && w > 0 && h > 0 {
				return w, h, nil
			}
			// Safe fallback to prevent divide by zero (addresses go-prompt issue #277)
			return fallbackWidth, fallbackHeight, err
		}
	}
	return fallbackWidth, fallbackHeight, nil
}

// The size assumed when the terminal cannot report one.
const (
	fallbackWidth  = 80
	fallbackHeight = 24
)

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
