package prompt

import (
	"context"
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
// what stops it from also cancelling the next line.
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

	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case res, ok := <-reads:
				if !ok {
					// Input ended. The next Run reports it; there is nothing here
					// to cancel the work for, because no key was pressed.
					return
				}
				if res.r == ctrlC {
					cancel()
					return
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
