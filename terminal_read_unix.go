//go:build !windows

package prompt

import (
	"errors"
	"io"
	"sync"

	"golang.org/x/sys/unix"
)

// errTerminalClosed is what a read reports once the terminal it was waiting on
// has been closed.
var errTerminalClosed = errors.New("prompt: terminal closed")

// ttyReader reads the controlling terminal and can be interrupted.
//
// It exists because closing go-tty's descriptor does not end a read in
// progress. go-tty opens /dev/tty with os.Open and then calls Fd() on it, to
// pass the raw descriptor to the termios ioctls. Fd() takes the file out of the
// runtime's poller and puts it back in blocking mode, and a blocking read is
// what nothing can interrupt: closing the file leaves the goroutine sitting in
// read(2) on a descriptor number the kernel has taken back. Once that number is
// reused -- which running a child process is enough to cause -- the abandoned
// goroutine is reading whatever now holds it.
//
// Handing the descriptor to the runtime's poller instead is not portable
// enough. os.OpenFile with O_NONBLOCK polls only a FIFO on the BSDs, and
// os.NewFile over an already non-blocking descriptor does not poll a terminal
// there either: both return EAGAIN from every read on macOS rather than waiting
// for a key. So the waiting is done here. The descriptor is non-blocking, and a
// read with nothing to return waits in poll(2) on the terminal and on a pipe
// that Close writes to, whichever speaks first.
//
// Reading here rather than through go-tty changes nothing else: /dev/tty is the
// same controlling terminal go-tty opens, and raw mode is applied to os.Stdin,
// which is that terminal too.
type ttyReader struct {
	fd    int // /dev/tty, in non-blocking mode
	wakeR int // read end of the pipe Close writes to
	wakeW int // write end of that pipe

	// mu is held for reading while a read is in progress and for writing while
	// closing, so no descriptor is closed under a reader. Close signals the pipe
	// before taking the lock, so the read it waits for returns at once.
	mu     sync.RWMutex
	closed bool
}

// openTTYReader opens the controlling terminal for reading, or returns nil when
// it cannot. A nil reader leaves reading to go-tty: a session that cannot be
// interrupted is better than one that cannot read.
func openTTYReader() *ttyReader {
	fd, err := unix.Open("/dev/tty", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return nil
	}

	var pipe [2]int
	if err := unix.Pipe(pipe[:]); err != nil {
		_ = unix.Close(fd)
		return nil
	}
	for _, p := range pipe {
		if err := unix.SetNonblock(p, true); err != nil {
			_ = unix.Close(fd)
			_ = unix.Close(pipe[0])
			_ = unix.Close(pipe[1])
			return nil
		}
	}
	return &ttyReader{fd: fd, wakeR: pipe[0], wakeW: pipe[1]}
}

// Read fills p with what the terminal has, waiting for a key when it has
// nothing. It reports errTerminalClosed once Close has been called, whether the
// read was already waiting or started after.
func (r *ttyReader) Read(p []byte) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for {
		if r.closed {
			return 0, errTerminalClosed
		}
		n, err := unix.Read(r.fd, p)
		switch {
		case err == nil && n > 0:
			return n, nil
		case err == nil:
			return 0, io.EOF
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EAGAIN):
			if err := r.wait(); err != nil {
				return 0, err
			}
		default:
			return 0, err
		}
	}
}

// wait blocks until the terminal has something to read, or Close has written to
// the pipe.
func (r *ttyReader) wait() error {
	fds := []unix.PollFd{
		{Fd: int32(r.fd), Events: unix.POLLIN},    //nolint:gosec // a descriptor fits an int32
		{Fd: int32(r.wakeR), Events: unix.POLLIN}, //nolint:gosec // a descriptor fits an int32
	}
	for {
		_, err := unix.Poll(fds, -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if fds[1].Revents != 0 {
			return errTerminalClosed
		}
		return nil
	}
}

// Close ends a read in progress and releases the descriptors. It is idempotent.
//
// The pipe is written to before the lock is taken, so a reader waiting in
// poll(2) returns and gives up its read lock. Taking the write lock is
// therefore a wait for that reader rather than a deadlock, and nothing is
// closed while a reader could still be using it.
func (r *ttyReader) Close() error {
	r.mu.RLock()
	running := !r.closed
	r.mu.RUnlock()
	if running {
		var b [1]byte
		// A pipe already holding a byte says "wake up" just as well, so a full
		// one is not an error worth reporting.
		if _, err := unix.Write(r.wakeW, b[:]); err != nil && !errors.Is(err, unix.EAGAIN) {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return errors.Join(unix.Close(r.fd), unix.Close(r.wakeR), unix.Close(r.wakeW))
}

// openReadHandle opens the terminal the prompt reads keystrokes from, or
// returns nil to leave reading to go-tty.
func openReadHandle() terminalReader {
	if r := openTTYReader(); r != nil {
		return r
	}
	return nil
}
