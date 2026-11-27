//go:build !windows

package prompt

import (
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

// ttyReaderOn returns a reader of a duplicate of fd, so closing the reader
// leaves fd to its owner.
func ttyReaderOn(t *testing.T, fd uintptr) *ttyReader {
	t.Helper()
	dup, err := unix.Dup(int(fd)) // #nosec G115 -- a descriptor fits an int
	if err != nil {
		t.Fatal(err)
	}
	r := newTTYReader(dup)
	if r == nil {
		t.Fatal("newTTYReader returned nil for an open descriptor")
	}
	return r
}

// TestTTYReaderReadsWhatTheTerminalSends checks a read that has to wait for
// the key: it waits in poll, then returns what the other end wrote.
func TestTTYReaderReadsWhatTheTerminalSends(t *testing.T) {
	t.Parallel()

	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()
	if err := unix.SetNonblock(int(tty.Fd()), false); err != nil { // #nosec G115
		t.Fatal(err)
	}
	r := ttyReaderOn(t, tty.Fd())
	defer r.Close()

	go func() {
		time.Sleep(20 * time.Millisecond)
		// The line discipline is canonical here, so a line is what arrives.
		if _, err := ptmx.WriteString("k\n"); err != nil {
			t.Error(err)
		}
	}()
	buf := make([]byte, 8)
	n, err := r.Read(buf)
	if err != nil || string(buf[:n]) != "k\n" {
		t.Fatalf("Read = %q, %v; want \"k\\n\", nil", buf[:n], err)
	}
}

// TestTTYReaderEndings checks how a read ends without a key: the other end
// closing, which is the end of input; an error from read(2); and Close, which
// ends a read waiting for a key and every read after it, and can be called
// again.
func TestTTYReaderEndings(t *testing.T) {
	t.Parallel()

	t.Run("pipe writer closed", func(t *testing.T) {
		t.Parallel()
		var p [2]int
		if err := unix.Pipe(p[:]); err != nil {
			t.Fatal(err)
		}
		_ = unix.Close(p[1])
		r := newTTYReader(p[0])
		defer r.Close()
		if _, err := r.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
			t.Fatalf("Read = %v, want io.EOF", err)
		}
	})
	t.Run("read error", func(t *testing.T) {
		t.Parallel()
		// read(2) of a directory fails with EISDIR, which stands for any
		// error the terminal reports.
		fd, err := unix.Open(t.TempDir(), unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		r := newTTYReader(fd)
		defer r.Close()
		if _, err := r.Read(make([]byte, 1)); !errors.Is(err, unix.EISDIR) {
			t.Fatalf("Read = %v, want EISDIR", err)
		}
	})
	t.Run("close", func(t *testing.T) {
		t.Parallel()
		var p [2]int
		if err := unix.Pipe(p[:]); err != nil {
			t.Fatal(err)
		}
		defer unix.Close(p[1])
		r := newTTYReader(p[0])
		done := make(chan error, 1)
		go func() {
			_, err := r.Read(make([]byte, 1))
			done <- err
		}()
		time.Sleep(20 * time.Millisecond)
		if err := r.Close(); err != nil {
			t.Fatalf("Close = %v", err)
		}
		if err := <-done; !errors.Is(err, errTerminalClosed) {
			t.Fatalf("waiting Read = %v, want errTerminalClosed", err)
		}
		if _, err := r.Read(make([]byte, 1)); !errors.Is(err, errTerminalClosed) {
			t.Fatalf("Read after Close = %v, want errTerminalClosed", err)
		}
		if err := r.Close(); err != nil {
			t.Fatalf("second Close = %v, want nil", err)
		}
	})
}

// TestTTYReaderOfABadDescriptor checks that a descriptor that cannot be made
// non-blocking gives no reader, which leaves reading to go-tty.
func TestTTYReaderOfABadDescriptor(t *testing.T) {
	t.Parallel()

	if r := newTTYReader(-1); r != nil {
		t.Fatal("newTTYReader(-1) returned a reader")
	}
}

// TestOpenReadHandleFollowsTheTerminal checks that the read handle is the
// controlling terminal when there is one and nil, leaving reading to go-tty,
// when there is none, as on a CI runner.
func TestOpenReadHandleFollowsTheTerminal(t *testing.T) {
	t.Parallel()

	_, statErr := os.Stat("/dev/tty")
	fd, openErr := unix.Open("/dev/tty", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if openErr == nil {
		_ = unix.Close(fd)
	}
	handle := openReadHandle()
	switch {
	case openErr != nil && handle != nil:
		t.Fatalf("openReadHandle = %v with no terminal to open (%v, stat %v); want nil", handle, openErr, statErr)
	case openErr == nil && handle == nil:
		t.Fatal("openReadHandle = nil although /dev/tty opens")
	case handle != nil:
		if err := handle.Close(); err != nil {
			t.Fatalf("Close = %v", err)
		}
	}
}
