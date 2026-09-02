package prompt

import (
	"bufio"
	"strings"
)

// mockTerminal implements terminalInterface for testing and development.
//
// This implementation provides predictable, deterministic behavior for unit tests
// and development scenarios. It simulates terminal behavior without requiring
// actual terminal interaction, allowing for automated testing of prompt functionality.
//
// Features:
//   - Deterministic input: Pre-configured input sequence for reproducible tests
//   - Configurable size: Fixed terminal dimensions for consistent layout testing
//   - Mode tracking: Tracks raw mode state for verification in tests
//   - No side effects: Safe to use in CI/CD environments and headless testing
//
// The mock terminal is essential for testing complex scenarios like multi-line
// input, history navigation, and completion without manual interaction.
type mockTerminal struct {
	// input holds the bytes of the script, decoded the way the real terminal
	// decodes them. Holding runes instead would have decoded them here, where
	// a byte that is not valid UTF-8 becomes U+FFFD before the prompt has seen
	// it: a test of what the prompt does with such a byte would then be a test
	// of the conversion in this file.
	input *bufio.Reader
	// script is what the reader was built from, for a test that asserts what a
	// helper handed the terminal rather than what the terminal did with it.
	script       string
	rawMode      bool   // Track raw mode state for test verification
	terminalSize [2]int // Fixed terminal dimensions [width, height]
	setRawCount  int    // Number of SetRaw calls, for verifying raw mode is entered once per session
	restoreCount int    // Number of Restore calls, for verifying restoration happens
}

func newMockTerminal(input string) *mockTerminal {
	return &mockTerminal{
		input:        bufio.NewReader(strings.NewReader(input)),
		script:       input,
		rawMode:      false,
		terminalSize: [2]int{80, 24},
	}
}

func (m *mockTerminal) SetRaw() error {
	m.rawMode = true
	m.setRawCount++
	return nil
}

func (m *mockTerminal) Restore() error {
	m.rawMode = false
	m.restoreCount++
	return nil
}

func (m *mockTerminal) Size() (width, height int, err error) {
	return m.terminalSize[0], m.terminalSize[1], nil
}

func (m *mockTerminal) ReadRune() (rune, int, error) {
	r, _, err := m.input.ReadRune()
	if err != nil {
		return 0, 0, err
	}
	// The real terminal answers 1 whatever the rune's width, because what the
	// caller counts is runes.
	return r, 1, nil
}

func (m *mockTerminal) Close() error {
	return nil
}
