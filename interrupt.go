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
// was typed, so typing ahead keeps working. The Ctrl+C itself is taken, however
// many arrive, which is what stops one from also canceling the next line.
//
// The key reaches the process in one of two forms, and which one depends on the
// mode the terminal is in: the byte 0x03 while the prompt holds the terminal,
// and SIGINT once it has given it back, which is where a prompt built without
// WithPersistentRawMode sits between one line and the next. Both are watched.
// While the watch is active the signal's default action is taken away, so the
// interrupt cancels the work instead of killing the application; the stop
// function gives it back. That holds for every interrupt the watch sees, not
// only the first: the work has been told to stop and has not finished stopping,
// which is exactly when the key gets pressed again.
//
// The returned context is a child of the one passed in, so a caller whose own
// context is canceled still sees the work stop. It is canceled by the returned
// function too, so work cannot outlive the call it was started for.
//
// Watches may be nested. There is one watcher for the prompt however many are
// active, because the reader hands its runes to one receiver at a time and two
// receivers hold what they took in whichever order they are scheduled in, not
// the order it was typed in. An interrupt cancels every watch that is active,
// which is what an interrupt means with more than one: the work is nested, and
// canceling the inner half alone would leave the outer half running with nothing
// left to stop it.
//
// Do not call Run while a watch is active: the prompt reads one terminal, and a
// line editor and a watcher cannot both own it. Run after stop, which is the
// order a REPL naturally has.
func (p *Prompt) WatchInterrupt(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	p.watchMu.Lock()
	if p.watchers == nil {
		p.watchers = make(map[uint64]context.CancelFunc)
	}
	id := p.watchNext
	p.watchNext++
	p.watchers[id] = cancel
	if p.watchStop == nil {
		p.startWatcherLocked()
	}
	p.watchMu.Unlock()

	// A CancelFunc may be called more than once, so the watch is taken out of
	// the list exactly once however often the caller (or a defer plus an
	// explicit call) invokes it.
	var once sync.Once
	return ctx, func() {
		once.Do(func() { p.stopWatch(id) })
		cancel()
	}
}

// startWatcherLocked starts the goroutine that watches for the interrupt. It is
// called with watchMu held, by the watch that found the list empty.
//
// The signal registration happens here rather than inside the goroutine, so the
// signal is trapped by the time WatchInterrupt returns and there is no window in
// which the key still kills the caller. It is given back when the goroutine
// ends, which is when the last watch is stopped -- not when the first interrupt
// arrives, which would hand the default action back while the work was still
// winding down.
func (p *Prompt) startWatcherLocked() {
	reads := p.startInputReader()
	stop := make(chan struct{})
	done := make(chan struct{})
	p.watchStop, p.watchDone = stop, done

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
				p.cancelWatchers()
			case res, ok := <-reads:
				if !ok {
					// Input ended. There is nothing here to cancel the work for,
					// because no key was pressed, and the next Run reports the
					// ending. The watch goes on: what it promised about the
					// signal lasts until it is stopped, and a nil channel is
					// never ready, so this waits on the other two.
					reads = nil
					continue
				}
				if res.r == ctrlC {
					p.cancelWatchers()
					continue
				}
				p.stashTypeAhead(res.r)
			}
		}
	}()
}

// cancelWatchers cancels every watch that is active. Canceling one that already
// is does nothing, which is what a second interrupt amounts to.
func (p *Prompt) cancelWatchers() {
	p.watchMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(p.watchers))
	for _, cancel := range p.watchers {
		cancels = append(cancels, cancel)
	}
	p.watchMu.Unlock()

	// Outside the lock: a cancel runs whatever the caller hung off the context,
	// and that must not be holding the lock the watcher needs to stash a rune.
	for _, cancel := range cancels {
		cancel()
	}
}

// endWatching stops the watcher however many watches are still registered. It is
// what Close does: the session is over, so the signal goes back to its default
// action and nothing is left reading the terminal on the prompt's behalf.
//
// The watch's own stop function and this one both take the channel out from
// under the lock before closing it, so exactly one of them closes it.
func (p *Prompt) endWatching() {
	p.watchMu.Lock()
	stop, done := p.watchStop, p.watchDone
	p.watchStop, p.watchDone = nil, nil
	p.watchMu.Unlock()

	if stop == nil {
		return
	}
	close(stop)
	<-done
}

// stopWatch takes one watch out of the list and, if it was the last, ends the
// watcher and waits for it: no rune can be stashed after that, so the next Run
// sees a settled buffer.
func (p *Prompt) stopWatch(id uint64) {
	p.watchMu.Lock()
	delete(p.watchers, id)
	stop, done := p.watchStop, p.watchDone
	last := len(p.watchers) == 0
	if last {
		p.watchStop, p.watchDone = nil, nil
	}
	p.watchMu.Unlock()

	if !last || stop == nil {
		return
	}
	close(stop)
	<-done
}
