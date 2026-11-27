package prompt

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// faultTerminal is a mock terminal whose calls can fail: once its script is
// read, ReadRune returns readErr instead of io.EOF, and each other call
// returns its own error when one is set.
type faultTerminal struct {
	*mockTerminal
	readErr    error
	setRawErr  error
	restoreErr error
	sizeErr    error
	closeErr   error
}

func (f *faultTerminal) ReadRune() (rune, int, error) {
	r, n, err := f.mockTerminal.ReadRune()
	if errors.Is(err, io.EOF) && f.readErr != nil {
		return 0, 0, f.readErr
	}
	return r, n, err
}

func (f *faultTerminal) SetRaw() error {
	if f.setRawErr != nil {
		return f.setRawErr
	}
	return f.mockTerminal.SetRaw()
}

func (f *faultTerminal) Restore() error {
	if f.restoreErr != nil {
		return f.restoreErr
	}
	return f.mockTerminal.Restore()
}

func (f *faultTerminal) Size() (int, int, error) {
	if f.sizeErr != nil {
		return 0, 0, f.sizeErr
	}
	return f.mockTerminal.Size()
}

func (f *faultTerminal) Close() error {
	return f.closeErr
}

var errFault = errors.New("terminal failed")

// TestReadFailureEndsTheSession covers a read that fails for a reason other
// than the end of input, at each point a key can be half read: between keys,
// after ESC, after ESC O, inside a CSI sequence, and inside reverse search.
// The failure is the caller's to see; reading on would spin on a dead
// terminal, and calling it the end of input would hide that it broke.
func TestReadFailureEndsTheSession(t *testing.T) {
	t.Parallel()

	for _, script := range []string{"", "ab", "a\x1b", "a\x1bO", "a\x1b[1;", "\x12se\x1b"} {
		t.Run(strings.NewReplacer("\x1b", "ESC", "\x12", "^R").Replace(script), func(t *testing.T) {
			t.Parallel()

			p := newTestPromptOn(&faultTerminal{mockTerminal: newMockTerminal(script), readErr: errFault})
			p.history = []string{"select 1"}
			got, err := p.Run(context.Background())
			if !errors.Is(err, errFault) || errors.Is(err, ErrEOF) {
				t.Fatalf("Run = %q, %v; want the read's error", got, err)
			}
		})
	}
}

// TestRawModeFailureLeavesThePromptUsable checks that a terminal refusing raw
// mode fails the Run with that error and leaves nothing marked as raw, so the
// next Run on a terminal that accepts it enters raw mode again.
func TestRawModeFailureLeavesThePromptUsable(t *testing.T) {
	t.Parallel()

	ft := &faultTerminal{mockTerminal: newMockTerminal("ok\r"), setRawErr: errFault}
	p := newTestPromptOn(ft)
	if _, err := p.Run(context.Background()); !errors.Is(err, errFault) {
		t.Fatalf("Run = %v, want the raw mode error", err)
	}
	ft.setRawErr = nil
	got, err := p.Run(context.Background())
	if err != nil || got != "ok" {
		t.Fatalf("second Run = %q, %v; want \"ok\", nil", got, err)
	}
	if ft.setRawCount != 1 {
		t.Errorf("raw mode entered %d times, want 1", ft.setRawCount)
	}
}

// TestRestoreFailureKeepsTheResult checks the endings whose only failure is
// giving the terminal back: the entry, the end of input, and Close in a
// persistent session. What the user typed, or the end of it, is still what
// they get, and Close still releases the terminal.
func TestRestoreFailureKeepsTheResult(t *testing.T) {
	t.Parallel()

	t.Run("entry", func(t *testing.T) {
		t.Parallel()
		p := newTestPromptOn(&faultTerminal{mockTerminal: newMockTerminal("ls\r"), restoreErr: errFault})
		if got, err := p.Run(context.Background()); err != nil || got != "ls" {
			t.Fatalf("Run = %q, %v; want \"ls\", nil", got, err)
		}
	})
	t.Run("end of input", func(t *testing.T) {
		t.Parallel()
		p := newTestPromptOn(&faultTerminal{mockTerminal: newMockTerminal("ls"), restoreErr: errFault}, WithPersistentRawMode())
		if _, err := p.Run(context.Background()); !errors.Is(err, ErrEOF) {
			t.Fatalf("Run = %v, want ErrEOF", err)
		}
	})
	t.Run("close", func(t *testing.T) {
		t.Parallel()
		ft := &faultTerminal{mockTerminal: newMockTerminal("ls\r"), restoreErr: errFault, closeErr: errFault}
		p := newTestPromptOn(ft, WithPersistentRawMode())
		if got, err := p.Run(context.Background()); err != nil || got != "ls" {
			t.Fatalf("Run = %q, %v; want \"ls\", nil", got, err)
		}
		if err := p.Close(); !errors.Is(err, errFault) {
			t.Fatalf("Close = %v, want the terminal's close error", err)
		}
	})
}

// TestZeroPromptCloses checks that Close on a Prompt that was never opened,
// which has no terminal to release, succeeds.
func TestZeroPromptCloses(t *testing.T) {
	t.Parallel()

	var p Prompt
	if err := p.Close(); err != nil {
		t.Fatalf("Close = %v, want nil", err)
	}
}

// TestSizeFailureFallsBack checks that a terminal that cannot report its size
// is drawn at the fallback size, by the editor and by reverse search alike,
// and the session goes on.
func TestSizeFailureFallsBack(t *testing.T) {
	t.Parallel()

	p := newTestPromptOn(&faultTerminal{mockTerminal: newMockTerminal("\x12sel\r\r"), sizeErr: errFault})
	p.history = []string{"select 1"}
	if got, err := p.Run(context.Background()); err != nil || got != "select 1" {
		t.Fatalf("Run = %q, %v; want \"select 1\", nil", got, err)
	}
	if w, h := p.searchTerminalSize(); w != defaultTerminalWidth || h != fallbackHeight {
		t.Errorf("searchTerminalSize = %d, %d; want the fallback %d, %d", w, h, defaultTerminalWidth, fallbackHeight)
	}
	p.terminal = nil
	if w, h := p.searchTerminalSize(); w != defaultTerminalWidth || h != fallbackHeight {
		t.Errorf("searchTerminalSize without a terminal = %d, %d; want the fallback", w, h)
	}
}

// TestOpenFailureClosesTheTerminal checks that a history file that cannot be
// loaded fails the open, and that the terminal opened for it is closed, with
// both errors reported when closing fails too.
func TestOpenFailureClosesTheTerminal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir() // a directory where the history file should be
	config := options{historyConfig: &historyConfig{Enabled: true, File: dir, MaxEntries: 10}}
	_, err := newFromConfigOn(config, &faultTerminal{mockTerminal: newMockTerminal(""), closeErr: errFault}, io.Discard)
	if err == nil || !errors.Is(err, errFault) || !strings.Contains(err.Error(), "failed to load history") {
		t.Fatalf("newFromConfigOn = %v; want the load error joined with the close error", err)
	}
}

// TestCloseReportsNothingWhenHistoryCannotBeSaved checks that Close goes on to
// release the terminal when the history cannot be written, since losing the
// history must not also leave the terminal held.
func TestCloseReportsNothingWhenHistoryCannotBeSaved(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	p := newTestPrompt(newMockTerminal("ls\r"))
	p.historyManager = newHistoryManager(&historyConfig{Enabled: true, File: filepath.Join(blocker, "history"), MaxEntries: 10})
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close = %v, want nil", err)
	}
}

// TestHistoryWithoutAManager covers a prompt holding its history itself, the
// way one built without a history manager does: History returns a copy,
// SetHistory keeps the newest entries up to the limit, and a repeat of the
// last entry is not added again.
func TestHistoryWithoutAManager(t *testing.T) {
	t.Parallel()

	p := newTestPrompt(newMockTerminal(""))
	p.SetHistory(make([]string, defaultMaxHistoryEntries+5))
	if n := len(p.History()); n != defaultMaxHistoryEntries {
		t.Fatalf("SetHistory kept %d entries, want %d", n, defaultMaxHistoryEntries)
	}
	p.SetHistory([]string{"a"})
	p.addToHistory("a")
	got := p.History()
	if len(got) != 1 {
		t.Fatalf("History = %q, want [a]", got)
	}
	got[0] = "changed"
	if p.History()[0] != "a" {
		t.Error("History returned the prompt's own slice")
	}
}

// TestNonPositiveHistoryLimitsMeanTheDefault checks that a limit of zero or
// less, which would keep no history at all, is read as the default.
func TestNonPositiveHistoryLimitsMeanTheDefault(t *testing.T) {
	t.Parallel()

	for name, option := range map[string]Option{
		"memory": WithMemoryHistory(0),
		"file":   WithFileHistory(filepath.Join(t.TempDir(), "h"), -1),
	} {
		var config options
		option(&config)
		if config.historyConfig.MaxEntries != defaultMaxHistoryEntries {
			t.Errorf("%s: MaxEntries = %d, want %d", name, config.historyConfig.MaxEntries, defaultMaxHistoryEntries)
		}
	}
}

// TestHistoryPathNeedsAHome checks that a path under ~ fails to expand, rather
// than landing in the working directory, when there is no home directory.
func TestHistoryPathNeedsAHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("home", "")
	for _, path := range []string{"~", "~/history"} {
		if got, err := expandHistoryPath(path); err == nil {
			t.Errorf("expandHistoryPath(%q) = %q, want an error", path, got)
		}
	}
}

// TestHistorySaveReportsFileSystemFailures makes each step of a save that
// touches the file system fail: removing the oldest backup, shifting the
// backups, moving the file aside, and creating its directory. A save that
// fails says which step did, and leaves the history file as it was, since a
// rotation that half ran must not be followed by a write over the only copy.
func TestHistorySaveReportsFileSystemFailures(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not stop a rename here")
	}
	t.Parallel()

	// overflowing returns a manager whose next save drops entries the file
	// holds, which is what makes it rotate.
	overflowing := func(t *testing.T, dir string, backups int) (*historyManager, string) {
		t.Helper()
		file := filepath.Join(dir, "history")
		if err := os.WriteFile(file, []byte("a\nb\nc\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		hm := newHistoryManager(&historyConfig{Enabled: true, File: file, MaxEntries: 1, MaxBackups: backups})
		if err := hm.loadHistory(); err != nil {
			t.Fatal(err)
		}
		return hm, file
	}
	readOnly := func(t *testing.T, dir string) {
		t.Helper()
		// A directory needs its search bit, which is what the modes below keep.
		if err := os.Chmod(dir, 0o500); err != nil { // #nosec G302
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(dir, 0o700); err != nil { // #nosec G302
				t.Error(err)
			}
		})
	}

	tests := []struct {
		name  string
		setup func(t *testing.T) (*historyManager, string)
		want  string
	}{
		{
			name: "oldest backup cannot be removed",
			setup: func(t *testing.T) (*historyManager, string) {
				t.Helper()
				dir := t.TempDir()
				hm, file := overflowing(t, dir, 1)
				// A directory with something in it cannot be removed by os.Remove.
				if err := os.MkdirAll(filepath.Join(file+".1", "kept"), 0o700); err != nil {
					t.Fatal(err)
				}
				return hm, file
			},
			want: "failed to remove oldest backup",
		},
		{
			name: "backup cannot be shifted",
			setup: func(t *testing.T) (*historyManager, string) {
				t.Helper()
				dir := t.TempDir()
				hm, file := overflowing(t, dir, 2)
				if err := os.WriteFile(file+".1", []byte("old\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				readOnly(t, dir)
				return hm, file
			},
			want: "failed to rotate backup 1",
		},
		{
			name: "file cannot be moved aside",
			setup: func(t *testing.T) (*historyManager, string) {
				t.Helper()
				dir := t.TempDir()
				hm, file := overflowing(t, dir, 1)
				readOnly(t, dir)
				return hm, file
			},
			want: "failed to create backup",
		},
		{
			name: "directory cannot be created",
			setup: func(t *testing.T) (*historyManager, string) {
				t.Helper()
				parent := t.TempDir()
				readOnly(t, parent)
				file := filepath.Join(parent, "missing", "history")
				hm := newHistoryManager(&historyConfig{Enabled: true, File: file})
				hm.addEntry("ls")
				return hm, ""
			},
			want: "failed to create history directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hm, file := tt.setup(t)
			err := hm.saveHistory()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("saveHistory = %v, want an error saying %q", err, tt.want)
			}
			if file == "" {
				return
			}
			got, err := os.ReadFile(file) // #nosec G304 -- a file under t.TempDir
			if err != nil || string(got) != "a\nb\nc\n" {
				t.Errorf("history file after the failed save = %q, %v; want it untouched", got, err)
			}
		})
	}
}

// TestExitRawModeReportsAFailedWrite checks that leaving raw mode reports a
// failure to turn bracketed paste off, and still restores the terminal.
func TestExitRawModeReportsAFailedWrite(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("")
	p := newTestPrompt(mock)
	p.output = &failAfterWriter{}
	p.rawActive.Store(true)
	if err := p.exitRawMode(); !errors.Is(err, errWrite) {
		t.Fatalf("exitRawMode = %v, want the write's error", err)
	}
	if mock.restoreCount != 1 {
		t.Errorf("Restore called %d times, want 1", mock.restoreCount)
	}
}

// TestFirstSaveThatOverflowsMakesNoBackup checks a save that leaves entries
// out before any history file exists: there is nothing to move aside, so the
// save writes the file and makes no backup.
func TestFirstSaveThatOverflowsMakesNoBackup(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "history")
	hm := newHistoryManager(&historyConfig{Enabled: true, File: file, MaxFileSize: 8, MaxBackups: 2})
	hm.addEntry("first entry")
	hm.addEntry("second entry")
	if err := hm.saveHistory(); err != nil {
		t.Fatalf("saveHistory = %v", err)
	}
	if _, err := os.Stat(file + ".1"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("backup after the first save: %v, want none", err)
	}
	got, err := readHistoryFile(file)
	if err != nil || len(got) != 1 || got[0] != "second entry" {
		t.Errorf("history file = %q, %v; want the newest entry", got, err)
	}
}
