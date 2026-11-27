//go:build linux

package prompt

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestTTYReaderWaitsAgainAfterASignal interrupts a read waiting in poll(2)
// with a signal, which makes poll return EINTR, and checks that the read goes
// back to waiting: it ends only when Close wakes it, with errTerminalClosed.
// SIGURG is the signal the Go runtime sends itself to preempt goroutines, so
// delivering one more to the thread does nothing else.
func TestTTYReaderWaitsAgainAfterASignal(t *testing.T) {
	t.Parallel()

	var p [2]int
	if err := unix.Pipe(p[:]); err != nil {
		t.Fatal(err)
	}
	defer unix.Close(p[1])
	r := newTTYReader(p[0])

	tid := make(chan int, 1)
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		tid <- unix.Gettid()
		_, err := r.Read(make([]byte, 1))
		done <- err
	}()
	thread := <-tid
	for range 3 {
		time.Sleep(20 * time.Millisecond)
		if err := unix.Tgkill(unix.Getpid(), thread, unix.SIGURG); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-done:
		t.Fatalf("Read returned %v after a signal, want it to keep waiting", err)
	case <-time.After(20 * time.Millisecond):
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, errTerminalClosed) {
		t.Fatalf("Read = %v, want errTerminalClosed", err)
	}
}
