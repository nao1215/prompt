//go:build windows

package prompt

import (
	"bufio"
	"os"
)

// openReadHandle returns nil on Windows, leaving reading to go-tty.
//
// go-tty reads Windows input from a CONIN$ handle it opens itself, which under
// a ConPTY is a different handle than os.Stdin, and raw mode is routed through
// go-tty for the same reason (see rawEnter). Opening a second handle here would
// split the read path from the one raw mode governs, which is the bug that
// prompt v0.0.11 fixed.
//
// The consequence is that Close cannot end a read in progress on Windows, so it
// does not wait for one: see Prompt.Close.
func openReadHandle() *os.File { return nil }

// newReadBuffer is nil on Windows, where there is no handle of our own to read.
func newReadBuffer(*os.File) *bufio.Reader { return nil }
