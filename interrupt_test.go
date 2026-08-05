package prompt

import (
	"context"
	"errors"
	"testing"
	"time"
)

// waitForDone fails the test if ctx is not canceled quickly. The wait is only a
// deadline: the cancellation happens as soon as the watcher reads the byte, so a
// passing run does not sleep for it.
func waitForDone(ctx context.Context, t *testing.T, what string) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: context was not canceled", what)
	}
}

// TestWatchInterruptCancelsOnCtrlC covers the reason WatchInterrupt exists: work
// running between prompts (a long query) has nobody reading the terminal, so
// Ctrl+C could not reach it. The returned context is canceled when the key
// arrives, which is what lets the caller stop that work.
func TestWatchInterruptCancelsOnCtrlC(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("\x03")
	p := newTestPrompt(mock, WithPersistentRawMode())

	ctx, stop := p.WatchInterrupt(context.Background())
	defer stop()

	waitForDone(ctx, t, "Ctrl+C during watched work")
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Errorf("ctx.Err() = %v, want context.Canceled", ctx.Err())
	}
}

// TestWatchInterruptKeepsTypeAheadForTheNextRun pins the other half of the
// contract: keys typed while the work runs are the user's input, not the
// watcher's to eat. They must reach the next Run, in the order they were typed,
// with the interrupt removed.
func TestWatchInterruptKeepsTypeAheadForTheNextRun(t *testing.T) {
	t.Parallel()

	// "ab" is typed while the work runs, then Ctrl+C stops it, then the rest of
	// the line and Enter arrive.
	mock := newMockTerminal("ab\x03cd\r")
	p := newTestPrompt(mock, WithPersistentRawMode())

	ctx, stop := p.WatchInterrupt(context.Background())
	waitForDone(ctx, t, "Ctrl+C after type-ahead")
	stop()

	got, err := p.RunWithContext(context.Background())
	if err != nil {
		t.Fatalf("Run after the interrupt returned error: %v", err)
	}
	if want := "abcd"; got != want {
		t.Errorf("line after the watched work = %q, want %q (type-ahead must survive in order)", got, want)
	}
}

// TestWatchInterruptWithoutInterruptLosesNothing covers the ordinary case: the
// work finishes on its own and the watcher stops. Whatever was typed meanwhile
// belongs to the next line, whether the watcher had already read it or it was
// still in the terminal.
func TestWatchInterruptWithoutInterruptLosesNothing(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("xy\r")
	p := newTestPrompt(mock, WithPersistentRawMode())

	_, stop := p.WatchInterrupt(context.Background())
	stop()

	got, err := p.RunWithContext(context.Background())
	if err != nil {
		t.Fatalf("Run after the watched work returned error: %v", err)
	}
	if want := "xy"; got != want {
		t.Errorf("line after the watched work = %q, want %q", got, want)
	}
}

// TestWatchInterruptStopsAtEndOfInput asserts the watcher does not hold the
// session open when input ends: it returns, and the next Run reports EOF as it
// always does.
func TestWatchInterruptStopsAtEndOfInput(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("") // EOF immediately
	p := newTestPrompt(mock, WithPersistentRawMode())

	_, stop := p.WatchInterrupt(context.Background())
	stop()

	if _, err := p.RunWithContext(context.Background()); !errors.Is(err, ErrEOF) {
		t.Fatalf("Run error = %v, want ErrEOF", err)
	}
}

// TestWatchInterruptInheritsParentCancellation keeps the returned context a
// child of the caller's: a shell whose own context is canceled (a signal, a
// deadline) must see the work stop even with no key pressed.
func TestWatchInterruptInheritsParentCancellation(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("no interrupt here")
	p := newTestPrompt(mock, WithPersistentRawMode())

	parent, cancelParent := context.WithCancel(context.Background())
	ctx, stop := p.WatchInterrupt(parent)
	defer stop()

	cancelParent()
	waitForDone(ctx, t, "parent cancellation")
}

// TestWatchInterruptCancelsAfterStop keeps the returned context from outliving
// the work it was made for.
func TestWatchInterruptCancelsAfterStop(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("z")
	p := newTestPrompt(mock, WithPersistentRawMode())

	ctx, stop := p.WatchInterrupt(context.Background())
	stop()
	waitForDone(ctx, t, "stop()")
}
