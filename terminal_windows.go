//go:build windows

package prompt

// rawEnter puts the terminal into raw mode on Windows through go-tty's own Raw.
// go-tty reads input from a CONIN$ handle it opens itself, which under a ConPTY
// is a different handle than os.Stdin. Setting raw mode on os.Stdin there left
// the read handle in cooked mode, so input delivered right after a re-rendered
// prompt could be line-buffered instead of read immediately, leaving a command
// typed but never executed and the session hung. Routing raw mode through go-tty
// applies it to the same handle reads come from.
//
// When there is no tty there is nothing to make raw, so it returns a nil restore
// func and lets SetRaw stay a no-op.
func (t *realTerminal) rawEnter() (func() error, error) {
	if t.tty == nil {
		return nil, nil
	}
	return t.tty.Raw()
}
