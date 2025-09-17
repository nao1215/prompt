package prompt

import (
	"errors"
	"io"
	"os"
	"testing"
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
