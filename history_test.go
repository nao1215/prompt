package prompt

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/assert"
)

func TestDefaultHistoryConfig(t *testing.T) {
	config := defaultHistoryConfig()

	assert.True(t, config.Enabled, "Expected history to be enabled by default")
	assert.Empty(t, config.File, "Expected empty file path by default")
	assert.Equal(t, int64(1024*1024), config.MaxFileSize, "Expected MaxFileSize to be 1MB")
	assert.Equal(t, 3, config.MaxBackups, "Expected MaxBackups to be 3")
}

func TestNewHistoryManager(t *testing.T) {
	// Test with nil config
	hm := newHistoryManager(nil)
	if !hm.isEnabled() {
		t.Error("Expected history to be enabled with nil config")
	}

	// Test with custom config
	config := &HistoryConfig{
		Enabled:     false,
		File:        "/tmp/test_history",
		MaxFileSize: 2048,
		MaxBackups:  5,
	}
	hm = newHistoryManager(config)
	if hm.isEnabled() {
		t.Error("Expected history to be disabled")
	}
}

func TestHistoryManagerBasicOperations(t *testing.T) {
	config := &HistoryConfig{
		Enabled:     true,
		File:        "", // Memory only
		MaxFileSize: 1024,
		MaxBackups:  3,
	}
	hm := newHistoryManager(config)

	// Test empty history
	history := hm.getHistory()
	if len(history) != 0 {
		t.Error("Expected empty history initially")
	}

	// Test adding entries
	hm.addEntry("command1")
	hm.addEntry("command2")
	hm.addEntry("command2") // Consecutive duplicate should be ignored
	hm.addEntry("command3")

	history = hm.getHistory()
	expected := []string{"command1", "command2", "command3"}
	if len(history) != len(expected) {
		t.Errorf("Expected %d entries, got %d", len(expected), len(history))
	}
	for i, cmd := range expected {
		if history[i] != cmd {
			t.Errorf("Expected history[%d] = %q, got %q", i, cmd, history[i])
		}
	}

	// Test clear history
	hm.clearHistory()
	history = hm.getHistory()
	if len(history) != 0 {
		t.Error("Expected empty history after clear")
	}

	// Test set history
	newHistory := []string{"cmd1", "cmd2", "cmd3"}
	hm.setHistory(newHistory)
	history = hm.getHistory()
	if len(history) != len(newHistory) {
		t.Errorf("Expected %d entries, got %d", len(newHistory), len(history))
	}
}

func TestHistoryManagerDisabled(t *testing.T) {
	config := &HistoryConfig{
		Enabled: false,
	}
	hm := newHistoryManager(config)

	// All operations should be no-op when disabled
	hm.addEntry("command1")
	history := hm.getHistory()
	if len(history) != 0 {
		t.Error("Expected empty history when disabled")
	}

	hm.setHistory([]string{"cmd1", "cmd2"})
	history = hm.getHistory()
	if len(history) != 0 {
		t.Error("Expected empty history when disabled")
	}

	hm.clearHistory()
	// Should not panic
}

func TestHistoryFilePersistence(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "test_history")

	config := &HistoryConfig{
		Enabled:     true,
		File:        historyFile,
		MaxFileSize: 1024,
		MaxBackups:  3,
	}

	// Create first history manager and add some entries
	hm1 := newHistoryManager(config)
	hm1.addEntry("command1")
	hm1.addEntry("command2")
	hm1.addEntry("command3")

	// Save history
	err := hm1.saveHistory()
	if err != nil {
		t.Fatalf("Failed to save history: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(historyFile); os.IsNotExist(err) {
		t.Fatal("History file was not created")
	}

	// Create second history manager and load history
	hm2 := newHistoryManager(config)
	err = hm2.loadHistory()
	if err != nil {
		t.Fatalf("Failed to load history: %v", err)
	}

	// Verify loaded history matches saved history
	originalHistory := hm1.getHistory()
	loadedHistory := hm2.getHistory()

	if len(originalHistory) != len(loadedHistory) {
		t.Errorf("Expected %d entries, got %d", len(originalHistory), len(loadedHistory))
	}

	for i, cmd := range originalHistory {
		if loadedHistory[i] != cmd {
			t.Errorf("Expected history[%d] = %q, got %q", i, cmd, loadedHistory[i])
		}
	}
}

func TestHistoryFileRotation(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "test_history")

	config := &HistoryConfig{
		Enabled:     true,
		File:        historyFile,
		MaxFileSize: 50, // Very small size to trigger rotation
		MaxBackups:  2,
	}

	hm := newHistoryManager(config)

	// Add many entries to exceed file size
	for i := range 20 {
		hm.addEntry("very long command that will make the file large enough to trigger rotation " + strings.Repeat("x", i))
	}

	// Save history (should trigger rotation)
	err := hm.saveHistory()
	if err != nil {
		t.Fatalf("Failed to save history: %v", err)
	}

	// Check that backup files were created
	backup1 := historyFile + ".1"
	if _, err := os.Stat(backup1); os.IsNotExist(err) {
		// This is okay if the rotation was smart enough to keep the file small
		t.Logf("Backup file %s not created (this may be expected)", backup1)
	}

	// Verify main file still exists and has content
	if _, err := os.Stat(historyFile); os.IsNotExist(err) {
		t.Fatal("Main history file should still exist after rotation")
	}

	// Load and verify we still have some history
	hm2 := newHistoryManager(config)
	err = hm2.loadHistory()
	if err != nil {
		t.Fatalf("Failed to load history after rotation: %v", err)
	}

	history := hm2.getHistory()
	if len(history) == 0 {
		t.Error("Expected some history to remain after rotation")
	}
}

func TestHistoryFileRotationNoBackups(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "test_history")

	config := &HistoryConfig{
		Enabled:     true,
		File:        historyFile,
		MaxFileSize: 50,
		MaxBackups:  0, // No backups
	}

	hm := newHistoryManager(config)

	// Add entries to exceed file size
	for i := range 10 {
		hm.addEntry("command that will make file large " + strings.Repeat("x", i*5))
	}

	// Save history
	err := hm.saveHistory()
	if err != nil {
		t.Fatalf("Failed to save history: %v", err)
	}

	// Verify no backup files were created
	backup1 := historyFile + ".1"
	if _, err := os.Stat(backup1); !os.IsNotExist(err) {
		t.Error("Backup file should not exist when MaxBackups is 0")
	}
}

func TestPromptHistoryIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "prompt_history")

	config := Config{
		Prefix: "test> ",
		HistoryConfig: &HistoryConfig{
			Enabled:     true,
			MaxEntries:  100,
			File:        historyFile,
			MaxFileSize: 1024,
			MaxBackups:  3,
		},
	}

	// Create prompt and add some history
	p := newForTestingWithConfig(t, config, "")

	p.AddHistory("command1")
	p.AddHistory("command2")
	p.AddHistory("command3")

	history := p.GetHistory()
	if len(history) != 3 {
		t.Errorf("Expected 3 history entries, got %d", len(history))
	}

	// Clear history
	p.ClearHistory()
	history = p.GetHistory()
	if len(history) != 0 {
		t.Error("Expected empty history after clear")
	}

	// Set new history
	newHistory := []string{"new1", "new2", "new3", "new4"}
	p.SetHistory(newHistory)
	history = p.GetHistory()
	if len(history) != len(newHistory) {
		t.Errorf("Expected %d entries, got %d", len(newHistory), len(history))
	}

	// Close to trigger save
	err := p.Close()
	if err != nil {
		t.Fatalf("Failed to close prompt: %v", err)
	}
}

func TestPromptHistoryDisabled(t *testing.T) {
	config := Config{
		Prefix: "test> ",
		HistoryConfig: &HistoryConfig{
			Enabled: false,
		},
	}

	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Adding history should be no-op when disabled
	p.AddHistory("command1")
	history := p.GetHistory()
	if len(history) != 0 {
		t.Error("Expected empty history when disabled")
	}
}

func TestHistoryLoadNonExistentFile(t *testing.T) {
	config := &HistoryConfig{
		Enabled: true,
		File:    "/tmp/non_existent_history_file_12345",
	}

	hm := newHistoryManager(config)
	err := hm.loadHistory()
	if err != nil {
		t.Errorf("Loading non-existent file should not error, got: %v", err)
	}

	history := hm.getHistory()
	if len(history) != 0 {
		t.Error("Expected empty history when file doesn't exist")
	}
}

func TestHistoryFileRotationDetailed(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "detailed_history")

	config := &HistoryConfig{
		Enabled:     true,
		File:        historyFile,
		MaxFileSize: 100, // Very small to trigger rotation
		MaxBackups:  2,
	}

	hm := newHistoryManager(config)

	// Add enough content to trigger rotation
	longCommand := "very long command that will exceed the file size limit " + strings.Repeat("x", 50)
	for i := range 5 {
		hm.addEntry(fmt.Sprintf("%s_%d", longCommand, i))
	}

	// Save to create the file
	err := hm.saveHistory()
	if err != nil {
		t.Fatalf("Failed to save history: %v", err)
	}

	// Add more content to trigger rotation
	for i := range 10 {
		hm.addEntry(fmt.Sprintf("additional_command_%d", i))
	}

	// Save again to trigger rotation
	err = hm.saveHistory()
	if err != nil {
		t.Fatalf("Failed to save history during rotation: %v", err)
	}

	// Verify main file still exists
	if _, err := os.Stat(historyFile); os.IsNotExist(err) {
		t.Error("Main history file should exist after rotation")
	}

	// Test loading after rotation
	hm2 := newHistoryManager(config)
	err = hm2.loadHistory()
	if err != nil {
		t.Fatalf("Failed to load history after rotation: %v", err)
	}

	history := hm2.getHistory()
	if len(history) == 0 {
		t.Error("Should have some history after rotation")
	}
}

func TestHistoryRotationWithMultipleBackups(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "multi_backup_history")

	config := &HistoryConfig{
		Enabled:     true,
		File:        historyFile,
		MaxFileSize: 50, // Small size
		MaxBackups:  3,  // Multiple backups
	}

	hm := newHistoryManager(config)

	// Create initial content
	for i := range 5 {
		hm.addEntry(fmt.Sprintf("command_%d_%s", i, strings.Repeat("x", 20)))
	}
	err := hm.saveHistory()
	if err != nil {
		t.Errorf("Failed to save history: %v", err)
	}

	// Force multiple rotations
	for rotation := range 4 {
		for i := range 5 {
			hm.addEntry(fmt.Sprintf("rotation_%d_command_%d_%s", rotation, i, strings.Repeat("y", 20)))
		}
		err := hm.saveHistory()
		if err != nil {
			t.Errorf("Failed to save history: %v", err)
		}
	}

	// Check that we don't have more than MaxBackups+1 files
	files, err := filepath.Glob(historyFile + "*")
	if err != nil {
		t.Fatalf("Failed to glob history files: %v", err)
	}

	maxExpectedFiles := config.MaxBackups + 1 // main file + backups
	if len(files) > maxExpectedFiles {
		t.Errorf("Expected at most %d files, got %d: %v", maxExpectedFiles, len(files), files)
	}
}

func TestHistoryRotationEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("ZeroBackups", func(t *testing.T) {
		historyFile := filepath.Join(tmpDir, "zero_backup_history")
		config := &HistoryConfig{
			Enabled:     true,
			File:        historyFile,
			MaxFileSize: 30,
			MaxBackups:  0, // No backups
		}

		hm := newHistoryManager(config)
		for i := range 10 {
			hm.addEntry(fmt.Sprintf("long_command_%d_%s", i, strings.Repeat("z", 15)))
		}

		err := hm.saveHistory()
		if err != nil {
			t.Fatalf("Failed to save with zero backups: %v", err)
		}

		// Should not create any backup files
		backupFile := historyFile + ".1"
		if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
			t.Error("Should not create backup files when MaxBackups is 0")
		}
	})

	t.Run("FileCreationError", func(t *testing.T) {
		// Create a file, then try to create a directory with same name as parent
		filePath := filepath.Join(tmpDir, "existing_file")
		if err := os.WriteFile(filePath, []byte("content"), 0600); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Try to create history file in a path where parent is a file (not directory)
		invalidPath := filepath.Join(filePath, "history") // This should fail because parent is a file
		config := &HistoryConfig{
			Enabled:     true,
			File:        invalidPath,
			MaxFileSize: 1024,
			MaxBackups:  3,
		}

		hm := newHistoryManager(config)
		hm.addEntry("test command")

		err := hm.saveHistory()
		if err == nil {
			t.Error("Expected error when saving to invalid path")
		}
	})

	t.Run("RotationIfNeededNoFile", func(_ *testing.T) {
		config := &HistoryConfig{
			Enabled:     true,
			File:        "", // No file
			MaxFileSize: 1024,
			MaxBackups:  3,
		}

		hm := newHistoryManager(config)
		// Should not error when no file is configured
		// This tests the rotateIfNeeded function directly
		_ = hm // Test passes if no panic occurs during creation
	})
}

func TestHistoryRotationBoundsTheFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "history")
	const maxFileSize = 200
	config := &HistoryConfig{
		Enabled:     true,
		File:        file,
		MaxEntries:  1000,
		MaxFileSize: maxFileSize,
		MaxBackups:  3,
	}

	hm := newHistoryManager(config)
	for i := range 40 {
		hm.addEntry(strings.Repeat("x", 20) + string(rune('a'+i%26)))
		if err := hm.saveHistory(); err != nil {
			t.Fatalf("saveHistory() error = %v", err)
		}

		info, err := os.Stat(file)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		// The limit applies to what is written, not only to when the file is
		// rotated. Writing a file the size of the one just rotated away left it
		// over the limit the moment it appeared, so the next save rotated again
		// and within MaxBackups saves every backup held a copy of the newest
		// history.
		if info.Size() > maxFileSize {
			t.Fatalf("after %d entries the file is %d bytes, over the %d limit", i+1, info.Size(), maxFileSize)
		}
	}

	// Nothing the session remembers is lost to a save.
	if got := len(hm.getHistory()); got != 40 {
		t.Errorf("the history holds %d entries after saving, want 40", got)
	}

	// The entries that did not fit are in the backup, which is what it is for.
	backup, err := os.ReadFile(filepath.Clean(file + ".1"))
	if err != nil {
		t.Fatalf("reading the backup: %v", err)
	}
	if !strings.Contains(string(backup), "xxxxxxxxxxxxxxxxxxxxa") {
		t.Errorf("the backup does not hold the oldest entries: %q", backup)
	}

	// Rotation happens when the file fills, not on every save.
	if _, err := os.Stat(file + ".3"); err == nil {
		t.Errorf("the file rotated often enough to reach a third backup over 40 saves")
	}

	// What was written reads back.
	reloaded := newHistoryManager(&HistoryConfig{Enabled: true, File: file, MaxEntries: 1000})
	if err := reloaded.loadHistory(); err != nil {
		t.Fatalf("loadHistory() error = %v", err)
	}
	got := reloaded.getHistory()
	if len(got) == 0 {
		t.Fatal("the rotated file holds nothing")
	}
	if want := hm.getHistory(); got[len(got)-1] != want[len(want)-1] {
		t.Errorf("the file's newest entry is %q, want %q", got[len(got)-1], want[len(want)-1])
	}
}

// TestHistoryFileIsCreatedForItsOwnerAlone pins the mode. What the user typed is
// where a password given on a command line ends up, and os.Create asked for 0666
// and let the umask decide, which on a common default left the file readable by
// everyone.
func TestHistoryFileIsCreatedForItsOwnerAlone(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windowsOS {
		t.Skip("file modes do not carry the same meaning on Windows")
	}

	file := filepath.Join(t.TempDir(), "history")
	hm := newHistoryManager(&HistoryConfig{Enabled: true, File: file, MaxEntries: 100})
	hm.addEntry("psql -h db -U admin -W hunter2")
	if err := hm.saveHistory(); err != nil {
		t.Fatalf("saveHistory() error = %v", err)
	}

	info, err := os.Stat(file)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("the history file is %v, want it readable by its owner alone", perm)
	}
}

func TestHistorySearch(t *testing.T) {
	// Create a prompt with test history
	history := []string{
		"git status",
		"git commit -m 'test'",
		"ls -la",
		"grep pattern file.txt",
		"git push origin main",
	}

	t.Run("SearchWithEnter", func(t *testing.T) {
		// Simulate typing "git" and pressing Enter
		mockInput := "git\r"
		p := createPromptWithHistory(history, mockInput)

		result, err := p.searchHistory()
		if err != nil {
			t.Fatalf("searchHistory failed: %v", err)
		}

		// Should return the first match or the search query
		if !strings.Contains(result, "git") {
			t.Errorf("Expected result to contain 'git', got %q", result)
		}
	})

	t.Run("SearchWithEscape", func(t *testing.T) {
		// Simulate typing "git" and pressing Escape
		mockInput := "git\x1b"
		p := createPromptWithHistory(history, mockInput)

		result, err := p.searchHistory()
		if err != nil {
			t.Fatalf("searchHistory failed: %v", err)
		}

		// Should return empty string when cancelled
		if result != "" {
			t.Errorf("Expected empty result when cancelled, got %q", result)
		}
	})

	t.Run("SearchWithCtrlC", func(t *testing.T) {
		// Simulate typing "git" and pressing Ctrl+C
		mockInput := "git\x03"
		p := createPromptWithHistory(history, mockInput)

		result, err := p.searchHistory()
		if err != nil {
			t.Fatalf("searchHistory failed: %v", err)
		}

		// Should return empty string when cancelled
		if result != "" {
			t.Errorf("Expected empty result when cancelled, got %q", result)
		}
	})

	t.Run("every block the search draws is erased again", func(t *testing.T) {
		// The interface is redrawn on every keystroke. Drawing the next one
		// under the last stacked a block per character typed, and none of them
		// was removed when the search ended, so the accepted line appeared at
		// the foot of a column of dead search results.
		var out bytes.Buffer
		p := createPromptWithHistory(history, "git\r")
		p.output = &out

		if _, err := p.searchHistory(); err != nil {
			t.Fatalf("searchHistory failed: %v", err)
		}

		got := out.String()
		drawn := strings.Count(got, "reverse-i-search:")
		erased := strings.Count(got, "\x1b[0J")
		if drawn == 0 {
			t.Fatal("searchHistory() drew no search interface at all")
		}
		if erased != drawn {
			t.Errorf("the search drew %d block(s) and erased %d: what is left on screen is what the prompt is redrawn under", drawn, erased)
		}
		if !strings.HasSuffix(got, "\x1b[0J") {
			t.Errorf("searchHistory() wrote %q, want the interface erased before it returns", got)
		}
	})

	t.Run("a canceled search leaves nothing on screen", func(t *testing.T) {
		var out bytes.Buffer
		p := createPromptWithHistory(history, "git\x1b")
		p.output = &out

		if _, err := p.searchHistory(); err != nil {
			t.Fatalf("searchHistory failed: %v", err)
		}
		if got := out.String(); !strings.HasSuffix(got, "\x1b[0J") {
			t.Errorf("a canceled search wrote %q, want the interface erased before it returns", got)
		}
	})

	t.Run("SearchWithBackspace", func(t *testing.T) {
		// Simulate typing "gitx", backspace, then Enter
		mockInput := "gitx\x7f\r"
		p := createPromptWithHistory(history, mockInput)

		result, err := p.searchHistory()
		if err != nil {
			t.Fatalf("searchHistory failed: %v", err)
		}

		// Should find git commands after backspace removes 'x'
		if !strings.Contains(result, "git") {
			t.Errorf("Expected result to contain 'git', got %q", result)
		}
	})

	t.Run("SearchWithTab", func(t *testing.T) {
		// Simulate typing "git", tab (to cycle through results), then Enter
		mockInput := "git\t\r"
		p := createPromptWithHistory(history, mockInput)

		result, err := p.searchHistory()
		if err != nil {
			t.Fatalf("searchHistory failed: %v", err)
		}

		// Should return a git command
		if !strings.Contains(result, "git") {
			t.Errorf("Expected result to contain 'git', got %q", result)
		}
	})

	t.Run("SearchWithMultipleTabs", func(t *testing.T) {
		// Simulate typing "git", multiple tabs, then Enter
		mockInput := "git\t\t\t\r"
		p := createPromptWithHistory(history, mockInput)

		result, err := p.searchHistory()
		if err != nil {
			t.Fatalf("searchHistory failed: %v", err)
		}

		// Should return a valid result (could be git command or fall back to search query)
		if result == "" {
			t.Error("Expected non-empty result")
		}
		// Accept any reasonable result as cycling behavior may vary
		t.Logf("Search result after multiple tabs: %q", result)
	})

	t.Run("SearchEmptyQuery", func(t *testing.T) {
		// Simulate just pressing Enter with no search query
		mockInput := "\r"
		p := createPromptWithHistory(history, mockInput)

		result, err := p.searchHistory()
		if err != nil {
			t.Fatalf("searchHistory failed: %v", err)
		}

		// Should return the first history item or empty string
		if result != "" && !contains(history, result) {
			t.Errorf("Expected result to be empty or from history, got %q", result)
		}
	})

	t.Run("SearchNoMatches", func(t *testing.T) {
		// Simulate searching for something not in history
		mockInput := "zzznomatch\r"
		p := createPromptWithHistory(history, mockInput)

		result, err := p.searchHistory()
		if err != nil {
			t.Fatalf("searchHistory failed: %v", err)
		}

		// Nothing matched, so Enter has nothing to accept and the caller leaves
		// the line as the search found it. Returning the query put text typed
		// into a search onto the command line, a keystroke away from being run.
		if result != "" {
			t.Errorf("Expected the search to accept nothing, got %q", result)
		}
	})

	t.Run("SearchUnicodeInput", func(t *testing.T) {
		// Test with unicode characters
		unicodeHistory := []string{"こんにちは", "世界", "テスト"}
		mockInput := "こん\r"
		p := createPromptWithHistory(unicodeHistory, mockInput)

		result, err := p.searchHistory()
		if err != nil {
			t.Fatalf("searchHistory failed: %v", err)
		}

		// Should handle unicode correctly
		if !strings.Contains(result, "こん") {
			t.Errorf("Expected result to contain unicode, got %q", result)
		}
	})
}

func TestRenderHistorySearch(t *testing.T) {
	// Create a buffer to capture output
	var output bytes.Buffer
	p := &Prompt{
		config: Config{
			Prefix: "test> ",
			HistoryConfig: &HistoryConfig{
				Enabled:    true,
				MaxEntries: 100,
			},
		},
		output:   &output,
		terminal: newMockTerminal(""),
		keyMap:   NewDefaultKeyMap(),
		history:  []string{"cmd1", "cmd2", "cmd3"},
	}

	t.Run("BasicRender", func(t *testing.T) {
		output.Reset()
		results := []string{"git status", "git commit", "git push"}
		if _, err := p.renderHistorySearch("git", results, 0); err != nil {
			t.Fatalf("renderHistorySearch() error = %v", err)
		}

		outputStr := output.String()
		if !strings.Contains(outputStr, "git") {
			t.Error("Expected output to contain search query 'git'")
		}
		if !strings.Contains(outputStr, "git status") {
			t.Error("Expected output to contain selected result")
		}
	})

	t.Run("RenderWithSelection", func(t *testing.T) {
		output.Reset()
		results := []string{"git status", "git commit", "git push"}
		if _, err := p.renderHistorySearch("git", results, 1); err != nil {
			t.Fatalf("renderHistorySearch() error = %v", err)
		}

		outputStr := output.String()
		if !strings.Contains(outputStr, "git commit") {
			t.Error("Expected output to contain selected result 'git commit'")
		}
	})

	t.Run("RenderEmptyResults", func(t *testing.T) {
		output.Reset()
		results := []string{}
		if _, err := p.renderHistorySearch("nomatch", results, 0); err != nil {
			t.Fatalf("renderHistorySearch() error = %v", err)
		}

		outputStr := output.String()
		if !strings.Contains(outputStr, "nomatch") {
			t.Error("Expected output to contain search query even with no results")
		}
	})

	t.Run("RenderManyResults", func(t *testing.T) {
		output.Reset()
		results := []string{"cmd1", "cmd2", "cmd3", "cmd4", "cmd5", "cmd6", "cmd7"}
		if _, err := p.renderHistorySearch("cmd", results, 2); err != nil {
			t.Fatalf("renderHistorySearch() error = %v", err)
		}

		outputStr := output.String()
		// Should limit to top 5 results (excluding the search prompt line)
		lines := strings.Split(outputStr, "\n")
		resultLines := 0
		for _, line := range lines {
			// Count only the result lines (those that start with spaces)
			if strings.Contains(line, "cmd") && strings.HasPrefix(strings.TrimLeft(line, "\r"), "  ") {
				resultLines++
			}
		}
		if resultLines > 5 {
			t.Errorf("Expected at most 5 result lines, got %d: %v", resultLines, lines)
		}
	})
}

func TestHistorySearchErrorCases(t *testing.T) {
	t.Run("ReadRuneError", func(t *testing.T) {
		// Create a mock terminal that returns an error on read
		p := &Prompt{
			config: Config{
				Prefix: "test> ",
				HistoryConfig: &HistoryConfig{
					Enabled:    true,
					MaxEntries: 100,
				},
			},
			output:   &bytes.Buffer{},
			terminal: &errorMockTerminal{},
			keyMap:   NewDefaultKeyMap(),
			history:  []string{"test"},
		}

		_, err := p.searchHistory()
		if err == nil {
			t.Error("Expected error when readRune fails")
		}
	})
}

// Helper functions for testing

func createPromptWithHistory(history []string, mockInput string) *Prompt {
	return &Prompt{
		config: Config{
			Prefix: "test> ",
			HistoryConfig: &HistoryConfig{
				Enabled:    true,
				MaxEntries: 100,
			},
		},
		output:   &bytes.Buffer{},
		terminal: newMockTerminal(mockInput),
		keyMap:   NewDefaultKeyMap(),
		history:  history,
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// errorMockTerminal is a mock terminal that returns errors for testing
type errorMockTerminal struct {
	mockTerminal
}

func (t *errorMockTerminal) ReadRune() (rune, int, error) {
	return 0, 0, io.ErrUnexpectedEOF
}

func TestExpandHistoryPath(t *testing.T) {
	t.Run("EmptyPath", func(t *testing.T) {
		result, err := expandHistoryPath("")
		if err != nil {
			t.Errorf("expandHistoryPath(\"\") failed: %v", err)
		}
		if result != "" {
			t.Errorf("Expected empty result for empty path, got %q", result)
		}
	})

	t.Run("AbsolutePath", func(t *testing.T) {
		var absPath string
		if strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") {
			absPath = "C:\\tmp\\test_history"
		} else {
			absPath = "/tmp/test_history"
		}
		result, err := expandHistoryPath(absPath)
		if err != nil {
			t.Errorf("expandHistoryPath(%q) failed: %v", absPath, err)
		}
		if !filepath.IsAbs(result) {
			t.Errorf("Expected result to be absolute path, got %q", result)
		}
		// On Windows, the path might be normalized differently
		if filepath.Clean(result) != filepath.Clean(absPath) && result != absPath {
			t.Logf("Path normalized from %q to %q", absPath, result)
		}
	})

	t.Run("RelativePath", func(t *testing.T) {
		relPath := "./test_history"
		result, err := expandHistoryPath(relPath)
		if err != nil {
			t.Errorf("expandHistoryPath(%q) failed: %v", relPath, err)
		}

		expected, err := filepath.Abs(relPath)
		if err != nil {
			t.Fatalf("Failed to get absolute path: %v", err)
		}
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})

	t.Run("HomeDirectoryPath", func(t *testing.T) {
		homePath := "~/.test_history"
		result, err := expandHistoryPath(homePath)
		if err != nil {
			t.Errorf("expandHistoryPath(%q) failed: %v", homePath, err)
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("Failed to get user home dir: %v", err)
		}
		expected := filepath.Join(homeDir, ".test_history")
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})

	t.Run("HomeDirectoryOnly", func(t *testing.T) {
		result, err := expandHistoryPath("~")
		if err != nil {
			t.Errorf("expandHistoryPath(\"~\") failed: %v", err)
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("Failed to get user home dir: %v", err)
		}
		if result != homeDir {
			t.Errorf("Expected %q, got %q", homeDir, result)
		}
	})

	t.Run("HomeDirectorySubpath", func(t *testing.T) {
		homePath := "~/config/.app_history"
		result, err := expandHistoryPath(homePath)
		if err != nil {
			t.Errorf("expandHistoryPath(%q) failed: %v", homePath, err)
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("Failed to get user home dir: %v", err)
		}
		expected := filepath.Join(homeDir, "config", ".app_history")
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})
}

func TestNewHistoryManagerPathExpansion(t *testing.T) {
	t.Run("WithHomePath", func(t *testing.T) {
		config := &HistoryConfig{
			Enabled:     true,
			File:        "~/.test_history_manager",
			MaxFileSize: 1024,
			MaxBackups:  3,
		}

		hm := newHistoryManager(config)

		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("Failed to get user home dir: %v", err)
		}
		expected := filepath.Join(homeDir, ".test_history_manager")
		if hm.config.File != expected {
			t.Errorf("Expected expanded path %q, got %q", expected, hm.config.File)
		}
	})

	t.Run("WithRelativePath", func(t *testing.T) {
		config := &HistoryConfig{
			Enabled:     true,
			File:        "./relative_history",
			MaxFileSize: 1024,
			MaxBackups:  3,
		}

		hm := newHistoryManager(config)

		expected, err := filepath.Abs("./relative_history")
		if err != nil {
			t.Fatalf("Failed to get absolute path: %v", err)
		}
		if hm.config.File != expected {
			t.Errorf("Expected absolute path %q, got %q", expected, hm.config.File)
		}
	})

	t.Run("WithAbsolutePath", func(t *testing.T) {
		var absPath string
		if strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") {
			absPath = "C:\\tmp\\absolute_history"
		} else {
			absPath = "/tmp/absolute_history"
		}
		config := &HistoryConfig{
			Enabled:     true,
			File:        absPath,
			MaxFileSize: 1024,
			MaxBackups:  3,
		}

		hm := newHistoryManager(config)

		if !filepath.IsAbs(hm.config.File) {
			t.Errorf("Expected result to be absolute path, got %q", hm.config.File)
		}
		// On Windows, the path might be normalized differently
		if filepath.Clean(hm.config.File) != filepath.Clean(absPath) && hm.config.File != absPath {
			t.Logf("Path normalized from %q to %q", absPath, hm.config.File)
		}
	})
}

func TestHistoryFileOperationsWithExpandedPaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with a path that needs expansion
	relativeFile := "./test_expanded_history"

	config := &HistoryConfig{
		Enabled:     true,
		File:        relativeFile,
		MaxFileSize: 1024,
		MaxBackups:  3,
	}

	// Change to temp dir for predictable relative path behavior
	t.Chdir(tmpDir)

	hm := newHistoryManager(config)

	// Verify path was expanded
	expectedPath := filepath.Join(tmpDir, "test_expanded_history")
	if hm.config.File != expectedPath {
		t.Errorf("Expected expanded path %q, got %q", expectedPath, hm.config.File)
	}

	// Test save and load operations
	hm.addEntry("test command 1")
	hm.addEntry("test command 2")

	err := hm.saveHistory()
	if err != nil {
		t.Fatalf("Failed to save history: %v", err)
	}

	// Verify file was created at expanded path
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("History file was not created at expected path %q", expectedPath)
	}

	// Test loading
	hm2 := newHistoryManager(config)
	err = hm2.loadHistory()
	if err != nil {
		t.Fatalf("Failed to load history: %v", err)
	}

	loadedHistory := hm2.getHistory()
	if len(loadedHistory) != 2 {
		t.Errorf("Expected 2 history entries, got %d", len(loadedHistory))
	}
}

func TestGetDefaultHistoryFile(t *testing.T) {
	result := GetDefaultHistoryFile()

	// Should return an expanded path
	if !filepath.IsAbs(result) {
		t.Errorf("Expected absolute path, got %q", result)
	}

	// Should contain .config/prompt/history pattern
	if !strings.Contains(result, filepath.Join(".config", "prompt", "history")) {
		t.Errorf("Expected path to contain .config/prompt/history, got %q", result)
	}
}

func TestRotateHistoryFile(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "rotate_test_history")

	// Create initial history file
	initialContent := []byte("line1\nline2\nline3\n")
	err := os.WriteFile(historyFile, initialContent, 0600)
	if err != nil {
		t.Fatalf("Failed to create initial file: %v", err)
	}

	config := &HistoryConfig{
		Enabled:     true,
		File:        historyFile,
		MaxFileSize: 100,
		MaxBackups:  2,
	}

	hm := newHistoryManager(config)

	// Test rotation
	err = hm.rotateHistoryFile()
	if err != nil {
		t.Fatalf("rotateHistoryFile failed: %v", err)
	}

	// Check backup file was created
	backupFile := historyFile + ".1"
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		t.Error("Backup file .1 should have been created")
	}

	// Check backup content matches original
	backupContent, err := os.ReadFile(filepath.Clean(backupFile)) // #nosec G304 - test file path is controlled
	if err != nil {
		t.Fatalf("Failed to read backup file: %v", err)
	}
	if !bytes.Equal(backupContent, initialContent) {
		t.Error("Backup content doesn't match original")
	}

	// Test multiple rotations
	err = os.WriteFile(historyFile, []byte("new content\n"), 0600)
	if err != nil {
		t.Fatalf("Failed to write new content: %v", err)
	}

	err = hm.rotateHistoryFile()
	if err != nil {
		t.Fatalf("Second rotation failed: %v", err)
	}

	// Check .2 file was created
	backup2File := historyFile + ".2"
	if _, err := os.Stat(backup2File); os.IsNotExist(err) {
		t.Error("Backup file .2 should have been created")
	}

	// Test rotation with max backups reached
	err = os.WriteFile(historyFile, []byte("third content\n"), 0600)
	if err != nil {
		t.Fatalf("Failed to write third content: %v", err)
	}

	err = hm.rotateHistoryFile()
	if err != nil {
		t.Fatalf("Third rotation failed: %v", err)
	}

	// Check that we don't have more than MaxBackups
	backup3File := historyFile + ".3"
	if _, err := os.Stat(backup3File); !os.IsNotExist(err) {
		t.Error("Backup file .3 should NOT exist (MaxBackups=2)")
	}
}

func TestLoadHistoryCorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "corrupted_history")

	// Create a file with mixed valid and invalid content
	content := "valid line 1\n\x00\x00\x00\nvalid line 2\n"
	err := os.WriteFile(historyFile, []byte(content), 0600)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	config := &HistoryConfig{
		Enabled:     true,
		File:        historyFile,
		MaxFileSize: 1024,
		MaxBackups:  3,
	}

	hm := newHistoryManager(config)
	err = hm.loadHistory()
	if err != nil {
		t.Fatalf("LoadHistory should handle corrupted content gracefully: %v", err)
	}

	history := hm.getHistory()
	// Should load the valid lines
	if len(history) == 0 {
		t.Error("Should have loaded some valid entries")
	}
}

func TestSaveHistoryPermissionError(t *testing.T) {
	// A read-only directory is not read-only to root, and a directory mode does
	// not stop a write on Windows. Those are the conditions the test needs;
	// whether it is running in CI is not one of them, and asking that instead
	// left the error path untested everywhere a developer works.
	if runtime.GOOS == windowsOS {
		t.Skip("a directory mode does not stop a write on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes to a read-only directory")
	}

	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	err := os.Mkdir(readOnlyDir, 0750) // #nosec G301 - test directory permissions
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Make directory read-only
	err = os.Chmod(readOnlyDir, 0555) // #nosec G302 - test requires specific permissions
	if err != nil {
		t.Fatalf("Failed to set permissions: %v", err)
	}
	defer os.Chmod(readOnlyDir, 0755) // #nosec G302 - test cleanup

	historyFile := filepath.Join(readOnlyDir, "history")
	config := &HistoryConfig{
		Enabled:     true,
		File:        historyFile,
		MaxFileSize: 1024,
		MaxBackups:  3,
	}

	hm := newHistoryManager(config)
	hm.addEntry("test")

	err = hm.saveHistory()
	if err == nil {
		t.Error("Expected error when saving to read-only directory")
	}
}

func TestHistoryManagerEdgeCases(t *testing.T) {
	t.Run("LoadEmptyFile", func(t *testing.T) {
		tmpDir := t.TempDir()
		historyFile := filepath.Join(tmpDir, "empty_history")

		// Create empty file
		err := os.WriteFile(historyFile, []byte(""), 0600)
		if err != nil {
			t.Fatalf("Failed to create empty file: %v", err)
		}

		config := &HistoryConfig{
			Enabled:     true,
			File:        historyFile,
			MaxFileSize: 1024,
			MaxBackups:  3,
		}

		hm := newHistoryManager(config)
		err = hm.loadHistory()
		if err != nil {
			t.Fatalf("LoadHistory failed on empty file: %v", err)
		}

		history := hm.getHistory()
		if len(history) != 0 {
			t.Errorf("Expected empty history from empty file, got %d entries", len(history))
		}
	})

	t.Run("LoadFileWithBlankLines", func(t *testing.T) {
		tmpDir := t.TempDir()
		historyFile := filepath.Join(tmpDir, "blank_lines_history")

		// Create file with blank lines
		content := "line1\n\n\nline2\n\nline3\n"
		err := os.WriteFile(historyFile, []byte(content), 0600)
		if err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}

		config := &HistoryConfig{
			Enabled:     true,
			File:        historyFile,
			MaxFileSize: 1024,
			MaxBackups:  3,
		}

		hm := newHistoryManager(config)
		err = hm.loadHistory()
		if err != nil {
			t.Fatalf("LoadHistory failed: %v", err)
		}

		history := hm.getHistory()
		// Should skip blank lines
		expectedCount := 3
		if len(history) != expectedCount {
			t.Errorf("Expected %d non-blank entries, got %d", expectedCount, len(history))
		}
	})

	t.Run("SaveWithNoEntries", func(t *testing.T) {
		tmpDir := t.TempDir()
		historyFile := filepath.Join(tmpDir, "no_entries_history")

		config := &HistoryConfig{
			Enabled:     true,
			File:        historyFile,
			MaxFileSize: 1024,
			MaxBackups:  3,
		}

		hm := newHistoryManager(config)
		// Don't add any entries

		err := hm.saveHistory()
		if err != nil {
			t.Fatalf("SaveHistory failed with no entries: %v", err)
		}

		// File should be created but empty
		info, err := os.Stat(historyFile)
		if os.IsNotExist(err) {
			t.Error("History file should be created even with no entries")
		}
		if info != nil && info.Size() > 1 { // Allow for potential newline
			t.Errorf("Expected empty or near-empty file, got size %d", info.Size())
		}
	})
}

func TestHistoryFileExpansionErrors(t *testing.T) {
	t.Run("InvalidHomePath", func(t *testing.T) {
		// Test with a path that looks like home but isn't valid
		path := "~nonexistentuser/.history"
		result, err := expandHistoryPath(path)
		// Should still process it but may not expand correctly
		if err != nil {
			// This is acceptable
			t.Logf("expandHistoryPath returned error as expected: %v", err)
		} else {
			// Should at least return something
			if result == "" {
				t.Error("Expected non-empty result even for invalid home path")
			}
		}
	})
}

func TestRotationWithZeroMaxBackups(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "zero_backups_test")

	// Create initial file
	err := os.WriteFile(historyFile, []byte("initial content\n"), 0600)
	if err != nil {
		t.Fatalf("Failed to create initial file: %v", err)
	}

	config := &HistoryConfig{
		Enabled:     true,
		File:        historyFile,
		MaxFileSize: 10, // Very small to trigger rotation
		MaxBackups:  0,  // No backups
	}

	hm := newHistoryManager(config)

	// Try to rotate
	err = hm.rotateHistoryFile()
	if err != nil {
		t.Fatalf("rotateHistoryFile failed: %v", err)
	}

	// No backup files should exist
	backupFile := historyFile + ".1"
	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Error("No backup files should be created when MaxBackups is 0")
	}

	// Original file should still exist
	if _, err := os.Stat(historyFile); os.IsNotExist(err) {
		t.Error("Original file should still exist")
	}
}

func TestHistoryFileRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		entries []string
	}{
		{
			name:    "an entry keeps the spaces it was submitted with",
			entries: []string{"   indented cmd   "},
		},
		{
			name:    "a multi-line entry reloads as one entry",
			entries: []string{"line1\nline2"},
		},
		{
			name:    "a carriage return inside an entry survives",
			entries: []string{"before\rafter"},
		},
		{
			name:    "a backslash is not eaten by the escaping",
			entries: []string{`SELECT 'a\nb'`, `C:\path\to\file`},
		},
		{
			name:    "a tab-only entry is kept",
			entries: []string{"\t"},
		},
		{
			name:    "an invalid UTF-8 byte is not replaced",
			entries: []string{"caf\xe9 latte"},
		},
		{
			name:    "several entries keep their order",
			entries: []string{"first", "  second  ", "third\nfourth"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := &HistoryConfig{
				Enabled:     true,
				File:        filepath.Join(t.TempDir(), "history"),
				MaxFileSize: 1024 * 1024,
				MaxBackups:  3,
			}

			saver := newHistoryManager(config)
			saver.setHistory(tt.entries)
			if err := saver.saveHistory(); err != nil {
				t.Fatalf("SaveHistory failed: %v", err)
			}

			loader := newHistoryManager(config)
			if err := loader.loadHistory(); err != nil {
				t.Fatalf("LoadHistory failed: %v", err)
			}

			got := loader.getHistory()
			if len(got) != len(tt.entries) {
				t.Fatalf("got %d entries %q, want %d entries %q", len(got), got, len(tt.entries), tt.entries)
			}
			for i, want := range tt.entries {
				if got[i] != want {
					t.Errorf("entry %d: got %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

func TestLoadHistorySkipsEmptyLinesAndCRLF(t *testing.T) {
	t.Parallel()

	historyFile := filepath.Join(t.TempDir(), "history")
	// A file written on Windows, with a blank line between entries.
	content := "first\r\n\r\nsecond\r\n"
	if err := os.WriteFile(historyFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write history file: %v", err)
	}

	hm := newHistoryManager(&HistoryConfig{
		Enabled:     true,
		File:        historyFile,
		MaxFileSize: 1024 * 1024,
		MaxBackups:  3,
	})
	if err := hm.loadHistory(); err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}

	want := []string{"first", "second"}
	got := hm.getHistory()
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestHistoryLineEncodingRoundTrip(t *testing.T) {
	t.Parallel()

	roundTrip := func(entry string) bool {
		decoded, ok := decodeHistoryLine(encodeHistoryLine(entry))
		if entry == "" {
			return !ok
		}
		return ok && decoded == entry
	}

	config := &quick.Config{
		MaxCount: 500,
		Rand:     rand.New(rand.NewSource(20260809)), //nolint:gosec // reproducible test input, not security
	}
	if err := quick.Check(roundTrip, config); err != nil {
		t.Errorf("encode/decode round trip failed: %v", err)
	}
}

// TestHistoryManagerKeepsAtMostMaxEntries pins the limit at the layer that owns
// the history. It used to be applied only by the prompt, which read the
// manager's history back and pushed a shortened copy down again, so a manager
// used on its own grew without bound.
func TestHistoryManagerKeepsAtMostMaxEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxEntries int
		added      []string
		want       []string
	}{
		{
			name:       "fewer entries than the limit are all kept",
			maxEntries: 3,
			added:      []string{"a", "b"},
			want:       []string{"a", "b"},
		},
		{
			name:       "exactly the limit is kept",
			maxEntries: 3,
			added:      []string{"a", "b", "c"},
			want:       []string{"a", "b", "c"},
		},
		{
			name:       "the oldest entries are dropped past the limit",
			maxEntries: 3,
			added:      []string{"a", "b", "c", "d", "e"},
			want:       []string{"c", "d", "e"},
		},
		{
			name:       "a limit of one keeps the newest entry",
			maxEntries: 1,
			added:      []string{"a", "b", "c"},
			want:       []string{"c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hm := newHistoryManager(&HistoryConfig{Enabled: true, MaxEntries: tt.maxEntries})
			for _, entry := range tt.added {
				hm.addEntry(entry)
			}
			got := hm.getHistory()
			if !slices.Equal(got, tt.want) {
				t.Errorf("GetHistory() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHistoryManagerLoadHistoryReplacesWhatItHolds covers reloading. A load is a
// read of the file, but it appended, so asking a manager for the file's current
// contents duplicated every entry it already had.
func TestHistoryManagerLoadHistoryReplacesWhatItHolds(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "history")
	saved := newHistoryManager(&HistoryConfig{Enabled: true, File: file})
	saved.addEntry("one")
	saved.addEntry("two")
	if err := saved.saveHistory(); err != nil {
		t.Fatalf("SaveHistory() error = %v", err)
	}

	loaded := newHistoryManager(&HistoryConfig{Enabled: true, File: file})
	for range 2 {
		if err := loaded.loadHistory(); err != nil {
			t.Fatalf("LoadHistory() error = %v", err)
		}
	}

	want := []string{"one", "two"}
	if got := loaded.getHistory(); !slices.Equal(got, want) {
		t.Errorf("GetHistory() = %q, want %q", got, want)
	}
}

// TestHistoryManagerLoadHistoryKeepsTheHistoryWhenThereIsNoFile pins the other
// half of replacing: a file that does not exist yet says nothing about the
// history, and must not empty it.
func TestHistoryManagerLoadHistoryKeepsTheHistoryWhenThereIsNoFile(t *testing.T) {
	t.Parallel()

	hm := newHistoryManager(&HistoryConfig{Enabled: true, File: filepath.Join(t.TempDir(), "absent")})
	hm.addEntry("kept")
	if err := hm.loadHistory(); err != nil {
		t.Fatalf("LoadHistory() error = %v", err)
	}
	if got := hm.getHistory(); !slices.Equal(got, []string{"kept"}) {
		t.Errorf("GetHistory() = %q, want [kept]", got)
	}
}

// TestHistoryManagerLoadHistoryKeepsAtMostMaxEntries covers a file holding more
// than the limit, which is what a file written by an older version, or by hand,
// can hold.
func TestHistoryManagerLoadHistoryKeepsAtMostMaxEntries(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "history")
	if err := os.WriteFile(file, []byte("a\nb\nc\nd\ne\n"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	hm := newHistoryManager(&HistoryConfig{Enabled: true, File: file, MaxEntries: 2})
	if err := hm.loadHistory(); err != nil {
		t.Fatalf("LoadHistory() error = %v", err)
	}
	want := []string{"d", "e"}
	if got := hm.getHistory(); !slices.Equal(got, want) {
		t.Errorf("GetHistory() = %q, want %q", got, want)
	}
}

// TestRenderHistorySearchDrawsOneRowPerResult covers reverse search over a
// statement entered across several lines, which is what the history file's
// escaping exists to preserve. Each result is one row of the block, but an entry
// holding a newline was written to the terminal as it was stored, so the block
// drew rows it had not counted and they stayed on screen when the search closed.
func TestRenderHistorySearchDrawsOneRowPerResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		results []string
		want    int
	}{
		{name: "single line results", results: []string{"select 1", "select 2"}, want: 3},
		{name: "a result entered across two lines", results: []string{"select *\nfrom t"}, want: 2},
		{name: "a result holding an escape sequence", results: []string{"\x1b[2Jselect 1"}, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			p := newTestPrompt(newMockTerminal(""))
			p.output = &out

			rows, err := p.renderHistorySearch("sel", tt.results, 0)
			if err != nil {
				t.Fatalf("renderHistorySearch() error = %v", err)
			}
			if rows != tt.want {
				t.Errorf("renderHistorySearch() reported %d rows, want %d", rows, tt.want)
			}
			screen := newScreenModel(40)
			screen.feed(out.String())
			if drawn := len(screen.rows()); drawn != tt.want {
				t.Errorf("renderHistorySearch() drew %d rows, want %d: %q", drawn, tt.want, screen.rows())
			}
		})
	}
}

// TestSearchHistoryConsumesEscapeSequences covers keys the terminal sends as an
// escape sequence while reverse search is open. The search read raw runes, so
// the ESC that introduced an arrow key cancelled the search and the rest of the
// sequence arrived at the read loop as typing: pressing Up left `[A` in the line.
func TestSearchHistoryConsumesEscapeSequences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "escape cancels and leaves the line empty", input: "\x12sel\x1b\r", want: ""},
		{name: "up moves to the previous match", input: "\x12sel\x1b[A\r\r", want: "select 2"},
		{name: "down moves to the next match", input: "\x12sel\x1b[B\r\r", want: "select 2"},
		{name: "home is consumed and changes nothing", input: "\x12sel\x1b[H\r\r", want: "select 1"},
		{name: "delete is consumed and changes nothing", input: "\x12sel\x1b[3~\r\r", want: "select 1"},
		{name: "a function key is consumed", input: "\x12sel\x1bOP\r\r", want: "select 1"},
		{name: "tab still moves to the next match", input: "\x12sel\t\r\r", want: "select 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPrompt(newMockTerminal(tt.input), WithMemoryHistory(10))
			p.SetHistory([]string{"select 1", "select 2"})
			got, err := p.Run()
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Run() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestLoadHistoryReadsAnEntryOfAnyLength covers what the prompt itself can
// write. A pasted statement is content, so an entry has no length limit, but the
// read used a bufio.Scanner and refused a line over 64KB: the load failed, the
// whole history was lost with it, and New -- which returns that error -- could
// not open a prompt again until the file was deleted by hand.
func TestLoadHistoryReadsAnEntryOfAnyLength(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "history")
	long := "INSERT INTO t VALUES ('" + strings.Repeat("x", 70000) + "');"

	saved := newHistoryManager(&HistoryConfig{Enabled: true, File: file, MaxEntries: 100, MaxFileSize: 1 << 30})
	saved.addEntry("select 1;")
	saved.addEntry(long)
	saved.addEntry("select 2;")
	if err := saved.saveHistory(); err != nil {
		t.Fatalf("saveHistory() error = %v", err)
	}

	loaded := newHistoryManager(&HistoryConfig{Enabled: true, File: file, MaxEntries: 100})
	if err := loaded.loadHistory(); err != nil {
		t.Fatalf("loadHistory() error = %v", err)
	}
	want := []string{"select 1;", long, "select 2;"}
	if got := loaded.getHistory(); !slices.Equal(got, want) {
		t.Errorf("loaded %d entries, want %d; the long one came back as %d characters",
			len(got), len(want), len(got[min(1, len(got)-1)]))
	}
}

// TestLoadHistoryKeepsALastLineWithoutANewline covers a file another program
// wrote, or one cut short: the last line is an entry whether or not it ends the
// way this package writes it.
func TestLoadHistoryKeepsALastLineWithoutANewline(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "history")
	if err := os.WriteFile(file, []byte("first\nsecond"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	hm := newHistoryManager(&HistoryConfig{Enabled: true, File: file, MaxEntries: 100})
	if err := hm.loadHistory(); err != nil {
		t.Fatalf("loadHistory() error = %v", err)
	}
	want := []string{"first", "second"}
	if got := hm.getHistory(); !slices.Equal(got, want) {
		t.Errorf("getHistory() = %q, want %q", got, want)
	}
}

// FuzzHistoryLineRoundTrip pins the encoding a history file is written in. An
// entry is whatever the user submitted, including a line break, a backslash, and
// bytes that are not valid UTF-8, and it has to come back as it went in --
// otherwise the file quietly rewrites what was typed.
func FuzzHistoryLineRoundTrip(f *testing.F) {
	for _, s := range []string{"", "a", "a\\b", "a\nb", "a\r\nb", "\\", "\\\\", "\\n", "日本語", "  spaced  ", "\x00\x01"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, entry string) {
		encoded := encodeHistoryLine(entry)
		if strings.ContainsAny(encoded, "\n\r") {
			t.Fatalf("encodeHistoryLine(%q) = %q, which holds a line break", entry, encoded)
		}
		decoded, ok := decodeHistoryLine(encoded)
		if entry == "" {
			if ok {
				t.Fatalf("an empty entry decoded as an entry")
			}
			return
		}
		if !ok {
			t.Fatalf("encodeHistoryLine(%q) = %q did not decode as an entry", entry, encoded)
		}
		if decoded != entry {
			t.Fatalf("round trip: %q -> %q -> %q", entry, encoded, decoded)
		}
	})
}

// TestDisabledHistoryStaysEmpty pins what Enabled: false means. SetHistory fell
// into the branch meant for a prompt with no manager at all, so the entries
// landed in the list the arrow keys walk while GetHistory -- which asks the
// manager -- reported nothing. The two answers cannot both be right.
func TestDisabledHistoryStaysEmpty(t *testing.T) {
	t.Parallel()

	newDisabled := func(t *testing.T, script string) *Prompt {
		t.Helper()
		p, err := newFromConfigOn(Config{
			Prefix:        "$ ",
			HistoryConfig: &HistoryConfig{Enabled: false, MaxEntries: 10},
			ColorScheme:   ThemeDefault,
			KeyMap:        NewDefaultKeyMap(),
		}, newMockTerminal(script), io.Discard)
		if err != nil {
			t.Fatalf("newFromConfigOn() error = %v", err)
		}
		return p
	}

	t.Run("SetHistory sets nothing", func(t *testing.T) {
		t.Parallel()

		p := newDisabled(t, "")
		p.SetHistory([]string{"set while disabled"})
		if got := p.GetHistory(); len(got) != 0 {
			t.Errorf("GetHistory() = %q, want nothing", got)
		}
		if len(p.history) != 0 {
			t.Errorf("the history the arrow keys walk holds %q, want nothing", p.history)
		}
	})

	t.Run("Up brings nothing back", func(t *testing.T) {
		t.Parallel()

		p := newDisabled(t, "\x1b[A\x1b[A\r")
		p.SetHistory([]string{"set while disabled"})
		got, err := p.Run()
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got != "" {
			t.Errorf("Run() = %q, want an empty line: there is no history to walk", got)
		}
	})

	t.Run("AddHistory adds nothing", func(t *testing.T) {
		t.Parallel()

		p := newDisabled(t, "")
		p.AddHistory("added while disabled")
		if got := p.GetHistory(); len(got) != 0 {
			t.Errorf("GetHistory() = %q, want nothing", got)
		}
		if len(p.history) != 0 {
			t.Errorf("the history the arrow keys walk holds %q, want nothing", p.history)
		}
	})
}

// TestReverseSearchFitsTheTerminal compares the block Ctrl+R draws with the room
// the terminal has for it. The block is erased by a cursor move back up its own
// height from the row below it, so it has to leave that row on screen: the room
// is a row less than the terminal's height. A block that takes more never gets
// its first row back, so every redraw starts a row lower than the last and
// pushes that many rows of the session off the top, with the header -- which
// names the entry Enter would take -- gone first.
//
// The block is a header and up to five matches, each as long as the entry it
// names, and an entry has no bound: a pasted statement is one line of history.
// A split pane, or a history of long statements on a terminal of the usual size,
// is enough to overflow it.
func TestReverseSearchFitsTheTerminal(t *testing.T) {
	t.Parallel()

	long := func(prefix string, cells int) string {
		return prefix + strings.Repeat("x", cells-len(prefix))
	}

	tests := []struct {
		name    string
		width   int
		height  int
		query   string
		results []string
	}{
		{
			name:    "five statements of the length a pasted query reaches",
			width:   80,
			height:  10,
			query:   "select",
			results: []string{long("select a", 100), long("select b", 100), long("select c", 100), long("select d", 100), long("select e", 100)},
		},
		{
			name:    "long statements on a terminal of the usual size",
			width:   80,
			height:  24,
			query:   "select",
			results: []string{long("select a", 320), long("select b", 320), long("select c", 320), long("select d", 320), long("select e", 320)},
		},
		{
			name:    "a split pane",
			width:   40,
			height:  4,
			query:   "s",
			results: []string{"select 1", "select 2", "select 3", "select 4", "select 5"},
		},
		{
			name:    "a header that fills the terminal on its own",
			width:   20,
			height:  3,
			query:   "s",
			results: []string{long("select a", 200)},
		},
		{
			name:    "a terminal with one row under the cursor's",
			width:   40,
			height:  2,
			query:   "s",
			results: []string{"select 1", "select 2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			p := newTestPromptOn(&sizedMockTerminal{width: tt.width, height: tt.height})
			p.output = &out

			rows, err := p.renderHistorySearch(tt.query, tt.results, 0)
			if err != nil {
				t.Fatalf("renderHistorySearch() error = %v", err)
			}
			// One row below the block is the cursor's, so the block itself has
			// the terminal's height less one.
			room := tt.height - 1
			if rows > room {
				t.Errorf("renderHistorySearch() reported %d rows where there is room for %d on a terminal of %d: the erase never reaches the block's first row again", rows, room, tt.height)
			}

			screen := newScreenModel(tt.width)
			screen.feed(out.String())
			if drawn := len(screen.rows()); drawn > room {
				t.Errorf("renderHistorySearch() drew %d rows where there is room for %d on a terminal of %d", drawn, room, tt.height)
			}
			if drawn := len(screen.rows()); drawn != rows {
				t.Errorf("renderHistorySearch() drew %d rows and reported %d: the erase covers what was reported", drawn, rows)
			}
		})
	}
}

// TestTruncateToRows covers the cut the search header is made with. It walks the
// way layout measures rather than by counting runes or bytes, because the answer
// has to be in the same terms as the height it is cut to: a wide glyph is two
// cells and moves whole to the next row, a tab goes to the next tab stop and no
// further than the last column, and a combining mark occupies nothing.
func TestTruncateToRows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		text  string
		width int
		rows  int
		want  string
	}{
		{name: "no room at all", text: "select", width: 10, rows: 0, want: ""},
		{name: "shorter than the row", text: "select", width: 10, rows: 1, want: "select"},
		{name: "exactly the row", text: "0123456789", width: 10, rows: 1, want: "0123456789"},
		{name: "one cell over", text: "0123456789a", width: 10, rows: 1, want: "0123456789"},
		{name: "two rows of three", text: "0123456789abcdefghij0", width: 10, rows: 2, want: "0123456789abcdefghij"},
		{name: "a wide glyph that does not fit the rest of the row", text: "0123456789あ", width: 11, rows: 1, want: "0123456789"},
		{name: "a wide glyph counted as two cells", text: "あいうえお", width: 6, rows: 1, want: "あいう"},
		{name: "a combining mark occupies nothing", text: "012345678e\u0301x", width: 10, rows: 1, want: "012345678e\u0301"},
		{name: "a tab reaches the next stop", text: "ab\tcdefgh", width: 10, rows: 1, want: "ab\tcd"},
		{name: "a tab on a row with room left stops at the last column", text: "012345678\tx", width: 10, rows: 1, want: "012345678\tx"},
		{name: "a tab on a full row wraps first", text: "0123456789\tx", width: 10, rows: 1, want: "0123456789"},
		{name: "a rune wider than the row stays", text: "あ", width: 1, rows: 1, want: "あ"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := truncateToRows(tt.text, tt.width, tt.rows)
			if got != tt.want {
				t.Errorf("truncateToRows(%q, %d, %d) = %q, want %q", tt.text, tt.width, tt.rows, got, tt.want)
			}
			if rows := rowsOf(got, tt.width); tt.rows > 0 && rows > tt.rows {
				t.Errorf("truncateToRows(%q, %d, %d) = %q, which is %d rows", tt.text, tt.width, tt.rows, got, rows)
			}
		})
	}
}

// TestSavingDoesNotDeleteWhatTheFileHeldWithoutABackup writes a history file
// with more entries than MaxEntries, loads it, and saves, which is what an
// application does by starting up and quitting without typing anything.
//
// MaxEntries is documented as a limit on what is kept in memory, and the load
// applies it before any save is considered -- so the entries past it are not in
// the list the save writes, and the rotation that keeps what a save drops is
// asked only whether MaxFileSize had to cut. It had not: the file here is a few
// kilobytes against a megabyte limit. Every entry the file held has to still be
// on disk somewhere afterwards.
func TestSavingDoesNotDeleteWhatTheFileHeldWithoutABackup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "history")

	const entries = 500
	var written strings.Builder
	for i := range entries {
		fmt.Fprintf(&written, "command %d\n", i)
	}
	if err := os.WriteFile(file, []byte(written.String()), 0o600); err != nil {
		t.Fatalf("writing the history file: %v", err)
	}

	hm := newHistoryManager(&HistoryConfig{
		Enabled: true, MaxEntries: 100, File: file, MaxFileSize: 1024 * 1024, MaxBackups: 3,
	})
	if err := hm.loadHistory(); err != nil {
		t.Fatalf("loadHistory() error = %v", err)
	}
	if err := hm.saveHistory(); err != nil {
		t.Fatalf("saveHistory() error = %v", err)
	}

	onDisk := map[string]bool{}
	names, err := filepath.Glob(file + "*")
	if err != nil {
		t.Fatalf("looking for the history file and its backups: %v", err)
	}
	for _, name := range names {
		content, err := os.ReadFile(name) //nolint:gosec // a file this test just wrote
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for _, line := range strings.Split(strings.TrimSuffix(string(content), "\n"), "\n") {
			onDisk[line] = true
		}
	}

	var lost []string
	for i := range entries {
		entry := fmt.Sprintf("command %d", i)
		if !onDisk[entry] {
			lost = append(lost, entry)
		}
	}
	if len(lost) > 0 {
		t.Errorf("%d of %d entries are on no file after one load and one save, the oldest being %q; the files are %v",
			len(lost), entries, lost[0], names)
	}
}

// TestNormalizeHistoryConfigKeepsAValueTheCallerMeant covers the one field
// whose zero is a setting rather than an omission. Zero entries and a zero-byte
// file are not settings anyone wants, so zero there reads as "unset"; zero
// backups is what a caller says when they do not want copies of what the user
// typed left beside the history file.
//
// A caller who passes no HistoryConfig at all is a different case and takes
// defaultHistoryConfig, backups included: this is about a literal somebody
// wrote, where a field left out was left out on purpose.
func TestNormalizeHistoryConfigKeepsAValueTheCallerMeant(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in   HistoryConfig
		want HistoryConfig
	}{
		"the counts whose zero is not a setting take their defaults": {
			in:   HistoryConfig{Enabled: true},
			want: HistoryConfig{Enabled: true, MaxEntries: 1000, MaxFileSize: 1024 * 1024, MaxBackups: 0},
		},
		"no backups is kept": {
			in:   HistoryConfig{Enabled: true, MaxEntries: 10, MaxFileSize: 20, MaxBackups: 0},
			want: HistoryConfig{Enabled: true, MaxEntries: 10, MaxFileSize: 20, MaxBackups: 0},
		},
		"a negative count is not a setting": {
			in:   HistoryConfig{Enabled: true, MaxEntries: -1, MaxFileSize: -1, MaxBackups: -1},
			want: HistoryConfig{Enabled: true, MaxEntries: 1000, MaxFileSize: 1024 * 1024, MaxBackups: 3},
		},
		"values the caller set are left alone": {
			in:   HistoryConfig{Enabled: true, MaxEntries: 7, MaxFileSize: 11, MaxBackups: 2},
			want: HistoryConfig{Enabled: true, MaxEntries: 7, MaxFileSize: 11, MaxBackups: 2},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := tt.in
			normalizeHistoryConfig(&got)
			if got != tt.want {
				t.Errorf("normalizeHistoryConfig(%+v) left %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

// TestReverseSearchThatMatchedNothingLeavesTheLineAlone presses Enter on a
// search showing no matches. The block names the entry Enter would take, and
// with no matches it names none, so Enter has nothing to take: it must leave
// the line as the search found it rather than handing back the query, which the
// user typed into a search and not onto the command line.
func TestReverseSearchThatMatchedNothingLeavesTheLineAlone(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		history []string
		// script is Ctrl+R, the query, Enter to accept, Enter to submit.
		script string
		want   string
	}{
		"a query nothing matches": {
			history: []string{"select * from users"},
			script:  "\x12zzz\r\r",
			want:    "",
		},
		"an empty history matches nothing at all": {
			history: nil,
			script:  "\x12zzz\r\r",
			want:    "",
		},
		"a query that does match is still accepted": {
			history: []string{"select * from users"},
			script:  "\x12sel\r\r",
			want:    "select * from users",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mock := newMockTerminal(tt.script)
			p := newTestPrompt(mock, WithMemoryHistory(10))
			var out bytes.Buffer
			p.output = &out
			p.renderer = newRenderer(&out, ThemeDefault, mock)
			p.SetHistory(tt.history)

			got, err := p.RunWithContext(context.Background())
			if err != nil {
				t.Fatalf("RunWithContext() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("the search left %q on the line, want %q", got, tt.want)
			}
		})
	}
}

// TestReverseSearchFitsUnderTheEntryItSearchesFrom is the other half of
// TestReverseSearchFitsTheTerminal. That one measures the block against the
// terminal; this one measures it against the room the terminal has left, which
// is what a multiline entry above the cursor takes away.
//
// The block is drawn from the row the caret was left on rather than from the top
// of the screen, so a block that fits the terminal can still need rows below the
// caret that the terminal does not have. What scrolls off the top then is the
// entry the search is looking for a replacement for, and the session's output
// above it.
func TestReverseSearchFitsUnderTheEntryItSearchesFrom(t *testing.T) {
	t.Parallel()

	const width, height = 20, 8
	results := []string{"select 1", "select 2", "select 3", "select 4", "select 5"}

	tests := map[string]struct {
		entry string
		rows  int // the rows the entry occupies
	}{
		"an entry of one line":   {entry: "one", rows: 1},
		"an entry of five lines": {entry: "one\ntwo\nthree\nfour\nfive", rows: 5},
		"an entry that fills it": {entry: "one\ntwo\nthree\nfour\nfive\nsix\nseven", rows: 7},
		"an entry that wraps":    {entry: strings.Repeat("x", width*3), rows: 3},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			terminal := &sizedMockTerminal{width: width, height: height}
			p := newTestPromptOn(terminal, WithMultiline(true))
			p.output = &out
			p.renderer = newRenderer(&out, ThemeDefault, terminal)
			p.buffer = []rune(tt.entry)
			p.cursor = len(p.buffer)

			if err := p.render(); err != nil {
				t.Fatalf("render: %v", err)
			}
			screen := newBoundedScreenModel(width, height)
			screen.feed(out.String())
			if screen.scrolled != 0 {
				t.Fatalf("the entry alone scrolled the screen by %d rows", screen.scrolled)
			}
			caret := screen.row

			out.Reset()
			rows, err := p.renderHistorySearch("select", results, 0)
			if err != nil {
				t.Fatalf("renderHistorySearch: %v", err)
			}
			screen.feed(out.String())

			if screen.scrolled != 0 {
				t.Errorf("the search block scrolled the screen by %d rows: the entry it is searching from goes first\n%q",
					screen.scrolled, screen.rows())
			}
			// The block starts on the caret's row and ends one row below its
			// last, which is the row the erase moves up from.
			if room := height - caret - 1; rows > room {
				t.Errorf("the search drew %d rows with room for %d under a caret on row %d of a %d-row terminal",
					rows, room, caret, height)
			}
			// The header is worth drawing however little room there is: a search
			// that shows nothing cannot be steered.
			if rows < 1 {
				t.Errorf("the search drew nothing at all")
			}
		})
	}
}
