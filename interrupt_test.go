package prompt

import (
	"bytes"
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
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

// TestCloseReleasesAReaderBlockedOnHandover covers the reader's other way of
// waiting. Between a stopped watch and the next Run nobody collects keystrokes,
// so a reader with a full buffer sits on the channel — where closing the
// terminal, which only ends a read in progress, would never reach it. Close must
// still let it go.
func TestCloseReleasesAReaderBlockedOnHandover(t *testing.T) {
	t.Parallel()

	// More runes than the channel holds, so the reader is left mid-handover.
	mock := newMockTerminal(strings.Repeat("x", 4096))
	p := newTestPrompt(mock, WithPersistentRawMode())

	_, stop := p.WatchInterrupt(context.Background())
	stop()

	if err := p.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	// Nothing here drains the channel: draining would let the reader go on its
	// own and prove nothing. The goroutine must end because Close said so.
	select {
	case <-p.readerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the reader is still waiting to hand over a rune nobody will take")
	}
}

// TestWatchInterruptCancelsOnSIGINT covers the mode the terminal is actually in
// while a watch runs in a default session. Ctrl+C is only a byte in raw mode,
// and outside a persistent session Run gives raw mode back before it returns, so
// the key the watcher is looking for is turned into SIGINT by the terminal
// driver and never reaches the reader at all. Watching for the signal too is
// what makes the documented pattern work in both modes -- and registering for it
// is also what stops the default action from killing the application in the
// middle of the work.
//
// It does not call t.Parallel, on purpose: the signal goes to the whole test
// process, and a parallel test with a watch of its own would see it too. Go
// pauses parallel tests until the sequential ones have finished, so a
// sequential test is the one place a signal can be sent without reaching them.
func TestWatchInterruptCancelsOnSIGINT(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a process cannot send itself SIGINT on Windows")
	}

	// A blocking terminal, because a mock that ends its script reports EOF and
	// the watcher returns before any signal could arrive.
	terminal := newBlockingTerminal(true)
	p := newTestPromptOn(terminal)
	defer func() { _ = p.Close() }()

	ctx, stop := p.WatchInterrupt(context.Background())
	defer stop()

	// Wait for the watch to be watching. Sending the signal before it registers
	// would kill the test binary rather than cancel the context.
	if !waitFor(func() bool { return terminal.readers.Load() > 0 }) {
		t.Fatal("the watch never reached the terminal, so the signal would land on nothing")
	}
	self, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("finding the test process: %v", err)
	}
	if err := self.Signal(os.Interrupt); err != nil {
		t.Fatalf("sending SIGINT to the test process: %v", err)
	}

	waitForDone(ctx, t, "SIGINT during watched work")
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Errorf("ctx.Err() = %v, want context.Canceled", ctx.Err())
	}
}

// TestNestedWatchesKeepTheOrderOfWhatWasTyped types through two watches at once.
//
// Every watch used to run a receiver of its own on the shared reader's channel.
// A channel hands its receivers the runes in order, but what a receiver does
// next is the scheduler's business, so the goroutine that took the second rune
// could stash it before the goroutine that took the first: the keys reached the
// next Run in an order nobody typed them in. The race detector sees nothing --
// every access is under the lock the type-ahead is kept behind -- so it takes a
// test that reads the line back.
func TestNestedWatchesKeepTheOrderOfWhatWasTyped(t *testing.T) {
	t.Parallel()

	const typed = "abcdef"

	// Enough runs that a scheduling order which only sometimes goes wrong is
	// caught: at two watches the failure showed up about once in twenty-five.
	for i := range 300 {
		var out bytes.Buffer
		terminal := newMockTerminal(typed + "\r")
		p := newTestPromptOn(terminal)
		p.output = &out
		p.renderer = newRenderer(&out, ThemeDefault, terminal)

		_, stopOuter := p.WatchInterrupt(context.Background())
		_, stopInner := p.WatchInterrupt(context.Background())
		stopInner()
		stopOuter()

		line, err := p.Run()
		if err != nil {
			t.Fatalf("run %d: Run() error = %v", i, err)
		}
		if line != typed {
			t.Fatalf("run %d: %q was typed through two watches and came back as %q", i, typed, line)
		}
	}
}

// TestCloseEndsAWatchNobodyStopped closes a prompt whose watch was never
// stopped. The watch holds the interrupt away from its default action and reads
// the terminal on the prompt's behalf, so one left running after the session is
// over holds the interrupt for the rest of the process.
func TestCloseEndsAWatchNobodyStopped(t *testing.T) {
	t.Parallel()

	terminal := newBlockingTerminal(true)
	p := newTestPromptOn(terminal)
	// The stop function is deliberately dropped, which is what a caller that
	// returns early does.
	_, _ = p.WatchInterrupt(context.Background())

	p.watchMu.Lock()
	done := p.watchDone
	p.watchMu.Unlock()
	if done == nil {
		t.Fatal("the watch started no watcher, so this test would prove nothing")
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-done:
	default:
		t.Error("Close left the watcher running, so the interrupt is still held away from its default action")
	}
}

// TestAWatchOutlivesTheInputItWatches ends the input under an active watch. What
// the watch promised about the interrupt lasts until the caller stops it: the
// keyboard having nothing more to say is not the caller saying its work is done.
func TestAWatchOutlivesTheInputItWatches(t *testing.T) {
	t.Parallel()

	terminal := newMockTerminal("") // end of input on the first read
	p := newTestPromptOn(terminal)
	_, stop := p.WatchInterrupt(context.Background())

	p.watchMu.Lock()
	done := p.watchDone
	p.watchMu.Unlock()
	if done == nil {
		t.Fatal("the watch started no watcher, so this test would prove nothing")
	}
	// The reader goroutine is gone, which means the channel the watcher reads
	// from is closed and it has had every chance to act on that.
	<-p.readerDone

	select {
	case <-done:
		t.Error("the watch ended with the input, so the signal went back to its default while the caller still held the watch")
	case <-time.After(200 * time.Millisecond):
	}

	stop()
	select {
	case <-done:
	default:
		t.Error("stopping the watch left the watcher running")
	}
}
