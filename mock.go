package prompt

import "io"

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
	input        []rune // Pre-configured input sequence for testing
	inputPos     int    // Current position in the input sequence
	rawMode      bool   // Track raw mode state for test verification
	terminalSize [2]int // Fixed terminal dimensions [width, height]
}

func newMockTerminal(input string) *mockTerminal {
	return &mockTerminal{
		input:        []rune(input),
		inputPos:     0,
		rawMode:      false,
		terminalSize: [2]int{80, 24},
	}
}

func (m *mockTerminal) SetRaw() error {
	m.rawMode = true
	return nil
}

func (m *mockTerminal) Restore() error {
	m.rawMode = false
	return nil
}

func (m *mockTerminal) Size() (width, height int, err error) {
	return m.terminalSize[0], m.terminalSize[1], nil
}

func (m *mockTerminal) ReadRune() (rune, int, error) {
	if m.inputPos >= len(m.input) {
		return 0, 0, io.EOF
	}
	r := m.input[m.inputPos]
	m.inputPos++
	return r, 1, nil
}

func (m *mockTerminal) Close() error {
	return nil
}
