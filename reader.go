package prompt

import (
	"context"
	"errors"
	"io"
)

// This file holds the path a keystroke takes from the terminal to the line
// editor: the shared reader goroutine, the runes held back for the next read,
// and the ways a read ends. It is one surface because only one reader may own
// the terminal at a time: a watcher and the line editor reading at once would
// each take half of what was typed.

// readResult is one rune from the shared input reader.
type readResult struct {
	r rune
}

// readRuneContext reads the next rune and gives up when ctx is done.
//
// A terminal read cannot be canceled, so a context can only be noticed between
// one key and the next: checking it before a blocking read and then waiting made
// a deadline fire on the keystroke after it rather than on time, and an idle
// prompt never returned at all. Where the context can actually be canceled the
// read therefore moves to the shared reader goroutine, whose channel can be
// waited on alongside the context.
//
// A context that can never be canceled -- context.Background(), which is what
// Run passes -- keeps reading the terminal directly and starts no goroutine.
// That is deliberate: on Windows the read goes through go-tty and Close does not
// wait for the reader, so a session that never asked for cancellation should not
// grow a goroutine that nothing can interrupt.
func (p *Prompt) readRuneContext(ctx context.Context) (rune, error) {
	done := ctx.Done()
	if done == nil {
		return p.readRune()
	}
	// A context already done takes precedence over input already held, which is
	// what checking it at the top of the read loop used to do.
	select {
	case <-done:
		return 0, ctx.Err()
	default:
	}
	if r, ok := p.takePending(); ok {
		return r, nil
	}
	reads := p.startInputReader()
	select {
	case <-done:
		return 0, ctx.Err()
	case res, ok := <-reads:
		if !ok {
			return 0, p.readErr
		}
		return res.r, nil
	}
}

// endOfInput reports whether err ends this prompt's input rather than being a
// failure to report. The terminal saying so is one way; the other is Close,
// which ends the read a Run is waiting in -- from the reader's side that is an
// error, and to the caller it is the same ending as EOF.
func (p *Prompt) endOfInput(err error) bool {
	return errors.Is(err, io.EOF) || p.closed.Load()
}

func (p *Prompt) readRune() (rune, error) {
	if r, ok := p.takePending(); ok {
		return r, nil
	}
	// Once a watcher has started the shared reader, every rune must come from it:
	// a second reader on the same terminal would take bytes the other was waiting
	// for.
	if p.reads != nil {
		res, ok := <-p.reads
		if !ok {
			return 0, p.readErr
		}
		return res.r, nil
	}
	r, _, err := p.terminal.ReadRune()
	return r, err
}

// takePending removes and returns the oldest rune held back, if any.
func (p *Prompt) takePending() (rune, bool) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	if len(p.pending) == 0 {
		return 0, false
	}
	r := p.pending[0]
	p.pending = p.pending[1:]
	return r, true
}

// unreadRune pushes a rune back so the next readRune returns it. It is used when
// a rune read ahead turns out to be input rather than part of a key sequence, so
// it goes to the front: it was read before everything already held.
func (p *Prompt) unreadRune(r rune) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	p.pending = append([]rune{r}, p.pending...)
}

// stashTypeAhead holds a rune read while WatchInterrupt was watching, to be
// delivered to the next Run. It goes to the back, because it was typed after
// everything already held.
func (p *Prompt) stashTypeAhead(r rune) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	p.pending = append(p.pending, r)
}

// errReaderClosed is what a read reports once Close has ended the session.
var errReaderClosed = errors.New("prompt: input closed")

// startInputReader starts the goroutine that reads the terminal into a channel,
// and returns that channel. Reading in one place is what lets a watcher and the
// line editor take turns without either losing a keystroke to the other.
//
// The goroutine has two ways out, because it has two ways to wait. Blocked on the
// terminal it ends when the read fails, which closing the terminal causes.
// Blocked on the channel — nothing is consuming keystrokes between a stopped
// watch and the next Run, and someone can hold a key down — it ends on the signal
// Close sends, which no amount of closing the terminal would deliver. Either way
// the channel is closed and every later read reports why.
//
// It cannot be stopped while a read is in progress, which is why there is no
// pause: a terminal read cannot be canceled on every platform, and a goroutine
// abandoned mid-read would eat the next key.
func (p *Prompt) startInputReader() <-chan readResult {
	p.readerOnce.Do(func() {
		// Buffered so a burst typed during long work does not block the reader,
		// which would leave the terminal unread and lose the keys after it.
		reads := make(chan readResult, 1024)
		stop := make(chan struct{})
		done := make(chan struct{})
		p.reads = reads
		p.readerStop = stop
		p.readerDone = done
		go func() {
			defer close(done)
			for {
				r, _, err := p.terminal.ReadRune()
				if err != nil {
					p.readErr = err
					close(reads)
					return
				}
				select {
				case reads <- readResult{r: r}:
				case <-stop:
					p.readErr = errReaderClosed
					close(reads)
					return
				}
			}
		}()
	})
	return p.reads
}

// readInterrupter is implemented by a terminal whose Close ends a read in
// progress. Only such a terminal can be waited on: where a read cannot be
// interrupted, waiting for the reader to notice would hang Close forever.
type readInterrupter interface {
	interruptsReads() bool
}

// awaitReaderExit waits for the shared reader goroutine to finish, but only
// where closing the terminal is known to have ended the read it was in.
//
// Waiting matters because a reader that outlives Close is not idle. It is
// blocked on a descriptor the process has closed, and once that descriptor
// number is reused — running a child process is enough to cause that — the
// goroutine is reading whatever now holds it, taking input meant for something
// else. A prompt opened after one was closed received nothing at all, because
// the previous session's reader had the new session's terminal.
//
// Where the terminal cannot be interrupted the goroutine is left to end when
// its read returns, which is what happened before this and is still better than
// a Close that never returns.
func (p *Prompt) awaitReaderExit() {
	if p.readerDone == nil {
		return
	}
	if ri, ok := p.terminal.(readInterrupter); !ok || !ri.interruptsReads() {
		return
	}
	<-p.readerDone
}

// stopInputReader releases the shared reader, if one was started. A goroutine
// waiting to hand over a rune has no other way out: closing the terminal ends a
// read in progress, but says nothing to one blocked on the channel.
func (p *Prompt) stopInputReader() {
	if p.readerStop == nil {
		return
	}
	p.readerStopOnce.Do(func() { close(p.readerStop) })
}
