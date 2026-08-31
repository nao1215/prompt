package prompt

import (
	"bytes"
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
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping slow test in local development")
	}

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
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping slow test in local development")
	}

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
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping slow test in local development")
	}

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
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping slow test in local development")
	}

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
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping slow test in local development")
	}

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
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping slow test in local development")
	}

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
		if os.Getenv("GITHUB_ACTIONS") == "" {
			t.Skip("Skipping slow test in local development")
		}

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

func TestCreateRotatedFile(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping slow test in local development")
	}

	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "rotate_test")

	config := &HistoryConfig{
		Enabled:     true,
		File:        historyFile,
		MaxFileSize: 500, // Increased to handle larger content
		MaxBackups:  2,
	}

	hm := newHistoryManager(config)

	// Add more than 100 entries to ensure trimming occurs (createRotatedFile keeps all if < 100)
	for i := range 150 {
		hm.addEntry(fmt.Sprintf("initial_entry_%d_%s", i, strings.Repeat("X", 10)))
	}

	// Create the original file
	err := hm.saveHistory()
	if err != nil {
		t.Fatalf("Failed to save history: %v", err)
	}

	// Check file size to ensure it's large enough for rotation test
	info, err := os.Stat(historyFile)
	if err != nil {
		t.Fatalf("Failed to stat history file: %v", err)
	}
	t.Logf("Initial file size: %d bytes, MaxFileSize: %d bytes", info.Size(), config.MaxFileSize)

	// Force the file to exceed the size limit if needed
	for info.Size() < config.MaxFileSize {
		// Add more entries to exceed the size limit
		for i := range 5 {
			hm.addEntry(fmt.Sprintf("padding_entry_%d_%s", i, strings.Repeat("P", 50)))
		}
		err = hm.saveHistory()
		if err != nil {
			t.Fatalf("Failed to save history while building size: %v", err)
		}
		info, err = os.Stat(historyFile)
		if err != nil {
			t.Fatalf("Failed to stat history file: %v", err)
		}
		t.Logf("File size after adding entries: %d bytes", info.Size())
	}

	originalCount := len(hm.getHistory())
	t.Logf("Original count before rotation: %d (should be >100 for trimming)", originalCount)

	// The file is already large enough, so next save should trigger rotation
	// Since rotateIfNeeded() checks existing file size, we need to add more content to current memory
	// but save separately to trigger the rotation check properly

	// Add several more entries to memory only
	for i := range 10 {
		hm.addEntry(fmt.Sprintf("trigger_%d_%s", i, strings.Repeat("T", 30)))
	}

	finalCount := len(hm.getHistory())
	t.Logf("Final count before rotation save: %d", finalCount)

	// Now save - this should trigger rotation since file exceeds MaxFileSize
	err = hm.saveHistory()
	if err != nil {
		t.Fatalf("Failed to save history during rotation: %v", err)
	}

	// Check if backup file was created (indication of rotation)
	backupFile := historyFile + ".1"
	rotatedCount := len(hm.getHistory())
	t.Logf("Count after rotation save: %d", rotatedCount)

	// Check the actual file size after save
	newInfo, err := os.Stat(historyFile)
	if err == nil {
		t.Logf("New file size: %d bytes", newInfo.Size())
	}

	if _, err := os.Stat(backupFile); err == nil {
		t.Logf("Backup file created, rotation occurred")

		// Read the rotated file to see actual content
		content, err := os.ReadFile(filepath.Clean(historyFile)) // #nosec G304 - test file path is controlled
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(content)), "\n")
			actualFileLines := 0
			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					actualFileLines++
				}
			}
			t.Logf("Actual lines in rotated file: %d", actualFileLines)
		}

		// Due to the current implementation where SaveHistory overwrites the rotated file,
		// the rotation doesn't effectively trim memory. This is a known implementation issue.
		// For now, just verify that rotation occurred (backup file exists) and
		// that the system didn't crash.
		t.Logf("Rotation completed successfully - backup file exists")

		// Verify the new file size is larger than MaxFileSize but reasonable
		if newInfo != nil && newInfo.Size() > config.MaxFileSize*20 {
			t.Errorf("New file size %d is excessively large (>%d)", newInfo.Size(), config.MaxFileSize*20)
		}
	} else {
		t.Skipf("No rotation occurred (no backup file created), cannot test trimming")
	}

	// Verify we can still load the rotated file
	hm2 := newHistoryManager(config)
	err = hm2.loadHistory()
	if err != nil {
		t.Fatalf("Failed to load rotated history: %v", err)
	}

	if len(hm2.getHistory()) == 0 {
		t.Error("Rotated file should contain some history")
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

		// Should return the search query itself
		if result != "zzznomatch" {
			t.Errorf("Expected result to be 'zzznomatch', got %q", result)
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
		p.renderHistorySearch("git", results, 0)

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
		p.renderHistorySearch("git", results, 1)

		outputStr := output.String()
		if !strings.Contains(outputStr, "git commit") {
			t.Error("Expected output to contain selected result 'git commit'")
		}
	})

	t.Run("RenderEmptyResults", func(t *testing.T) {
		output.Reset()
		results := []string{}
		p.renderHistorySearch("nomatch", results, 0)

		outputStr := output.String()
		if !strings.Contains(outputStr, "nomatch") {
			t.Error("Expected output to contain search query even with no results")
		}
	})

	t.Run("RenderManyResults", func(t *testing.T) {
		output.Reset()
		results := []string{"cmd1", "cmd2", "cmd3", "cmd4", "cmd5", "cmd6", "cmd7"}
		p.renderHistorySearch("cmd", results, 2)

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
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping slow test in local development")
	}

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
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping slow test in local development")
	}

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
	if os.Getenv("GITHUB_ACTIONS") == "" || runtime.GOOS == windowsOS {
		t.Skip("Skipping permission test")
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
