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
// It exists because closing go-tty's descriptor does not end a read in progress.
// go-tty opens /dev/tty with os.Open and then calls Fd() on it, to pass the raw
// descriptor to the termios ioctls. Fd() takes the file out of the runtime's
// poller and puts it back in blocking mode, and a blocking read is what nothing
// can interrupt: closing the file leaves the goroutine sitting in read(2) on a
// descriptor number the kernel has taken back. Once that number is reused --
// which running a child process is enough to cause -- the abandoned goroutine
// is reading whatever now holds it.
//
// The descriptor is put in non-blocking mode before os.NewFile sees it, which
// is what makes the runtime poll it: os.NewFile documents that it returns a
// pollable file for a descriptor already in non-blocking mode, and a polled
// read is one Close can end. os.OpenFile with O_NONBLOCK is not the same thing
// and does not work here -- on the BSDs it makes only a FIFO pollable, so a
// terminal opened that way returns EAGAIN instead of waiting.
//
// Reading here rather than through go-tty changes nothing else: /dev/tty is the
// same controlling terminal go-tty opens, and raw mode is applied to os.Stdin,
// which is that terminal too.
func openReadHandle() *os.File {
	fd, err := syscall.Open("/dev/tty", syscall.O_RDONLY, 0)
	if err != nil {
		// No controlling terminal to open separately. go-tty found one, so let
		// it do the reading; a session that cannot be interrupted is better than
		// one that cannot read.
		return nil
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		_ = syscall.Close(fd)
		return nil
	}
	return os.NewFile(uintptr(fd), "/dev/tty")
}

// newReadBuffer wraps the read handle for rune-wise reading. bufio decodes
// UTF-8, which is what ReadRune has to return.
func newReadBuffer(f *os.File) *bufio.Reader {
	if f == nil {
		return nil
	}
	return bufio.NewReader(f)
}
