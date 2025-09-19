package prompt

import (
	"errors"
	"io"
	"os"
	"testing"

	"golang.org/x/term"
)

func TestMockTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		width  int
		height int
	}{
		{
			name:   "simple input",
			input:  "hello",
			width:  80,
			height: 24,
		},
		{
			name:   "empty input",
			input:  "",
			width:  120,
			height: 30,
		},
		{
			name:   "unicode input",
			input:  "こんにちは",
			width:  100,
			height: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockTerminal{
				input:        []rune(tt.input),
				terminalSize: [2]int{tt.width, tt.height},
			}

			// Test SetRaw
			err := mock.SetRaw()
			if err != nil {
				t.Errorf("SetRaw() error = %v", err)
			}
			if !mock.rawMode {
				t.Error("Expected rawMode to be true after SetRaw()")
			}

			// Test Size
			w, h, err := mock.Size()
			if err != nil {
				t.Errorf("Size() error = %v", err)
			}
			if w != tt.width {
				t.Errorf("Expected width %d, got %d", tt.width, w)
			}
			if h != tt.height {
				t.Errorf("Expected height %d, got %d", tt.height, h)
			}

			// Test ReadRune
			for i, expectedRune := range []rune(tt.input) {
				r, size, err := mock.ReadRune()
				if err != nil {
					t.Errorf("ReadRune() at position %d error = %v", i, err)
				}
				if r != expectedRune {
					t.Errorf("Expected rune %c, got %c at position %d", expectedRune, r, i)
				}
				if size != 1 {
					t.Errorf("Expected size 1, got %d at position %d", size, i)
				}
			}

			// Test EOF after input is consumed
			_, _, err = mock.ReadRune()
			if !errors.Is(err, io.EOF) {
				t.Errorf("Expected EOF after consuming all input, got %v", err)
			}

			// Test Restore
			err = mock.Restore()
			if err != nil {
				t.Errorf("Restore() error = %v", err)
			}
			if mock.rawMode {
				t.Error("Expected rawMode to be false after Restore()")
			}

			// Test Close
			err = mock.Close()
			if err != nil {
				t.Errorf("Close() error = %v", err)
			}
		})
	}
}

func TestMockTerminalInputPosition(t *testing.T) {
	t.Parallel()

	mock := &mockTerminal{
		input: []rune("abc"),
	}

	// Read first rune
	r1, _, err := mock.ReadRune()
	if err != nil {
		t.Errorf("First ReadRune() error = %v", err)
	}
	if r1 != 'a' {
		t.Errorf("Expected 'a', got %c", r1)
	}
	if mock.inputPos != 1 {
		t.Errorf("Expected inputPos 1, got %d", mock.inputPos)
	}

	// Read second rune
	r2, _, err := mock.ReadRune()
	if err != nil {
		t.Errorf("Second ReadRune() error = %v", err)
	}
	if r2 != 'b' {
		t.Errorf("Expected 'b', got %c", r2)
	}
	if mock.inputPos != 2 {
		t.Errorf("Expected inputPos 2, got %d", mock.inputPos)
	}

	// Read third rune
	r3, _, err := mock.ReadRune()
	if err != nil {
		t.Errorf("Third ReadRune() error = %v", err)
	}
	if r3 != 'c' {
		t.Errorf("Expected 'c', got %c", r3)
	}
	if mock.inputPos != 3 {
		t.Errorf("Expected inputPos 3, got %d", mock.inputPos)
	}

	// Try to read beyond input
	_, _, err = mock.ReadRune()
	if !errors.Is(err, io.EOF) {
		t.Errorf("Expected EOF, got %v", err)
	}
}

func TestMockTerminalEmptyInput(t *testing.T) {
	t.Parallel()

	mock := &mockTerminal{
		input: []rune{},
	}

	// Should return EOF immediately for empty input
	_, _, err := mock.ReadRune()
	if !errors.Is(err, io.EOF) {
		t.Errorf("Expected EOF for empty input, got %v", err)
	}
}

func TestMockTerminalDefaultSize(t *testing.T) {
	t.Parallel()

	mock := &mockTerminal{} // No size set

	w, h, err := mock.Size()
	if err != nil {
		t.Errorf("Size() error = %v", err)
	}

	// Default size should be [0, 0]
	if w != 0 || h != 0 {
		t.Errorf("Expected default size [0, 0], got [%d, %d]", w, h)
	}
}

func TestMockTerminalRawModeToggle(t *testing.T) {
	t.Parallel()

	mock := &mockTerminal{}

	// Initial state should be not raw
	if mock.rawMode {
		t.Error("Expected initial rawMode to be false")
	}

	// Enable raw mode
	err := mock.SetRaw()
	if err != nil {
		t.Errorf("SetRaw() error = %v", err)
	}
	if !mock.rawMode {
		t.Error("Expected rawMode to be true after SetRaw()")
	}

	// Disable raw mode
	err = mock.Restore()
	if err != nil {
		t.Errorf("Restore() error = %v", err)
	}
	if mock.rawMode {
		t.Error("Expected rawMode to be false after Restore()")
	}

	// Multiple calls should not cause issues
	err = mock.SetRaw()
	if err != nil {
		t.Errorf("Second SetRaw() error = %v", err)
	}
	err = mock.SetRaw()
	if err != nil {
		t.Errorf("Third SetRaw() error = %v", err)
	}

	err = mock.Restore()
	if err != nil {
		t.Errorf("Second Restore() error = %v", err)
	}
	err = mock.Restore()
	if err != nil {
		t.Errorf("Third Restore() error = %v", err)
	}
}

func TestRealTerminalCreation(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping real terminal test in local development")
	}

	t.Parallel()

	// This test attempts to create a real terminal
	// It may fail in CI environments without /dev/tty
	terminal, err := newRealTerminal()
	if err != nil {
		t.Logf("Cannot create real terminal in test environment: %v", err)
		// This is expected in many test environments
		return
	}

	if terminal == nil {
		t.Error("Expected non-nil terminal")
		return
	}

	// Test basic operations
	if terminal.tty == nil {
		t.Error("Expected non-nil tty")
	}

	if terminal.output == nil {
		t.Error("Expected non-nil output")
	}

	// Test size operation
	_, _, err = terminal.Size()
	if err != nil {
		t.Logf("Cannot get terminal size: %v", err)
		// Size might not be available in test environments
	}

	// Clean up
	err = terminal.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestTerminalInterface(t *testing.T) {
	t.Parallel()

	// Test that mockTerminal implements terminalInterface
	var _ terminalInterface = (*mockTerminal)(nil)

	// Test that realTerminal implements terminalInterface
	var _ terminalInterface = (*realTerminal)(nil)
}

func TestMockTerminalWithSpecialCharacters(t *testing.T) {
	t.Parallel()

	specialChars := []struct {
		name string
		char rune
	}{
		{"newline", '\n'},
		{"carriage return", '\r'},
		{"tab", '\t'},
		{"backspace", '\b'},
		{"escape", '\x1b'},
		{"null", '\x00'},
		{"unicode", '🚀'},
		{"unicode text", 'こ'},
	}

	for _, tc := range specialChars {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockTerminal{
				input: []rune{tc.char},
			}

			r, size, err := mock.ReadRune()
			if err != nil {
				t.Errorf("ReadRune() error = %v", err)
			}
			if r != tc.char {
				t.Errorf("Expected %c, got %c", tc.char, r)
			}
			if size != 1 {
				t.Errorf("Expected size 1, got %d", size)
			}
		})
	}
}

func TestMockTerminalLongInput(t *testing.T) {
	t.Parallel()

	// Test with a long input string
	longInput := make([]rune, 1000)
	for i := range longInput {
		longInput[i] = rune('a' + (i % 26)) // a-z repeating
	}

	mock := &mockTerminal{
		input: longInput,
	}

	// Read all characters
	for i, expected := range longInput {
		r, size, err := mock.ReadRune()
		if err != nil {
			t.Errorf("ReadRune() at position %d error = %v", i, err)
		}
		if r != expected {
			t.Errorf("Expected %c at position %d, got %c", expected, i, r)
		}
		if size != 1 {
			t.Errorf("Expected size 1 at position %d, got %d", i, size)
		}
	}

	// Should get EOF after all input is consumed
	_, _, err := mock.ReadRune()
	if !errors.Is(err, io.EOF) {
		t.Errorf("Expected EOF after consuming all input, got %v", err)
	}
}

func TestTerminalStateRestoration(t *testing.T) {
	t.Parallel()

	// Test that terminal state management methods work properly
	mock := &mockTerminal{}

	// Test initial state
	if mock.rawMode {
		t.Error("Expected initial rawMode to be false")
	}

	// Test SetRaw
	err := mock.SetRaw()
	if err != nil {
		t.Errorf("SetRaw() error = %v", err)
	}
	if !mock.rawMode {
		t.Error("Expected rawMode to be true after SetRaw()")
	}

	// Test Restore
	err = mock.Restore()
	if err != nil {
		t.Errorf("Restore() error = %v", err)
	}
	if mock.rawMode {
		t.Error("Expected rawMode to be false after Restore()")
	}

	// Test multiple SetRaw/Restore cycles
	for i := range 3 {
		err = mock.SetRaw()
		if err != nil {
			t.Errorf("SetRaw() cycle %d error = %v", i, err)
		}
		if !mock.rawMode {
			t.Errorf("Expected rawMode to be true after SetRaw() cycle %d", i)
		}

		err = mock.Restore()
		if err != nil {
			t.Errorf("Restore() cycle %d error = %v", i, err)
		}
		if mock.rawMode {
			t.Errorf("Expected rawMode to be false after Restore() cycle %d", i)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	// Test with stdin
	stdinFd := int(os.Stdin.Fd())
	isTerminal := term.IsTerminal(stdinFd)

	if os.Getenv("GITHUB_ACTIONS") != "" {
		// In CI, stdin is usually not a terminal
		t.Logf("IsTerminal(stdin) in CI: %v", isTerminal)
	} else {
		// In local development, it depends on how the test is run
		t.Logf("IsTerminal(stdin) locally: %v", isTerminal)
	}

	// Test with invalid fd
	invalidFd := -1
	isInvalidTerminal := term.IsTerminal(invalidFd)
	if isInvalidTerminal {
		t.Error("Expected IsTerminal(-1) to return false")
	}
}

func TestRealTerminalMultipleSetRawRestore(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping real terminal test in local development")
	}

	terminal, err := newRealTerminal()
	if err != nil {
		t.Logf("Cannot create real terminal: %v", err)
		return
	}
	defer terminal.Close()

	// Multiple SetRaw/Restore cycles
	for i := range 3 {
		err = terminal.SetRaw()
		if err != nil {
			t.Logf("SetRaw() cycle %d failed: %v (may be expected in CI)", i, err)
			return
		}

		err = terminal.Restore()
		if err != nil {
			t.Errorf("Restore() cycle %d failed: %v", i, err)
			return
		}
	}
}

func TestRealTerminalCloseWithoutTTY(t *testing.T) {
	// Create terminal with nil tty
	terminal := &realTerminal{
		tty:    nil,
		closed: false,
	}

	// Close should handle nil tty gracefully and return nil
	err := terminal.Close()
	if err != nil {
		t.Errorf("Close() with nil tty should not error, got: %v", err)
	}

	// Based on the implementation, closed flag is only set when tty exists
	// For nil tty, it should still be false
	if terminal.closed {
		t.Error("Expected closed flag to remain false with nil tty")
	}
}

func TestRealTerminalInterface(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping real terminal test in local development")
	}

	// Test creating a real terminal
	// Note: This might fail in headless environments, so we handle errors gracefully
	terminal, err := newRealTerminal()
	if err != nil {
		t.Skipf("Cannot create real terminal in this environment: %v", err)
		return
	}
	defer terminal.Close()

	// Test SetRaw and Restore
	err = terminal.SetRaw()
	if err != nil {
		t.Errorf("SetRaw failed: %v", err)
	}

	err = terminal.Restore()
	if err != nil {
		t.Errorf("Restore failed: %v", err)
	}

	// Test Size
	width, height, err := terminal.Size()
	if err != nil {
		t.Logf("Size returned error (may be expected in CI): %v", err)
	}
	if width <= 0 || height <= 0 {
		t.Errorf("Expected positive terminal size, got %dx%d", width, height)
	}

	// Test double close (should not panic)
	err1 := terminal.Close()
	err2 := terminal.Close()
	if err1 != nil {
		t.Errorf("First close failed: %v", err1)
	}
	if err2 != nil {
		t.Errorf("Second close should not fail: %v", err2)
	}
}

func TestMockTerminalInterface(t *testing.T) {
	input := "hello\r\nworld\x1b[A\x03"
	mock := newMockTerminal(input)

	// Test SetRaw
	err := mock.SetRaw()
	if err != nil {
		t.Errorf("Mock SetRaw should not fail: %v", err)
	}
	if !mock.rawMode {
		t.Error("Expected rawMode to be true after SetRaw")
	}

	// Test Size
	width, height, err := mock.Size()
	if err != nil {
		t.Errorf("Mock Size should not fail: %v", err)
	}
	if width != 80 || height != 24 {
		t.Errorf("Expected size 80x24, got %dx%d", width, height)
	}

	// Test ReadRune
	expectedRunes := []rune(input)
	for i, expectedRune := range expectedRunes {
		r, size, err := mock.ReadRune()
		if err != nil {
			t.Errorf("ReadRune[%d] failed: %v", i, err)
		}
		if r != expectedRune {
			t.Errorf("ReadRune[%d] expected %q, got %q", i, expectedRune, r)
		}
		if size != 1 {
			t.Errorf("ReadRune[%d] expected size 1, got %d", i, size)
		}
	}

	// Test reading beyond input (should return EOF)
	_, _, err = mock.ReadRune()
	if !errors.Is(err, io.EOF) {
		t.Errorf("Expected EOF when reading beyond input, got: %v", err)
	}

	// Test Restore
	err = mock.Restore()
	if err != nil {
		t.Errorf("Mock Restore should not fail: %v", err)
	}
	if mock.rawMode {
		t.Error("Expected rawMode to be false after Restore")
	}

	// Test Close
	err = mock.Close()
	if err != nil {
		t.Errorf("Mock Close should not fail: %v", err)
	}
}

func TestMockTerminalEdgeCases(t *testing.T) {
	// Test empty input
	mock := newMockTerminal("")
	_, _, err := mock.ReadRune()
	if !errors.Is(err, io.EOF) {
		t.Errorf("Expected EOF for empty input, got: %v", err)
	}

	// Test multiple calls after EOF
	_, _, err = mock.ReadRune()
	if !errors.Is(err, io.EOF) {
		t.Errorf("Expected EOF on second call, got: %v", err)
	}

	// Test unicode input
	unicodeInput := "こんにちは"
	mock = newMockTerminal(unicodeInput)

	expectedRunes := []rune(unicodeInput)
	for i, expectedRune := range expectedRunes {
		r, _, err := mock.ReadRune()
		if err != nil {
			t.Errorf("Unicode ReadRune[%d] failed: %v", i, err)
		}
		if r != expectedRune {
			t.Errorf("Unicode ReadRune[%d] expected %q, got %q", i, expectedRune, r)
		}
	}
}

func TestTerminalSizeFallback(t *testing.T) {
	// This test covers the size fallback logic in realTerminal.Size()
	// We can't easily test this without mocking the underlying tty,
	// but we can test that the mock returns reasonable defaults
	mock := newMockTerminal("test")

	width, height, err := mock.Size()
	if err != nil {
		t.Errorf("Mock Size should not return error: %v", err)
	}

	if width != 80 || height != 24 {
		t.Errorf("Expected default size 80x24, got %dx%d", width, height)
	}
}

func TestTerminalInterfaceCompliance(_ *testing.T) {
	// Test that both implementations satisfy the interface
	var _ terminalInterface = &realTerminal{}
	var _ terminalInterface = &mockTerminal{}

	// This test ensures the interface is properly implemented
	// If it compiles, the interface compliance is verified
}
