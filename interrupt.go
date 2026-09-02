package prompt

import (
	"context"
	"os"
	"os/signal"
	"sync"
)

// ctrlC is the byte a terminal sends for Ctrl+C in raw mode, where it is a
// keystroke rather than a signal.
const ctrlC = '\x03'

// WatchInterrupt watches the terminal for Ctrl+C while the caller runs work
// between prompts, and returns a context that is canceled when the key arrives.
// Call the returned function when the work is finished.
//
// It exists because a REPL spends most of its time not reading the terminal. Run
// returns as soon as a line is submitted, and while the application executes that
// line — a query, an import, anything slow — nothing is consuming input. In raw
// mode Ctrl+C is a byte rather than a signal, so it simply waits in the input
// buffer: it cannot stop the work, and it is read as part of the next line once
// the work is over. Watching for it here is what lets the caller cancel the work
// the moment the key is pressed:
//
//	ctx, stop := p.WatchInterrupt(ctx)
//	err := runQuery(ctx, line)
//	stop()
//	if errors.Is(err, context.Canceled) {
//		fmt.Print("canceled\r\n")
//	}
//
// Everything else typed while the work runs is the user's input, not the
// watcher's to consume: it is held and delivered to the next Run in the order it
// was typed, so typing ahead keeps working. The Ctrl+C itself is taken, which is
// what stops it from also canceling the next line.
//
// The key reaches the process in one of two forms, and which one depends on the
// mode the terminal is in: the byte 0x03 while the prompt holds the terminal,
// and SIGINT once it has given it back, which is where a prompt built without
// WithPersistentRawMode sits between one line and the next. Both are watched.
// While the watch is active the signal's default action is taken away, so the
// interrupt cancels the work instead of killing the application; the stop
// function gives it back. That holds for every interrupt the watch sees, not
// only the first: the work has been told to stop and has not finished stopping,
// which is exactly when the key gets pressed again. The context is canceled
// once; what comes after it is held rather than acted on. An interrupt sent any other way -- kill -INT from
// another terminal -- cancels the work too, because there is nothing to tell it
// apart from the key.
//
// The returned context is a child of the one passed in, so a caller whose own
// context is canceled still sees the work stop. It is canceled by the returned
// function too, so work cannot outlive the call it was started for.
//
// Do not call Run while a watch is active: the prompt reads one terminal, and a
// line editor and a watcher cannot both own it. Run after stop, which is the
// order a REPL naturally has.
func (p *Prompt) WatchInterrupt(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	reads := p.startInputReader()
	stop := make(chan struct{})
	done := make(chan struct{})

	// The registration happens here rather than in the goroutine, so the signal
	// is trapped by the time this returns and there is no window in which the
	// key still kills the caller.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	go func() {
		defer close(done)
		defer signal.Stop(signals)
		// The watch runs until it is stopped, not until it has something to
		// cancel for. Returning on the first interrupt gave the signal's default
		// action back while the caller's work was still winding down, so a
		// second Ctrl+C -- which is what a person presses when the first one
		// appears to have done nothing -- killed the application with its work
		// half done and the prompt's own cleanup never run.
		//
		// After the first one there is nothing left to cancel: further signals
		// are dropped, which is what a channel of one that nobody drains does,
		// and further runes are the user's input for the next Run, as they were
		// before it.
		interrupted := false
		for {
			select {
			case <-stop:
				return
			case <-signals:
				if !interrupted {
					cancel()
					interrupted = true
				}
			case res, ok := <-reads:
				if !ok {
					// Input ended. The next Run reports it; there is nothing here
					// to cancel the work for, because no key was pressed.
					return
				}
				if res.r == ctrlC && !interrupted {
					cancel()
					interrupted = true
					continue
				}
				p.stashTypeAhead(res.r)
			}
		}
	}()

	// A CancelFunc may be called more than once, so the stop channel is closed
	// exactly once however often the caller (or a defer plus an explicit call)
	// invokes it.
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			close(stop)
			<-done // no rune can be stashed after this, so the next Run sees a settled buffer
		})
		cancel()
	}
}
