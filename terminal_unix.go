//go:build !windows

package prompt

import (
	"os"

	"golang.org/x/term"
)

// rawEnter puts the terminal into raw mode on Unix by setting raw mode on
// os.Stdin. Under a pseudo-terminal os.Stdin is the pty slave, and go-tty reads
// through /dev/tty, which resolves to that same controlling terminal, so raw
// mode set here governs the read path. This is the path that shipped on Unix
// before the Windows fix and is kept unchanged: go-tty's own Raw sets raw mode on
// a separately opened /dev/tty descriptor, which regressed interactive sessions
// on macOS, so only Windows (where the read handle genuinely differs from
// os.Stdin) routes raw mode through go-tty.
//
// When os.Stdin is not a terminal there is nothing to make raw, so it returns a
// nil restore func and lets SetRaw stay a no-op.
func (t *realTerminal) rawEnter() (func() error, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, nil
	}
	state, err := term.GetState(fd)
	if err != nil {
		return nil, err
	}
	if _, err := term.MakeRaw(fd); err != nil {
		return nil, err
	}
	return func() error { return term.Restore(fd, state) }, nil
}
