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
// Both spellings of the key are watched, because which one the terminal uses
// depends on the mode it is in: the byte 0x03 in raw mode, and SIGINT in cooked
// mode, which is where a prompt built without WithPersistentRawMode sits between
// one line and the next. While the watch is active the signal's default action
// is suppressed, so the interrupt cancels the work instead of killing the
// application; the stop function gives it back. An interrupt sent any other way
// -- kill -INT from another terminal -- is watched too, because there is nothing
// to tell it apart from the key.
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

	// Ctrl+C only reaches the reader as a byte while the terminal is in raw
	// mode, and outside a persistent session it is not: Run restores the
	// terminal when it returns, so the gap this watches over is a cooked
	// terminal, where the driver turns the key into SIGINT for the foreground
	// process group. Watching for the signal as well is what makes the key mean
	// the same thing in both modes. Registering for it also takes away its
	// default action, which was to kill the application in the middle of the
	// work this exists to cancel.
	//
	// The registration happens here rather than in the goroutine so the signal
	// is trapped by the time this returns, leaving no window where the key still
	// kills the caller.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	go func() {
		defer close(done)
		defer signal.Stop(signals)
		for {
			select {
			case <-stop:
				return
			case <-signals:
				cancel()
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
