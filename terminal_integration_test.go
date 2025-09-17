package prompt

import (
	"errors"
	"io"
	"os"
	"testing"
)

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
