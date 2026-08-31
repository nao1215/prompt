//go:build !windows

package prompt

import (
	"context"
	"errors"
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
	if scenario := os.Getenv("PROMPT_PTY_SCENARIO"); scenario != "" {
		helperScenario(scenario)
		return
	}
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

// ptyStep is one exchange with the program under the terminal: wait for what it
// draws, then type. Waiting first is what keeps a line out of a session that is
// not reading yet.
type ptyStep struct {
	await string
	send  string
	// resizeTo, when set, is the width the terminal is changed to once the
	// program has drawn what this step waits for and before anything is typed --
	// that is, while a read is waiting.
	resizeTo uint16
}

// runHelperUnderPTY re-executes the test binary as the prompt program, types
// two lines into it, and returns everything the terminal showed.
func runHelperUnderPTY(t *testing.T, watch, child bool) string {
	t.Helper()

	return runPromptUnderPTY(t,
		[]string{"PROMPT_PTY_WATCH=" + boolEnv(watch), "PROMPT_PTY_CHILD=" + boolEnv(child)},
		[]ptyStep{{await: "p1> ", send: "first\r"}, {await: "p2> ", send: "second\r"}},
	)
}

// runPromptUnderPTY re-executes the test binary as the prompt program with env
// added to its environment, walks it through steps, and returns everything the
// terminal showed.
func runPromptUnderPTY(t *testing.T, env []string, steps []ptyStep) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), os.Args[0]) //nolint:gosec // the test binary re-executing itself
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", helperEnv+"=1")
	cmd.Env = append(cmd.Env, env...)

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

	// A prompt is counted rather than looked for, because a scenario can draw
	// the same prefix twice and the second one has to be waited for again.
	seen := map[string]int{}
	for _, step := range steps {
		// The helper is a freshly started process on a runner that may be busy,
		// so this waits far longer than the prompt takes to appear on an idle
		// machine. It is a bound on a hang, not a measurement.
		if !waitUpTo(15*time.Second, func() bool {
			return strings.Count(transcript(), step.await) > seen[step.await]
		}) {
			kill(t, cmd)
			t.Fatalf("the program never drew %q\n--- transcript ---\n%s", step.await, transcript())
		}
		seen[step.await] = strings.Count(transcript(), step.await)
		if step.resizeTo > 0 {
			if err := pty.Setsize(ptmx, &pty.Winsize{Rows: 30, Cols: step.resizeTo}); err != nil {
				t.Logf("could not resize the pty (continuing): %v", err)
			}
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

	// The program has exited, so reading the terminal ends and the drain
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

// sttySettings reads the terminal's settings the way a shell would, because
// what a session must give back is every setting rather than the few this
// package sets. Asking stty keeps the test off the termios constants, which
// differ between Linux and the BSDs.
func sttySettings() string {
	cmd := exec.CommandContext(context.Background(), "stty", "-g")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return fmt.Sprintf("stty: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func boolEnv(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func helperScenario(name string) {
	open := func(n int) *Prompt {
		p, err := New(fmt.Sprintf("p%d> ", n), WithPersistentRawMode())
		if err != nil {
			fmt.Printf("new%d: %v\r\n", n, err)
			os.Exit(1)
		}
		return p
	}
	runChild := func(script string) {
		cmd := exec.CommandContext(context.Background(), "sh", "-c", script) //nolint:gosec // a fixed script from the table above
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("child: %v\r\n", err)
		}
	}

	switch name {
	case "reopen3":
		for i := 1; i <= 3; i++ {
			p := open(i)
			line, err := p.Run()
			fmt.Printf("session%d=%q err=%v\r\n", i, line, err)
			_, stop := p.WatchInterrupt(context.Background())
			stop()
			if err := p.Close(); err != nil {
				fmt.Printf("close%d: %v\r\n", i, err)
				os.Exit(1)
			}
			runChild("true")
		}
	case "watchtwice":
		p := open(1)
		line, err := p.Run()
		fmt.Printf("session1=%q err=%v\r\n", line, err)
		for range 3 {
			_, stop := p.WatchInterrupt(context.Background())
			stop()
			stop() // a CancelFunc may be called more than once
		}
		line, err = p.Run()
		fmt.Printf("session2=%q err=%v\r\n", line, err)
		if err := p.Close(); err != nil {
			fmt.Printf("close: %v\r\n", err)
			os.Exit(1)
		}
	case "watchnostop":
		p := open(1)
		line, err := p.Run()
		fmt.Printf("session1=%q err=%v\r\n", line, err)
		// The stop function is deliberately dropped: a watch left running is
		// what Close has to cope with.
		_, _ = p.WatchInterrupt(context.Background())
		if err := p.Close(); err != nil {
			fmt.Printf("close1: %v\r\n", err)
			os.Exit(1)
		}
		p2 := open(2)
		line, err = p2.Run()
		fmt.Printf("session2=%q err=%v\r\n", line, err)
		if err := p2.Close(); err != nil {
			fmt.Printf("close2: %v\r\n", err)
			os.Exit(1)
		}
	case "doubleclose":
		p := open(1)
		line, err := p.Run()
		fmt.Printf("session1=%q err=%v\r\n", line, err)
		for range 3 {
			if err := p.Close(); err != nil {
				fmt.Printf("close: %v\r\n", err)
				os.Exit(1)
			}
		}
		p2 := open(2)
		line, err = p2.Run()
		fmt.Printf("session2=%q err=%v\r\n", line, err)
		_ = p2.Close()
	case "childstty":
		p := open(1)
		line, err := p.Run()
		fmt.Printf("session1=%q err=%v\r\n", line, err)
		if err := p.Close(); err != nil {
			fmt.Printf("close1: %v\r\n", err)
			os.Exit(1)
		}
		runChild("stty sane")
		p2 := open(2)
		line, err = p2.Run()
		fmt.Printf("session2=%q err=%v\r\n", line, err)
		_ = p2.Close()
	case "nestedwatch":
		p := open(1)
		line, err := p.Run()
		fmt.Printf("session1=%q err=%v\r\n", line, err)
		_, stopA := p.WatchInterrupt(context.Background())
		_, stopB := p.WatchInterrupt(context.Background())
		stopB()
		stopA()
		line, err = p.Run()
		fmt.Printf("session2=%q err=%v\r\n", line, err)
		_ = p.Close()
	case "childraw":
		// A child that leaves the terminal in raw mode and dies there.
		p := open(1)
		line, err := p.Run()
		fmt.Printf("session1=%q err=%v\r\n", line, err)
		if err := p.Close(); err != nil {
			fmt.Printf("close1: %v\r\n", err)
		}
		runChild("stty raw -echo")
		p2 := open(2)
		line, err = p2.Run()
		fmt.Printf("session2=%q err=%v\r\n", line, err)
		_ = p2.Close()
	case "winch":
		p := open(1)
		line, err := p.Run()
		fmt.Printf("session1=%q err=%v\r\n", line, err)
		line, err = p.Run()
		fmt.Printf("session2=%q err=%v\r\n", line, err)
		_ = p.Close()
	case "cancelduringrun":
		// Cancelling from another goroutine is the one concurrent use the
		// package supports, so a read waiting for a key has to end on it.
		p := open(1)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			line, err := p.RunWithContext(ctx)
			fmt.Printf("session1=%q err=%v\r\n", line, err)
			close(done)
		}()
		time.Sleep(500 * time.Millisecond)
		cancel()
		select {
		case <-done:
			fmt.Printf("run returned\r\n")
		case <-time.After(5 * time.Second):
			fmt.Printf("run never returned\r\n")
			os.Exit(1)
		}
		_ = p.Close()
	case "typeahead":
		p := open(1)
		line, err := p.Run()
		fmt.Printf("session1=%q err=%v\r\n", line, err)
		ctx, stop := p.WatchInterrupt(context.Background())
		fmt.Printf("watching\r\n")
		<-ctx.Done()
		stop()
		fmt.Printf("interrupted\r\n")
		line, err = p.Run()
		fmt.Printf("session2=%q err=%v\r\n", line, err)
		_ = p.Close()
	case "watchdefault":
		// The same watch as "typeahead", in the mode a caller gets without
		// asking for anything: Run gives raw mode back when it returns, so the
		// terminal is cooked while the watch runs and Ctrl+C arrives as SIGINT
		// rather than as the byte the watcher reads.
		p, err := New("p1> ")
		if err != nil {
			fmt.Printf("new: %v\r\n", err)
			os.Exit(1)
		}
		line, err := p.Run()
		fmt.Printf("session1=%q err=%v\r\n", line, err)
		ctx, stop := p.WatchInterrupt(context.Background())
		fmt.Printf("watching\r\n")
		select {
		case <-ctx.Done():
			fmt.Printf("interrupted\r\n")
		case <-time.After(10 * time.Second):
			fmt.Printf("never-interrupted\r\n")
		}
		stop()
		_ = p.Close()
	case "runafterclose":
		start := sttySettings()
		p := open(1)
		line, err := p.Run()
		fmt.Printf("session1=%q err=%v\r\n", line, err)
		if err := p.Close(); err != nil {
			fmt.Printf("close: %v\r\n", err)
		}
		closed := sttySettings()
		// A REPL that closed on one path and came round again. The second Run has
		// to report that the session is over, and it must do so without touching
		// the terminal, because in a persistent session the Close that would
		// restore it has already run. The prefix changes so that anything that
		// Run draws is recognizable in the transcript: nothing may be.
		p.SetPrefix("p2> ")
		line, err = p.Run()
		fmt.Printf("session2=%q err=%v errEOF=%v\r\n", line, err, errors.Is(err, ErrEOF))
		fmt.Printf("restored=%v\r\n", sttySettings() == closed)
		// Diagnostic, not an assertion: whether a whole session gives the
		// terminal back exactly as it found it is a separate question from what
		// a Run on a closed prompt does, and the answer differs by platform.
		fmt.Printf("sessionrestored=%v\r\nstty0=%s\r\nstty1=%s\r\n", start == closed, start, closed)
	case "childinput":
		p := open(1)
		line, err := p.Run()
		fmt.Printf("session1=%q err=%v\r\n", line, err)
		if err := p.Close(); err != nil {
			fmt.Printf("close1: %v\r\n", err)
		}
		fmt.Printf("childstart\r\n")
		runChild("sleep 0.5")
		p2 := open(2)
		line, err = p2.Run()
		fmt.Printf("session2=%q err=%v\r\n", line, err)
		_ = p2.Close()
	default:
		fmt.Printf("unknown scenario %q\r\n", name)
		os.Exit(1)
	}
}

// TestPromptLifecycleOrdersUnderAPTY walks the orders a REPL can put a prompt
// through and asserts that every session still reads its line. #47 was found by
// one of these orders; the rest are the ones no application had tried, and a
// terminal is the only place they mean anything -- a mock returns EOF the moment
// its input runs out, so nothing that happens while a read is waiting can
// happen there at all.
func TestPromptLifecycleOrdersUnderAPTY(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		scenario string
		steps    []ptyStep
		want     []string
		// absent is what the terminal must never have been shown, for a scenario
		// whose point is that something was not drawn.
		absent []string
	}{
		{
			name:     "three sessions in a row, each handing the terminal to a child",
			scenario: "reopen3",
			steps:    []ptyStep{{await: "p1> ", send: "one\r"}, {await: "p2> ", send: "two\r"}, {await: "p3> ", send: "three\r"}},
			want:     []string{`session1="one"`, `session2="two"`, `session3="three"`},
		},
		{
			name:     "a watch started and stopped repeatedly, and stopped twice each time",
			scenario: "watchtwice",
			steps:    []ptyStep{{await: "p1> ", send: "one\r"}, {await: "p1> ", send: "two\r"}},
			want:     []string{`session1="one"`, `session2="two"`},
		},
		{
			name:     "a watch that is never stopped before the prompt is closed",
			scenario: "watchnostop",
			steps:    []ptyStep{{await: "p1> ", send: "one\r"}, {await: "p2> ", send: "two\r"}},
			want:     []string{`session1="one"`, `session2="two"`},
		},
		{
			name:     "two watches at once, stopped in reverse order",
			scenario: "nestedwatch",
			steps:    []ptyStep{{await: "p1> ", send: "one\r"}, {await: "p1> ", send: "two\r"}},
			want:     []string{`session1="one"`, `session2="two"`},
		},
		{
			name:     "closing more than once",
			scenario: "doubleclose",
			steps:    []ptyStep{{await: "p1> ", send: "one\r"}, {await: "p2> ", send: "two\r"}},
			want:     []string{`session1="one"`, `session2="two"`},
		},
		{
			name:     "a child that resets the terminal before the next session",
			scenario: "childstty",
			steps:    []ptyStep{{await: "p1> ", send: "one\r"}, {await: "p2> ", send: "two\r"}},
			want:     []string{`session1="one"`, `session2="two"`},
		},
		{
			name:     "typing while a child holds the terminal reaches the next session",
			scenario: "childinput",
			steps:    []ptyStep{{await: "p1> ", send: "one\r"}, {await: "childstart", send: "typed\r"}},
			want:     []string{`session1="one"`, `session2="typed"`},
		},
		{
			name:     "typing ahead of an interrupt is delivered to the next session",
			scenario: "typeahead",
			steps:    []ptyStep{{await: "p1> ", send: "one\r"}, {await: "watching", send: "ahead\x03"}, {await: "interrupted", send: "\r"}},
			want:     []string{`session1="one"`, `session2="ahead"`},
		},
		{
			name:     "a watch in the default mode sees the interrupt",
			scenario: "watchdefault",
			steps:    []ptyStep{{await: "p1> ", send: "one\r"}, {await: "watching", send: "\x03"}},
			want:     []string{`session1="one"`, "interrupted"},
		},
		{
			name:     "a Run on a closed prompt leaves the terminal as it found it",
			scenario: "runafterclose",
			steps:    []ptyStep{{await: "p1> ", send: "one\r"}},
			want:     []string{`session1="one"`, "errEOF=true", "restored=true", "sessionrestored=true"},
			absent:   []string{"p2> "},
		},
		{
			name:     "cancelling the context from another goroutine ends the read",
			scenario: "cancelduringrun",
			want:     []string{"run returned"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := runPromptUnderPTY(t, []string{"PROMPT_PTY_SCENARIO=" + tt.scenario}, tt.steps)
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("the transcript does not contain %s\n--- transcript ---\n%s", want, out)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(out, absent) {
					t.Errorf("the transcript contains %s, which nothing should have drawn\n--- transcript ---\n%s", absent, out)
				}
			}
		})
	}
}

// TestPromptSurvivesTheTerminalChangingUnderIt covers two things a session can
// do to the terminal between one prompt and the next, both of which surfaces
// this package has been bitten by leave in a state the next read has to cope
// with.
func TestPromptSurvivesTheTerminalChangingUnderIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		scenario string
		steps    []ptyStep
		want     []string
	}{
		{
			name:     "a child that leaves the terminal in raw mode",
			scenario: "childraw",
			steps:    []ptyStep{{await: "p1> ", send: "one\r"}, {await: "p2> ", send: "two\r"}},
			want:     []string{`session1="one"`, `session2="two"`},
		},
		{
			name:     "the terminal resized while a read is waiting",
			scenario: "winch",
			steps: []ptyStep{
				{await: "p1> ", send: "one\r", resizeTo: 20},
				{await: "p1> ", send: "two\r", resizeTo: 100},
			},
			want: []string{`session1="one"`, `session2="two"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := runPromptUnderPTY(t, []string{"PROMPT_PTY_SCENARIO=" + tt.scenario}, tt.steps)
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("the transcript does not contain %s\n--- transcript ---\n%s", want, out)
				}
			}
		})
	}
}
