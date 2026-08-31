package prompt

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// blockingTerminal models the terminal the library actually reads: ReadRune
// blocks until a key arrives, and Close is what ends it. The stock mock returns
// EOF as soon as its script runs out, so a reader over it always exits on its
// own and can never be caught outliving the session that started it.
type blockingTerminal struct {
	mu      sync.Mutex
	closed  bool
	release chan struct{}
	// readers counts the goroutines currently inside ReadRune, so a test can
	// see one that Close failed to end.
	readers atomic.Int64
	// interrupts says whether closing ends a read in progress, which is the
	// property the fix gives the real terminal on Unix.
	interrupts bool
}

func newBlockingTerminal(interrupts bool) *blockingTerminal {
	return &blockingTerminal{release: make(chan struct{}), interrupts: interrupts}
}

func (b *blockingTerminal) SetRaw() error                        { return nil }
func (b *blockingTerminal) Restore() error                       { return nil }
func (b *blockingTerminal) Size() (width, height int, err error) { return 80, 24, nil }

func (b *blockingTerminal) ReadRune() (rune, int, error) {
	b.readers.Add(1)
	defer b.readers.Add(-1)
	<-b.release
	return 0, 0, errors.New("terminal closed")
}

func (b *blockingTerminal) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	if b.interrupts {
		close(b.release)
	}
	return nil
}

func (b *blockingTerminal) interruptsReads() bool { return b.interrupts }

// TestCloseEndsTheSharedReader pins what Close guarantees over a terminal whose
// Close ends a read in progress: no goroutine of the prompt's is still reading
// the terminal once Close has returned.
//
// A reader that outlives its session is not idle. It is blocked on a descriptor
// the process has closed, and once that descriptor number is reused it reads
// whatever took it — which is how a second prompt opened after the first was
// closed received nothing at all.
func TestCloseEndsTheSharedReader(t *testing.T) {
	t.Parallel()

	terminal := newBlockingTerminal(true)
	p := newTestPromptOn(terminal)

	// What a REPL does while it runs the line it just read; this is what starts
	// the shared reader.
	_, stop := p.WatchInterrupt(context.Background())
	stop()

	// Wait for the reader to be inside the terminal read, so what Close has to
	// end is a read in progress rather than a goroutine yet to start one.
	if !waitFor(func() bool { return terminal.readers.Load() > 0 }) {
		t.Fatal("no reader reached the terminal, so this test would pass without proving anything")
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := terminal.readers.Load(); got != 0 {
		t.Errorf("%d reader goroutine(s) still reading the terminal after Close returned", got)
	}
}

// TestCloseDoesNotWaitOnATerminalItCannotInterrupt is the other half: where
// closing cannot end a read in progress, Close must still return. Waiting there
// would hang the session rather than leak a goroutine, which is worse.
func TestCloseDoesNotWaitOnATerminalItCannotInterrupt(t *testing.T) {
	t.Parallel()

	terminal := newBlockingTerminal(false)
	p := newTestPromptOn(terminal)

	_, stop := p.WatchInterrupt(context.Background())
	stop()

	done := make(chan error, 1)
	go func() { done <- p.Close() }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close blocked on a reader it had no way to end")
	}
	close(terminal.release) // let the abandoned reader go, so the test leaks nothing
}

// TestRealTerminalInterruptsReads checks the property the fix rests on, against
// the real terminal rather than a model of one: closing it ends a read in
// progress. It is skipped where there is no terminal to open, which is most CI
// runners without a pty.
func TestRealTerminalInterruptsReads(t *testing.T) {
	if _, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0); err != nil {
		t.Skipf("no controlling terminal: %v", err)
	}

	terminal, err := newRealTerminal()
	if err != nil {
		t.Skipf("cannot open a terminal here: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, _, err := terminal.ReadRune()
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)

	if err := terminal.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a read in progress survived Close, so a closed session keeps reading the terminal")
	}
}

// waitFor polls cond until it holds or a second passes, so a test can wait for
// a goroutine to reach a blocking call without sleeping for a fixed time it
// would have to over-estimate.
func waitFor(cond func() bool) bool { return waitUpTo(time.Second, cond) }

// waitUpTo is waitFor with the budget named, for a wait whose subject is a
// process rather than a goroutine.
func waitUpTo(budget time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return cond()
}
