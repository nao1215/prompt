package prompt

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultHistoryConfig returns a default history configuration following XDG Base Directory Specification
func DefaultHistoryConfig() *HistoryConfig {
	return &HistoryConfig{
		Enabled:     true,
		MaxEntries:  defaultMaxHistoryEntries,
		File:        "",          // Empty by default, can be set to use XDG config directory
		MaxFileSize: 1024 * 1024, // 1MB
		MaxBackups:  3,
	}
}

// GetDefaultHistoryFile returns the default history file path following XDG Base Directory Specification.
// Returns ~/.config/prompt/history or $XDG_CONFIG_HOME/prompt/history if XDG_CONFIG_HOME is set.
func GetDefaultHistoryFile() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(homeDir, ".config")
	}
	return filepath.Join(configDir, "prompt", "history")
}

// defaultMaxHistoryEntries is how many entries are kept when the configuration
// names no limit.
const defaultMaxHistoryEntries = 1000

// HistoryManager manages command history persistence and rotation
type HistoryManager struct {
	config  *HistoryConfig
	history []string
}

// NewHistoryManager creates a new history manager with the given configuration
func NewHistoryManager(config *HistoryConfig) *HistoryManager {
	if config == nil {
		config = DefaultHistoryConfig()
	}
	if config.MaxFileSize <= 0 {
		config.MaxFileSize = 1024 * 1024 // 1MB default
	}
	if config.MaxBackups < 0 {
		config.MaxBackups = 3
	}

	// Expand and convert file path to absolute path if specified
	if config.File != "" {
		if absPath, err := expandHistoryPath(config.File); err == nil {
			config.File = absPath
		}
	}

	return &HistoryManager{
		config:  config,
		history: make([]string, 0),
	}
}

// IsEnabled returns whether history functionality is enabled
func (hm *HistoryManager) IsEnabled() bool {
	return hm.config.Enabled
}

// LoadHistory loads history from the configured file
func (hm *HistoryManager) LoadHistory() error {
	if !hm.config.Enabled || hm.config.File == "" {
		return nil
	}

	file, err := os.Open(hm.config.File)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's ok
		}
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	// The file's contents replace what the manager holds rather than joining it.
	// A load answers "what is in the file", and appending made asking twice --
	// after another shell wrote to it, after the user edited it -- return every
	// entry twice. It is built up separately so a read that fails partway leaves
	// the existing history alone.
	loaded := make([]string, 0, len(hm.history))
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Only the line terminator is dropped: an entry's own leading and
		// trailing whitespace is part of the command the user submitted.
		entry, ok := decodeHistoryLine(strings.TrimSuffix(scanner.Text(), "\r"))
		if !ok {
			continue
		}
		loaded = append(loaded, entry)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read history file: %w", err)
	}

	hm.history = hm.trim(loaded)
	return nil
}

// SaveHistory saves the current history to the configured file
func (hm *HistoryManager) SaveHistory() (err error) {
	if !hm.config.Enabled || hm.config.File == "" {
		return nil
	}

	// Check if rotation is needed
	if err := hm.rotateIfNeeded(); err != nil {
		return fmt.Errorf("failed to rotate history file: %w", err)
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(hm.config.File)
	if dir != "." {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("failed to create history directory: %w", err)
		}
	}

	file, err := os.Create(hm.config.File)
	if err != nil {
		return fmt.Errorf("failed to create history file: %w", err)
	}
	// A buffered write can fail at close, and a history file that lost its tail
	// silently is what this reports.
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close history file: %w", cerr)
		}
	}()

	for _, entry := range hm.history {
		if _, err := fmt.Fprintln(file, encodeHistoryLine(entry)); err != nil {
			return fmt.Errorf("failed to write history entry: %w", err)
		}
	}

	return nil
}

// encodeHistoryLine renders one entry as a single physical line.
//
// The file is read back one entry per line, so a command typed across several
// lines has to survive as one. Backslash escaping is the whole rule: all other
// bytes are written as they are, so an ordinary command stays readable in a
// text editor, which a quoted or length-prefixed format would have cost.
//
// Bytes, not runes: ranging over runes would replace an invalid UTF-8 byte with
// U+FFFD, which is a byte of the entry lost to the encoding meant to preserve it.
func encodeHistoryLine(entry string) string {
	var b strings.Builder
	b.Grow(len(entry))
	for i := range len(entry) {
		switch entry[i] {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(entry[i])
		}
	}
	return b.String()
}

// decodeHistoryLine reads back what encodeHistoryLine wrote and reports whether
// the line held an entry. Only an empty line holds none: a line of spaces is how
// an indented command is spelled on disk.
func decodeHistoryLine(line string) (string, bool) {
	if line == "" {
		return "", false
	}

	var b strings.Builder
	b.Grow(len(line))
	escaped := false
	for i := range len(line) {
		c := line[i]
		if escaped {
			switch c {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case '\\':
				b.WriteByte('\\')
			default:
				// An undefined escape is written back as it was read, so a file
				// written before this encoding, or edited by hand, loses no
				// characters to a rule it never followed.
				b.WriteByte('\\')
				b.WriteByte(c)
			}
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		b.WriteByte(c)
	}
	// A trailing backslash has nothing left to escape; keeping it means the
	// entry loses no character.
	if escaped {
		b.WriteByte('\\')
	}
	return b.String(), true
}

// AddEntry adds a new entry to the history
func (hm *HistoryManager) AddEntry(entry string) {
	if !hm.config.Enabled || entry == "" {
		return
	}

	// Avoid duplicate consecutive entries
	if len(hm.history) > 0 && hm.history[len(hm.history)-1] == entry {
		return
	}

	hm.history = hm.trim(append(hm.history, entry))
}

// trim drops the oldest entries until at most MaxEntries remain.
//
// The limit belongs here, where the history is held. It used to be applied only
// by the prompt, which read this history back, cut it, and pushed the shortened
// copy down again, so a manager used on its own grew for as long as the process
// ran however small a limit it was given.
func (hm *HistoryManager) trim(history []string) []string {
	limit := hm.config.MaxEntries
	if limit <= 0 {
		limit = defaultMaxHistoryEntries
	}
	if len(history) <= limit {
		return history
	}
	return history[len(history)-limit:]
}

// GetHistory returns a copy of the current history
func (hm *HistoryManager) GetHistory() []string {
	if !hm.config.Enabled {
		return []string{}
	}
	return append([]string{}, hm.history...)
}

// SetHistory replaces the current history
func (hm *HistoryManager) SetHistory(history []string) {
	if !hm.config.Enabled {
		return
	}
	hm.history = hm.trim(append([]string{}, history...))
}

// ClearHistory clears the current history
func (hm *HistoryManager) ClearHistory() {
	if !hm.config.Enabled {
		return
	}
	hm.history = []string{}
}

// rotateIfNeeded checks if the history file needs rotation and performs it
func (hm *HistoryManager) rotateIfNeeded() error {
	if hm.config.File == "" {
		return nil
	}

	info, err := os.Stat(hm.config.File)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, no rotation needed
		}
		return err
	}

	if info.Size() < hm.config.MaxFileSize {
		return nil // File is small enough, no rotation needed
	}

	// Perform rotation
	return hm.rotateHistoryFile()
}

// rotateHistoryFile performs the actual file rotation
func (hm *HistoryManager) rotateHistoryFile() error {
	if hm.config.MaxBackups <= 0 {
		// If no backups allowed, just truncate the file
		return os.Truncate(hm.config.File, 0)
	}

	// Remove the oldest backup if it exists
	oldestBackup := hm.config.File + "." + strconv.Itoa(hm.config.MaxBackups)
	if _, err := os.Stat(oldestBackup); err == nil {
		if err := os.Remove(oldestBackup); err != nil {
			return fmt.Errorf("failed to remove oldest backup: %w", err)
		}
	}

	// Shift existing backups
	for i := hm.config.MaxBackups - 1; i >= 1; i-- {
		oldFile := hm.config.File + "." + strconv.Itoa(i)
		newFile := hm.config.File + "." + strconv.Itoa(i+1)

		if _, err := os.Stat(oldFile); err == nil {
			if err := os.Rename(oldFile, newFile); err != nil {
				return fmt.Errorf("failed to rotate backup %d: %w", i, err)
			}
		}
	}

	// Move current file to .1
	backup := hm.config.File + ".1"
	if err := os.Rename(hm.config.File, backup); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Keep only the most recent entries in the new file
	if err := hm.createRotatedFile(); err != nil {
		return fmt.Errorf("failed to create rotated file: %w", err)
	}

	return nil
}

// createRotatedFile creates a new history file with the most recent entries
func (hm *HistoryManager) createRotatedFile() (err error) {
	// Keep only half of the history entries to avoid immediate rotation
	keepEntries := len(hm.history) / 2
	if keepEntries < 100 {
		keepEntries = len(hm.history) // Keep all if less than 100 entries
	}

	startIndex := len(hm.history) - keepEntries
	if startIndex < 0 {
		startIndex = 0
	}

	file, err := os.Create(hm.config.File)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close history file: %w", cerr)
		}
	}()

	for i := startIndex; i < len(hm.history); i++ {
		if _, err := fmt.Fprintln(file, encodeHistoryLine(hm.history[i])); err != nil {
			return err
		}
	}

	// Update in-memory history to match the rotated file
	hm.history = hm.history[startIndex:]

	return nil
}

// expandHistoryPath expands and validates the history file path
// Supports:
// - Absolute paths: /home/user/.history
// - Home directory expansion: ~/.history or ~/config/.history
// - Relative paths: ./.history or config/.history (converted to absolute)
func expandHistoryPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	// Expand home directory (~)
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	} else if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		path = home
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to convert to absolute path: %w", err)
	}

	return absPath, nil
}
