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
// embedded options (for example WithPersistentRawMode).
func newTestPrompt(mock *mockTerminal, opts ...Option) *Prompt {
	return newTestPromptOn(mock, opts...)
}

// newTestPromptOn is newTestPrompt over any terminal, for a test that needs one
// the stock mock cannot model — a read that blocks until the terminal is
// closed, for instance.
func newTestPromptOn(terminal terminalInterface, opts ...Option) *Prompt {
	config := options{Prefix: "$ "}
	for _, option := range opts {
		option(&config)
	}
	// The key map comes from the config when the caller set one, the way New
	// does it. Hardcoding the default here made WithKeyMap silently do nothing
	// in a test.
	keyMap := config.KeyMap
	if keyMap == nil {
		keyMap = NewDefaultKeyMap()
	}
	var output bytes.Buffer
	return &Prompt{
		config:   config,
		terminal: terminal,
		keyMap:   keyMap,
		output:   &output,
		buffer:   []rune{},
		cursor:   0,
		history:  []string{},
		renderer: newRenderer(&output, ThemeDefault, terminal),
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
		got, err := p.Run(context.Background())
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

	got, err := p.Run(context.Background())
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
		got, err := p.Run(context.Background())
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
		if _, err := p.Run(context.Background()); err != nil {
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

	got, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}
	if got != "done" {
		t.Errorf("first Run = %q, want %q", got, "done")
	}
	if mock.restoreCount != 0 {
		t.Errorf("Restore called %d times after a normal submit, want 0 in persistent mode", mock.restoreCount)
	}

	_, err = p.Run(context.Background())
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

// TestPersistentRawModeKeepsTerminalOnInterrupt asserts Ctrl+C returns
// ErrInterrupted without releasing the terminal in persistent mode. A REPL
// treats the interrupt as "discard this line" and calls Run again, so releasing
// the terminal there would reopen the mode-switch window persistent raw mode
// exists to close. Close still restores it.
func TestPersistentRawModeKeepsTerminalOnInterrupt(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("\x03") // Ctrl+C
	p := newTestPrompt(mock, WithPersistentRawMode())

	_, err := p.Run(context.Background())
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("Run error = %v, want ErrInterrupted", err)
	}
	if mock.restoreCount != 0 {
		t.Errorf("Restore called %d times after interrupt, want 0 in persistent mode", mock.restoreCount)
	}
	if !mock.rawMode {
		t.Error("terminal should stay in raw mode after an interrupt in persistent mode")
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if mock.restoreCount != 1 {
		t.Errorf("Restore called %d times after Close, want 1", mock.restoreCount)
	}
	if mock.rawMode {
		t.Error("terminal should be restored to cooked mode by Close")
	}
}

// TestDefaultModeRestoresOnInterrupt keeps the single-shot contract: without
// persistent raw mode every Run restores the terminal before returning,
// interrupt included.
func TestDefaultModeRestoresOnInterrupt(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("\x03") // Ctrl+C
	p := newTestPrompt(mock)

	_, err := p.Run(context.Background())
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

// TestInterruptedLineIsDiscardedAndSessionContinues models the REPL loop a shell
// runs: a half-typed line is interrupted, the next Run starts empty and reads the
// next line, and raw mode was acquired once for the whole session.
func TestInterruptedLineIsDiscardedAndSessionContinues(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("SELE\x03SELECT 1;\r")
	p := newTestPrompt(mock, WithPersistentRawMode())
	// The session holds the terminal across calls, so it is closed here rather
	// than left raw on any exit path.
	defer func() {
		if err := p.Close(); err != nil {
			t.Errorf("Close returned error: %v", err)
		}
	}()

	if _, err := p.Run(context.Background()); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("first Run error = %v, want ErrInterrupted", err)
	}

	got, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}
	if want := "SELECT 1;"; got != want {
		t.Errorf("line after the interrupt = %q, want %q (the discarded line must not leak in)", got, want)
	}
	if mock.setRawCount != 1 {
		t.Errorf("SetRaw called %d times across the interrupted session, want 1", mock.setRawCount)
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
		got, err := p.Run(context.Background())
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

	config := options{}
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

// TestRunAfterCloseLeavesTheTerminalAlone pins what a Run on a closed prompt may
// do: report that the session is over, and touch nothing on the way.
//
// It used to enter raw mode first and learn the terminal was gone only when it
// read. Raw mode is set on a descriptor Close never touches, so it succeeded,
// and in a persistent session nothing restored it: the per-call cleanup is
// skipped there by design, and Close -- the one thing that would have restored
// it -- had already run. The application exited leaving the user's shell with no
// echo, no line editing and a dead Ctrl+C.
func TestRunAfterCloseLeavesTheTerminalAlone(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("one\r")
	p := newTestPrompt(mock, WithPersistentRawMode())

	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("the first Run returned error: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	setRawBefore := mock.setRawCount

	_, err := p.Run(context.Background())
	if !errors.Is(err, ErrEOF) {
		t.Errorf("Run after Close returned %v, want ErrEOF", err)
	}
	if mock.setRawCount != setRawBefore {
		t.Errorf("Run after Close entered raw mode %d time(s), want 0", mock.setRawCount-setRawBefore)
	}
	if mock.rawMode {
		t.Error("a Run on a closed prompt left the terminal in raw mode")
	}
}
