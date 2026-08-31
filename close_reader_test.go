package prompt

import (
	"context"
	"errors"
	"os"
	"runtime"
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
	probe, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		t.Skipf("no controlling terminal: %v", err)
	}
	if err := probe.Close(); err != nil {
		t.Fatalf("closing the probe descriptor: %v", err)
	}

	terminal, err := newRealTerminal()
	if err != nil {
		t.Skipf("cannot open a terminal here: %v", err)
	}

	// started says the goroutine has reached the read; done carries what the
	// read returned. Both are needed: a read that returns before Close proves
	// nothing about Close, and a fixed sleep cannot tell the two apart. A read
	// that gives up on its own -- EAGAIN surfacing instead of being waited on,
	// which is what an unpollable terminal used to do -- is a failure here.
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, _, err := terminal.ReadRune()
		done <- err
	}()

	<-started
	// Nothing is typed into this terminal, so a read that has not returned
	// within this window is waiting rather than racing the goroutine's start.
	select {
	case err := <-done:
		t.Fatalf("the read returned on its own before Close, with %v; it was never waiting for a key", err)
	case <-time.After(500 * time.Millisecond):
	}

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

// TestRunWithContextReturnsWhileWaitingForAKey pins what the context is for. A
// terminal read cannot be canceled, so the context used to be noticed only
// between one key and the next: a deadline fired on the keystroke after it, and
// an idle prompt never returned at all -- which is exactly the case the
// documented timeout example is written for.
//
// The terminal here never delivers a key, so nothing but the context can end the
// read.
func TestRunWithContextReturnsWhileWaitingForAKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		context func(t *testing.T) (context.Context, context.CancelFunc)
		want    error
	}{
		{
			name: "a deadline that passes while the prompt waits",
			context: func(t *testing.T) (context.Context, context.CancelFunc) {
				t.Helper()
				return context.WithTimeout(t.Context(), 50*time.Millisecond)
			},
			want: context.DeadlineExceeded,
		},
		{
			name: "a cancel from another goroutine",
			context: func(t *testing.T) (context.Context, context.CancelFunc) {
				t.Helper()
				ctx, cancel := context.WithCancel(t.Context())
				go func() {
					time.Sleep(50 * time.Millisecond)
					cancel()
				}()
				return ctx, cancel
			},
			want: context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			terminal := newBlockingTerminal(true)
			p := newTestPromptOn(terminal)
			ctx, cancel := tt.context(t)
			defer cancel()

			returned := make(chan error, 1)
			go func() {
				_, err := p.RunWithContext(ctx)
				returned <- err
			}()

			select {
			case err := <-returned:
				if !errors.Is(err, tt.want) {
					t.Errorf("RunWithContext() error = %v, want %v", err, tt.want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("RunWithContext did not return: the context was never noticed while the read was waiting")
			}
			if err := p.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		})
	}
}

// TestRunWithContextTakesTheContextOverInputAlreadyHeld pins the order between
// the two: a context already done ends the call rather than the prompt reading
// on, which is what checking the context at the top of the read loop did.
func TestRunWithContextTakesTheContextOverInputAlreadyHeld(t *testing.T) {
	t.Parallel()

	p := newTestPrompt(newMockTerminal("hello\r"))
	p.stashTypeAhead('x')

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := p.RunWithContext(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("RunWithContext() error = %v, want context.Canceled", err)
	}
}

// settle gives goroutines that are on their way out a chance to finish, so a
// count taken after them is not a race with their exit.
func settle() {
	for range 20 {
		runtime.Gosched()
		time.Sleep(2 * time.Millisecond)
	}
}

// TestCloseLeavesNoGoroutineBehind walks the lifecycles that start a goroutine
// of the prompt's own and asserts that none of them outlives Close.
//
// A reader that outlives its session is not idle: it is blocked on a descriptor
// the process has closed, and once that number is reused it reads whatever took
// it, which is how a prompt opened after one was closed received nothing at all.
// Counting goroutines is coarse, but it is the one check that covers every way
// one can be started rather than the way a particular test happens to.
func TestCloseLeavesNoGoroutineBehind(t *testing.T) {
	settle()
	base := runtime.NumGoroutine()

	for range 50 {
		terminal := newBlockingTerminal(true)
		p := newTestPromptOn(terminal)
		_, stop := p.WatchInterrupt(context.Background())
		stop()
		if err := p.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
	settle()
	if got := runtime.NumGoroutine(); got > base+2 {
		t.Errorf("after 50 watch/stop/close cycles there are %d goroutines, started at %d", got, base)
	}

	// A watch that is never stopped.
	for range 50 {
		terminal := newBlockingTerminal(true)
		p := newTestPromptOn(terminal)
		// Deliberately dropped: a watch left running is what Close has to cope with.
		_, _ = p.WatchInterrupt(context.Background())
		if err := p.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
	settle()
	if got := runtime.NumGoroutine(); got > base+2 {
		t.Errorf("after 50 unstopped watches there are %d goroutines, started at %d", got, base)
	}

	// A cancellable RunWithContext, which now starts the shared reader.
	for range 50 {
		terminal := newBlockingTerminal(true)
		p := newTestPromptOn(terminal)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		if _, err := p.RunWithContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("RunWithContext() error = %v, want the deadline to have fired", err)
		}
		cancel()
		if err := p.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
	settle()
	if got := runtime.NumGoroutine(); got > base+2 {
		t.Errorf("after 50 cancelled runs there are %d goroutines, started at %d", got, base)
	}
}
