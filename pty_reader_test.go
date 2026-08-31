//go:build !windows

package prompt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

// The scenario that produced this test, from github.com/nao1215/prompt/issues/47.
//
// A REPL reads a line, runs it while WatchInterrupt watches for Ctrl-C, and at
// some point closes the prompt to hand the terminal to a child process — an
// editor, a pager — before opening a prompt again. The second prompt received
// nothing at all: it drew its prefix and swallowed every keystroke, Ctrl-D
// included, so the session could not even be ended.
//
// The reader goroutine WatchInterrupt starts was blocked in a terminal read
// that closing the terminal did not end, because go-tty calls Fd() on the file
// it reads and that takes it out of the runtime's poller. Closing left the
// goroutine in read(2) on a descriptor number the kernel had taken back, and
// running a child process is enough to get that number reused — by the second
// prompt's own /dev/tty. The abandoned reader was then holding the new prompt's
// terminal.
//
// Both conditions are needed, which is why the matrix below runs all four: with
// either the watch or the child left out, the bug hides.

// helperEnv makes the test binary run as the prompt program below instead of as
// a test. Re-executing the test binary is what lets the scenario run under a
// pty without a second module to build.
const helperEnv = "PROMPT_PTY_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) != "" {
		helperMain()
		return
	}
	os.Exit(m.Run())
}

// helperMain is a two-session REPL: read a line, optionally watch for Ctrl-C
// while "running" it, close, optionally run a child on the same terminal, then
// open a second prompt and read another line.
func helperMain() {
	watch := os.Getenv("PROMPT_PTY_WATCH") == "1"
	child := os.Getenv("PROMPT_PTY_CHILD") == "1"

	open := func(n int) *Prompt {
		p, err := New(fmt.Sprintf("p%d> ", n), WithPersistentRawMode())
		if err != nil {
			fmt.Printf("new: %v\r\n", err)
			os.Exit(1)
		}
		return p
	}

	p := open(1)
	line, err := p.Run()
	fmt.Printf("session1=%q err=%v\r\n", line, err)
	if watch {
		_, stop := p.WatchInterrupt(context.Background())
		stop()
	}
	if err := p.Close(); err != nil {
		fmt.Printf("close1: %v\r\n", err)
		os.Exit(1)
	}

	if child {
		cmd := exec.CommandContext(context.Background(), "sh", "-c", "true")
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("child: %v\r\n", err)
		}
	}

	p2 := open(2)
	line, err = p2.Run()
	fmt.Printf("session2=%q err=%v\r\n", line, err)
	if err := p2.Close(); err != nil {
		fmt.Printf("close2: %v\r\n", err)
		os.Exit(1)
	}
}

// TestPromptReopensAfterCloseUnderAPTY drives the scenario against a real
// terminal, in every combination of the two conditions it needs.
func TestPromptReopensAfterCloseUnderAPTY(t *testing.T) {
	t.Parallel()

	for _, watch := range []bool{false, true} {
		for _, child := range []bool{false, true} {
			name := fmt.Sprintf("watch=%v child=%v", watch, child)
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				out := runHelperUnderPTY(t, watch, child)
				for _, want := range []string{`session1="first"`, `session2="second"`} {
					if !strings.Contains(out, want) {
						t.Errorf("%s: transcript does not contain %s\n--- transcript ---\n%s", name, want, out)
					}
				}
			})
		}
	}
}

// runHelperUnderPTY re-executes the test binary as the prompt program, types
// two lines into it, and returns everything the terminal showed.
func runHelperUnderPTY(t *testing.T, watch, child bool) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), os.Args[0]) //nolint:gosec // the test binary re-executing itself
	cmd.Env = append(os.Environ(),
		helperEnv+"=1",
		"PROMPT_PTY_WATCH="+boolEnv(watch),
		"PROMPT_PTY_CHILD="+boolEnv(child),
		"TERM=xterm-256color",
	)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Skipf("cannot allocate a pty here: %v", err)
	}
	defer func() { _ = ptmx.Close() }()
	if err := pty.Setsize(ptmx, &pty.Winsize{Rows: 30, Cols: 120}); err != nil {
		t.Logf("could not set the pty size (continuing): %v", err)
	}

	var (
		mu  sync.Mutex
		buf strings.Builder
	)
	// drained is closed once the terminal has no more to give, which is after
	// the helper has exited and everything it wrote has been copied. Reading the
	// transcript before then races the last line the helper printed against the
	// copy of it.
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		chunk := make([]byte, 4096)
		for {
			n, err := ptmx.Read(chunk)
			if n > 0 {
				mu.Lock()
				buf.Write(chunk[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	transcript := func() string {
		mu.Lock()
		defer mu.Unlock()
		return buf.String()
	}

	// Each line is written once the prompt that reads it is on screen, so
	// neither is typed into a session that is not reading yet.
	for _, step := range []struct{ await, send string }{
		{await: "p1> ", send: "first\r"},
		{await: "p2> ", send: "second\r"},
	} {
		// The helper is a freshly started process on a runner that may be busy,
		// so this waits far longer than the prompt takes to appear on an idle
		// machine. It is a bound on a hang, not a measurement.
		if !waitUpTo(15*time.Second, func() bool { return strings.Contains(transcript(), step.await) }) {
			kill(t, cmd)
			t.Fatalf("the %q prompt never appeared\n--- transcript ---\n%s", step.await, transcript())
		}
		if _, err := ptmx.WriteString(step.send); err != nil {
			t.Fatalf("writing %q to the terminal: %v", step.send, err)
		}
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("the program exited with %v\n--- transcript ---\n%s", err, transcript())
		}
	case <-time.After(30 * time.Second):
		kill(t, cmd)
		t.Fatalf("the program never finished; a session read nothing\n--- transcript ---\n%s", transcript())
	}

	// The helper has exited, so reading the terminal ends and the drain
	// finishes with everything it wrote.
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Log("the terminal did not reach end of input; the transcript may be short")
	}
	return transcript()
}

// kill ends the helper when a wait gives up, so a hung session does not outlive
// the test that was watching it.
func kill(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if err := cmd.Process.Kill(); err != nil {
		t.Logf("killing the helper: %v", err)
	}
}

func boolEnv(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
