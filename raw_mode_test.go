package prompt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// newTestPrompt builds a Prompt wired to the given mock terminal and a discard
// output, ready to drive RunWithContext in tests. The options configure the
// embedded Config (for example WithPersistentRawMode).
func newTestPrompt(mock *mockTerminal, options ...Option) *Prompt {
	config := Config{Prefix: "$ "}
	for _, option := range options {
		option(&config)
	}
	var output bytes.Buffer
	return &Prompt{
		config:   config,
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
		output:   &output,
		buffer:   []rune{},
		cursor:   0,
		history:  []string{},
		renderer: newRenderer(&output, ThemeDefault, mock),
	}
}

// TestConsecutiveRunNoInputLost feeds a REPL-style input sequence through
// consecutive Run calls sharing one terminal and asserts every line is consumed,
// including the rune delivered right at the render-to-read boundary of the second
// call. This pins the "no input dropped between consecutive Run calls" contract.
func TestConsecutiveRunNoInputLost(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("first\rsecond\rthird\r")
	p := newTestPrompt(mock)

	want := []string{"first", "second", "third"}
	for i, expected := range want {
		got, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Fatalf("Run %d returned error: %v", i, err)
		}
		if got != expected {
			t.Errorf("Run %d = %q, want %q", i, got, expected)
		}
	}
}

// TestInputAvailableBeforeReadIsConsumed makes input available before the read
// loop starts (the mock is pre-loaded with the whole line, which is present the
// moment render finishes) and asserts it is still read. This pins the "no input
// dropped between render and read" contract.
func TestInputAvailableBeforeReadIsConsumed(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("ready\r")
	p := newTestPrompt(mock)

	got, err := p.RunWithContext(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got != "ready" {
		t.Errorf("Run = %q, want %q", got, "ready")
	}
}

// TestPersistentRawModeEntersOnce asserts that with WithPersistentRawMode the
// terminal enters raw mode exactly once across a multi-line session (not once per
// line), that every line is still read, and that Close restores the terminal.
func TestPersistentRawModeEntersOnce(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("alpha\rbeta\rgamma\r")
	p := newTestPrompt(mock, WithPersistentRawMode())

	want := []string{"alpha", "beta", "gamma"}
	for i, expected := range want {
		got, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Fatalf("Run %d returned error: %v", i, err)
		}
		if got != expected {
			t.Errorf("Run %d = %q, want %q", i, got, expected)
		}
	}

	if mock.setRawCount != 1 {
		t.Errorf("SetRaw called %d times across the session, want 1", mock.setRawCount)
	}
	if mock.restoreCount != 0 {
		t.Errorf("Restore called %d times before Close, want 0 in persistent mode", mock.restoreCount)
	}
	if !mock.rawMode {
		t.Error("terminal should still be in raw mode between persistent Run calls")
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if mock.restoreCount != 1 {
		t.Errorf("Restore called %d times after Close, want 1", mock.restoreCount)
	}
	if mock.rawMode {
		t.Error("terminal should be restored to cooked mode after Close")
	}
}

// TestDefaultModeTogglesRawPerCall asserts the default behavior is preserved:
// raw mode is entered and restored on every Run call, so a two-line session
// toggles the terminal twice.
func TestDefaultModeTogglesRawPerCall(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("one\rtwo\r")
	p := newTestPrompt(mock)

	for i := range 2 {
		if _, err := p.RunWithContext(context.Background()); err != nil {
			t.Fatalf("Run %d returned error: %v", i, err)
		}
	}

	if mock.setRawCount != 2 {
		t.Errorf("SetRaw called %d times, want 2 (once per call in default mode)", mock.setRawCount)
	}
	if mock.restoreCount != 2 {
		t.Errorf("Restore called %d times, want 2 (once per call in default mode)", mock.restoreCount)
	}
	if mock.rawMode {
		t.Error("terminal should be restored to cooked mode after each default-mode Run")
	}
}

// TestPersistentRawModeRestoresOnEOF asserts that when input reaches EOF the
// terminal is restored even in persistent mode, and Run returns ErrEOF.
func TestPersistentRawModeRestoresOnEOF(t *testing.T) {
	t.Parallel()

	// One complete line, then EOF (no trailing submit) on the second Run.
	mock := newMockTerminal("done\r")
	p := newTestPrompt(mock, WithPersistentRawMode())

	got, err := p.RunWithContext(context.Background())
	if err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}
	if got != "done" {
		t.Errorf("first Run = %q, want %q", got, "done")
	}
	if mock.restoreCount != 0 {
		t.Errorf("Restore called %d times after a normal submit, want 0 in persistent mode", mock.restoreCount)
	}

	_, err = p.RunWithContext(context.Background())
	if !errors.Is(err, ErrEOF) {
		t.Fatalf("second Run error = %v, want ErrEOF", err)
	}
	if mock.restoreCount != 1 {
		t.Errorf("Restore called %d times after EOF, want 1", mock.restoreCount)
	}
	if mock.rawMode {
		t.Error("terminal should be restored to cooked mode after EOF")
	}
	if mock.setRawCount != 1 {
		t.Errorf("SetRaw called %d times, want 1 (entered once before EOF)", mock.setRawCount)
	}
}

// TestPersistentRawModeRestoresOnInterrupt asserts Ctrl+C restores the terminal
// and returns ErrInterrupted even in persistent mode.
func TestPersistentRawModeRestoresOnInterrupt(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("\x03") // Ctrl+C
	p := newTestPrompt(mock, WithPersistentRawMode())

	_, err := p.RunWithContext(context.Background())
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("Run error = %v, want ErrInterrupted", err)
	}
	if mock.restoreCount != 1 {
		t.Errorf("Restore called %d times after interrupt, want 1", mock.restoreCount)
	}
	if mock.rawMode {
		t.Error("terminal should be restored to cooked mode after interrupt")
	}
}

// TestPersistentRawModeStressManyLines drives many rapid consecutive lines with
// no inter-line delay and asserts none are lost and raw mode is entered only once,
// so the race the fix closes cannot silently regress.
func TestPersistentRawModeStressManyLines(t *testing.T) {
	t.Parallel()

	const lines = 200
	var sb strings.Builder
	for i := range lines {
		fmt.Fprintf(&sb, "line-%d\r", i)
	}

	mock := newMockTerminal(sb.String())
	p := newTestPrompt(mock, WithPersistentRawMode())

	for i := range lines {
		got, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Fatalf("Run %d returned error: %v", i, err)
		}
		want := fmt.Sprintf("line-%d", i)
		if got != want {
			t.Fatalf("Run %d = %q, want %q (input lost)", i, got, want)
		}
	}

	if mock.setRawCount != 1 {
		t.Errorf("SetRaw called %d times across %d lines, want 1", mock.setRawCount, lines)
	}
}

// TestWithPersistentRawModeOption asserts the option flips the config flag and is
// off by default.
func TestWithPersistentRawModeOption(t *testing.T) {
	t.Parallel()

	config := Config{}
	if config.PersistentRawMode {
		t.Error("PersistentRawMode should default to false")
	}
	WithPersistentRawMode()(&config)
	if !config.PersistentRawMode {
		t.Error("WithPersistentRawMode should set PersistentRawMode to true")
	}
}

// TestEnterExitRawModeIdempotent asserts the raw mode helpers are idempotent so
// they can be called from multiple cleanup paths without double toggling.
func TestEnterExitRawModeIdempotent(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("")
	p := newTestPrompt(mock)

	if err := p.enterRawMode(); err != nil {
		t.Fatalf("first enterRawMode error: %v", err)
	}
	if err := p.enterRawMode(); err != nil {
		t.Fatalf("second enterRawMode error: %v", err)
	}
	if mock.setRawCount != 1 {
		t.Errorf("SetRaw called %d times, want 1 (idempotent enter)", mock.setRawCount)
	}

	if err := p.exitRawMode(); err != nil {
		t.Fatalf("first exitRawMode error: %v", err)
	}
	if err := p.exitRawMode(); err != nil {
		t.Fatalf("second exitRawMode error: %v", err)
	}
	if mock.restoreCount != 1 {
		t.Errorf("Restore called %d times, want 1 (idempotent exit)", mock.restoreCount)
	}
}
