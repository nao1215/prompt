//go:build !windows

package prompt

import (
	"bufio"
	"os"
	"syscall"
)

// openReadHandle opens the descriptor the prompt reads keystrokes from, or
// returns nil to leave reading to go-tty.
//
// It exists because closing go-tty's descriptor does not end a read in progress
// on Unix. go-tty opens /dev/tty with os.Open and then calls Fd() on it, to pass
// the raw descriptor to the termios ioctls. Fd() takes the file out of the
// runtime's poller and puts it back in blocking mode, and a blocking read is
// what nothing can interrupt: closing the file leaves the goroutine sitting in
// read(2) on a descriptor number the kernel has taken back. Once that number is
// reused -- which running a child process is enough to cause -- the abandoned
// goroutine is reading whatever now holds it.
//
// O_NONBLOCK is what keeps this descriptor in the poller. Fd() leaves a file
// opened non-blocking in the poller, so even a later Fd() call cannot strand a
// read on it, and Close returns ErrFileClosing to a reader that is waiting.
//
// Reading here rather than through go-tty changes nothing else: /dev/tty is the
// same controlling terminal go-tty opens, and raw mode is applied to os.Stdin,
// which is that terminal too.
func openReadHandle() *os.File {
	f, err := os.OpenFile("/dev/tty", os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		// No controlling terminal to open separately. go-tty found one, so let
		// it do the reading; a session that cannot be interrupted is better than
		// one that cannot read.
		return nil
	}
	return f
}

// newReadBuffer wraps the read handle for rune-wise reading. bufio decodes
// UTF-8, which is what ReadRune has to return.
func newReadBuffer(f *os.File) *bufio.Reader {
	if f == nil {
		return nil
	}
	return bufio.NewReader(f)
}
