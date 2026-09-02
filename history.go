package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

// defaultHistoryConfig returns a default history configuration following XDG Base Directory Specification
func defaultHistoryConfig() *HistoryConfig {
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
// sets no limit.
const defaultMaxHistoryEntries = 1000

// defaultMaxHistoryFileSize is how large the history file may grow before it is
// rotated, and defaultHistoryBackups how many generations are kept.
const (
	defaultMaxHistoryFileSize = 1024 * 1024
	defaultHistoryBackups     = 3
)

// normalizeHistoryConfig fills in the defaults for the fields a caller left at
// their zero value.
func normalizeHistoryConfig(config *HistoryConfig) {
	if config.MaxEntries <= 0 {
		config.MaxEntries = defaultMaxHistoryEntries
	}
	if config.MaxFileSize <= 0 {
		config.MaxFileSize = defaultMaxHistoryFileSize
	}
	// Only a negative count is an omission. Zero backups is what a caller says
	// when they do not want copies of what the user typed left beside the
	// history file, and rotateHistoryFile implements it.
	if config.MaxBackups < 0 {
		config.MaxBackups = defaultHistoryBackups
	}
}

// historyManager manages command history persistence and rotation
type historyManager struct {
	config  *HistoryConfig
	history []string
	// overflowed records whether the last save had to leave entries out of the
	// file, so that the rotation which keeps them happens once rather than on
	// every save afterwards.
	overflowed bool
}

// newHistoryManager creates a new history manager with the given configuration
func newHistoryManager(config *HistoryConfig) *historyManager {
	if config == nil {
		config = defaultHistoryConfig()
	}
	normalizeHistoryConfig(config)

	// Expand and convert file path to absolute path if specified
	if config.File != "" {
		if absPath, err := expandHistoryPath(config.File); err == nil {
			config.File = absPath
		}
	}

	return &historyManager{
		config:  config,
		history: make([]string, 0),
	}
}

// IsEnabled returns whether history functionality is enabled
func (hm *historyManager) isEnabled() bool {
	return hm.config.Enabled
}

// LoadHistory loads history from the configured file
func (hm *historyManager) loadHistory() error {
	if !hm.config.Enabled || hm.config.File == "" {
		return nil
	}

	// The file's contents replace what the manager holds rather than being added
	// to it. A load answers "what is in the file", and because it appended,
	// asking a second time -- after another shell wrote to the file, after the
	// user edited it -- returned every entry twice. A read that fails partway
	// leaves the existing history alone, because readHistoryFile collects the
	// entries before any of them are handed over.
	loaded, err := readHistoryFile(hm.config.File)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // File doesn't exist yet, that's ok
		}
		return err
	}

	hm.history = hm.trim(loaded)
	return nil
}

// readHistoryFile returns the entries the file at path holds, in the order they
// were written.
//
// It reads with a bufio.Reader rather than a bufio.Scanner, which refuses a line
// over 64KB. An entry has no length limit -- a paste is content, and what the
// user submits is whatever they typed -- so the writer could produce a file the
// reader rejected. The load then failed, took the whole history with it, and New
// returns that error: one long paste left the application unable to start until
// the file was deleted by hand.
func readHistoryFile(path string) ([]string, error) {
	file, err := os.Open(path) //nolint:gosec // the path is the caller's own configuration
	if err != nil {
		return nil, fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	entries := make([]string, 0, 64)
	reader := bufio.NewReader(file)
	for {
		line, readErr := reader.ReadString('\n')
		if line != "" || readErr == nil {
			// Only the line terminator is dropped: an entry's own leading and
			// trailing whitespace is part of the command the user submitted. A
			// last line without a newline is an entry too.
			line = strings.TrimSuffix(line, "\n")
			if entry, ok := decodeHistoryLine(strings.TrimSuffix(line, "\r")); ok {
				entries = append(entries, entry)
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return nil, fmt.Errorf("failed to read history file: %w", readErr)
			}
			return entries, nil
		}
	}
}

// entriesOnDisk reports how many entries the history file holds now, which is
// what a save is about to write over. A file that is not there holds none.
func (hm *historyManager) entriesOnDisk() (int, error) {
	entries, err := readHistoryFile(hm.config.File)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	return len(entries), nil
}

// SaveHistory saves the current history to the configured file
func (hm *historyManager) saveHistory() (err error) {
	if !hm.config.Enabled || hm.config.File == "" {
		return nil
	}

	// What fits, and whether this save loses anything.
	//
	// A save writes over the file, so every entry the file holds and this save
	// does not write is destroyed. There are two ways for that to happen and
	// only one of them used to be asked about. MaxFileSize cuts what is written,
	// which is the overflow below; MaxEntries cuts what is held, and it does it
	// at load, before any save is considered -- so the entries it dropped are
	// not in hm.history and cannot make the overflow true. A history file with
	// more entries than MaxEntries was therefore cut down to MaxEntries on the
	// next save with no backup taken, however much room MaxFileSize had left.
	// Counting what is on disk asks the one question that covers both: is this
	// save writing fewer entries than the file already holds?
	writable := hm.writable()
	held, err := hm.entriesOnDisk()
	if err != nil {
		return fmt.Errorf("failed to read the history file before writing it: %w", err)
	}
	dropping := len(writable) < len(hm.history) || held > len(writable)
	if err := hm.rotateIfNeeded(dropping); err != nil {
		return fmt.Errorf("failed to rotate history file: %w", err)
	}
	hm.overflowed = dropping

	// Create directory if it doesn't exist
	dir := filepath.Dir(hm.config.File)
	if dir != "." {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("failed to create history directory: %w", err)
		}
	}

	// The file holds what the user typed, which is where a password given on a
	// command line ends up. It is created readable by its owner alone, the way a
	// shell creates its own. os.Create would ask for 0666 and let the umask
	// decide, which on a common default leaves the file world-readable. A file
	// that already exists keeps whatever mode it was given.
	file, err := os.OpenFile(hm.config.File, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, historyFileMode)
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

	for _, entry := range writable {
		if _, err := fmt.Fprintln(file, encodeHistoryLine(entry)); err != nil {
			return fmt.Errorf("failed to write history entry: %w", err)
		}
	}

	return nil
}

// historyFileMode is what a history file is created with: readable and writable
// by its owner and nobody else.
const historyFileMode = 0o600

// writable returns the newest entries whose encoded lines fit in MaxFileSize.
//
// The limit has to be applied to what is written, not only to when the file is
// rotated. Rotation used to write a file the size of the one it had just moved
// aside, so the file was over the limit the moment it appeared and the next save
// rotated again: within MaxBackups saves every backup held a near-identical copy
// of the newest history and the oldest entries, which rotation exists to keep,
// had been deleted.
//
// What does not fit stays in the backup. The history held in memory is not
// touched, because how much is remembered for the session is MaxEntries'
// business and saving should not change it.
func (hm *historyManager) writable() []string {
	limit := hm.config.MaxFileSize
	if limit <= 0 {
		return hm.history
	}
	var size int64
	for i := len(hm.history) - 1; i >= 0; i-- {
		size += int64(len(encodeHistoryLine(hm.history[i]))) + 1 // the newline
		// The newest entry is written whatever it costs. A limit small enough to
		// exclude it is a limit that would have the prompt save nothing at all,
		// which is worse than a file slightly over it.
		if size > limit && i < len(hm.history)-1 {
			return hm.history[i+1:]
		}
	}
	return hm.history
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
func (hm *historyManager) addEntry(entry string) {
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
// ran, however small a limit it was given.
func (hm *historyManager) trim(history []string) []string {
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
func (hm *historyManager) getHistory() []string {
	if !hm.config.Enabled {
		return []string{}
	}
	return append([]string{}, hm.history...)
}

// SetHistory replaces the current history
func (hm *historyManager) setHistory(history []string) {
	if !hm.config.Enabled {
		return
	}
	hm.history = hm.trim(append([]string{}, history...))
}

// ClearHistory clears the current history
func (hm *historyManager) clearHistory() {
	if !hm.config.Enabled {
		return
	}
	hm.history = []string{}
}

// rotateIfNeeded checks if the history file needs rotation and performs it
func (hm *historyManager) rotateIfNeeded(overflow bool) error {
	if hm.config.File == "" {
		return nil
	}
	// Rotation happens the first time the history outgrows the file, and keeps
	// the last file that held all of it. Rotating on every save after that would
	// write a near-identical copy each time and delete the oldest backup --
	// which is the only place the entries being left out still exist -- within
	// MaxBackups saves.
	if !overflow || hm.overflowed {
		return nil
	}

	if _, err := os.Stat(hm.config.File); err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing on disk to keep
		}
		return err
	}

	return hm.rotateHistoryFile()
}

// rotateHistoryFile performs the actual file rotation
func (hm *historyManager) rotateHistoryFile() error {
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

	// Move current file to .1. What replaces it is written by saveHistory, which
	// is the only place that decides what the file holds.
	backup := hm.config.File + ".1"
	if err := os.Rename(hm.config.File, backup); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

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

// Reverse search (Ctrl+R) and the glue between the prompt and the history it
// holds. It lives here rather than with the read loop because it belongs to what
// the screen shows while a search is open, not to the reading of keys, and
// because the tests that cover it are history tests.

// newHistorySearcher returns the search function Ctrl+R calls for matches. It
// ranks the history by fuzzy match against the query, closest first.
func newHistorySearcher(history []string) func(string) []string {
	fm := &fuzzyMatcher{
		items: history,
	}
	return fm.searchFunc
}

// searchHistory implements reverse history search (like Ctrl+R in bash)
func (p *Prompt) searchHistory() (_ string, err error) {
	search := newHistorySearcher(p.history)
	searchBuffer := []rune{}
	searchResults := search("")
	selectedIndex := 0

	// The interface is scratch space on the screen. Each render erases the one
	// before it, and the last is erased however the search ends, so the prompt
	// is redrawn on the line the search started on. Appending instead stacked a
	// block per keystroke and left every one of them in the scrollback.
	drawn := 0
	defer func() {
		// A cleanup that fails is worth reporting, but not at the cost of the
		// error that ended the search: that one says why it ended.
		if cerr := p.clearHistorySearch(drawn); cerr != nil && err == nil {
			err = cerr
		}
	}()

	for {
		if err := p.clearHistorySearch(drawn); err != nil {
			return "", err
		}
		rows, err := p.renderHistorySearch(string(searchBuffer), searchResults, selectedIndex)
		drawn = rows
		if err != nil {
			return "", err
		}

		// Read key input
		r, err := p.readRune()
		if err != nil {
			return "", err
		}

		switch r {
		case '\r', '\n': // Enter - accept selection
			if selectedIndex < len(searchResults) {
				return searchResults[selectedIndex], nil
			}
			// Nothing matched, so there is nothing to accept. The block names
			// the entry Enter would take and with no matches it names none;
			// handing back the query would put text the user typed into a
			// search onto the command line, a keystroke away from being run.
			// An empty result is what Escape and Ctrl+C return, and the caller
			// leaves the line as the search found it.
			return "", nil

		case '\x03': // Ctrl+C - cancel search
			return "", nil

		case '\x1b': // Escape, or a key the terminal spells as a sequence
			// The sequence has to be read here for the same reason the main loop
			// reads it: what is not interpreted is still in the input. Ending the
			// search on the ESC alone left the rest of an arrow key to be typed
			// into the line, so pressing Up put "[A" in front of the user.
			seq, serr := p.readEscapeSequence()
			if errors.Is(serr, io.EOF) {
				// Nothing followed the Escape and nothing ever will, so it was
				// the key on its own. The main loop reports the end of input.
				return "", nil
			}
			if serr != nil {
				return "", serr
			}
			switch {
			case seq == "":
				return "", nil // a bare Escape, or Alt+key, cancels
			case p.keyMap.GetSequenceAction(seq) == ActionMoveUp:
				selectedIndex = moveSearchSelection(selectedIndex, -1, len(searchResults))
			case p.keyMap.GetSequenceAction(seq) == ActionMoveDown:
				selectedIndex = moveSearchSelection(selectedIndex, 1, len(searchResults))
			}
			// Any other sequence is consumed and ignored: the search has no use
			// for it, and leaving it unread is how it became text.

		case '\x7f', '\b': // Backspace
			if len(searchBuffer) > 0 {
				searchBuffer = searchBuffer[:len(searchBuffer)-1]
				searchResults = search(string(searchBuffer))
				selectedIndex = 0
			}

		case '\t': // Tab - next result
			selectedIndex = moveSearchSelection(selectedIndex, 1, len(searchResults))

		default:
			if r >= 32 && r < 127 || r > 127 { // Printable characters
				searchBuffer = append(searchBuffer, r)
				searchResults = search(string(searchBuffer))
				selectedIndex = 0
			}
		}
	}
}

// moveSearchSelection moves the reverse-search selection by step through count
// matches, wrapping at either end. With no matches there is nothing to select
// and the index stays where it is.
func moveSearchSelection(selected, step, count int) int {
	if count == 0 {
		return selected
	}
	return ((selected+step)%count + count) % count
}

// renderHistorySearch renders the history search interface and returns how many
// terminal rows it occupies, which is what the next render has to erase.
func (p *Prompt) renderHistorySearch(query string, results []string, selected int) (int, error) {
	lines := p.searchLines(query, results, selected)

	if _, err := fmt.Fprint(p.output, "\r\x1b[K"); err != nil {
		return 0, err
	}
	for _, line := range lines {
		if _, err := fmt.Fprintf(p.output, "%s\r\n", line); err != nil {
			// Some of the block reached the terminal, so report the rows it was
			// meant to occupy: the caller erases what it asked for rather than
			// leaving a half-drawn block behind.
			return p.searchBlockRows(lines), err
		}
	}
	return p.searchBlockRows(lines), nil
}

// searchLines returns the rows of the reverse-search block: a header naming the
// query and the selection, then the matches that fit under it.
//
// The block is redrawn on every keystroke and erased by a cursor move back up
// its own height, so it has to leave the row that move starts from on screen.
// Each line is written with a line break after it, which puts the cursor one row
// below the block, so the room for it is a row less than what the terminal has
// left under the caret -- see searchBudget.
// A block that takes more than that never gets its first row back: every redraw
// starts a row lower than the last and pushes that many rows of the session off
// the top, and the header -- which is what names the entry Enter would take --
// is the first thing gone, so the user is steering a search they cannot see. On
// an 80x10 terminal a twelve-row block costs three rows of the screen per
// keystroke. Five matches of a length nothing bounds -- a pasted statement is
// one line of history -- reach that on a terminal of the usual size, and any
// history at all reaches it in a split pane.
//
// The header is always drawn, however little room there is, because a search
// that shows nothing is a search the user cannot steer; it is cut to the rows
// available. What the room costs is matches, down to none.
func (p *Prompt) searchLines(query string, results []string, selected int) []string {
	width, height := p.searchTerminalSize()
	budget := p.searchBudget(height)

	// A history entry can hold newlines -- a statement entered across several
	// lines is stored as one entry, which is what the file's escaping is for --
	// so every line is flattened to one row. Drawing one raw took rows the block
	// never counted and left them on screen when the search closed.
	header := "reverse-i-search: " + singleLine(query)
	if selected < len(results) && len(results) > 0 {
		header += " -> " + singleLine(results[selected])
	}
	header = truncateToRows(header, width, budget)

	// At most five matches whatever the room. A taller terminal is not a reason
	// to list the whole history: the point of the block is the few best matches
	// and the query being typed, and past five the eye has to search the search.
	// The header names the selection even when Tab has cycled past what is
	// listed, so it is built before this cut.
	const maxResults = 5
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	lines := make([]string, 0, len(results)+1)
	lines = append(lines, header)
	used := rowsOf(header, width)
	for i, result := range results {
		line := "    " + singleLine(result)
		if i == selected {
			line = "  > " + singleLine(result)
		}
		rows := rowsOf(line, width)
		if used+rows > budget {
			break
		}
		lines = append(lines, line)
		used += rows
	}
	return lines
}

// searchBudget returns how many rows the reverse-search block may occupy.
//
// The block is drawn from the row the caret was left on rather than from the top
// of the screen, so what it has is what the terminal has left under that row:
// the height, less the rows of the entry above the caret, less the row the
// block's own trailing line break lands on -- which is the row the erase moves
// up from and therefore has to stay on screen.
//
// Measuring against the height alone was right for a block at the top of the
// screen and wrong under a multiline entry, where the rows above the caret are
// rows nothing counted: a five-row entry on an eight-row terminal pushed itself
// and four rows of the session off the top the moment Ctrl+R was pressed.
//
// At least one row, because the header is what names the entry Enter would take
// and a search that shows nothing cannot be steered. What the room costs is
// matches, down to none.
func (p *Prompt) searchBudget(height int) int {
	caret := 0
	if p.renderer != nil {
		caret = p.renderer.lastCursorRow
	}
	return max(height-1-caret, 1)
}

// searchTerminalSize reports the size the search block is measured against,
// falling back the way the renderer does when there is no terminal or it could
// not say.
func (p *Prompt) searchTerminalSize() (width, height int) {
	width, height = defaultTerminalWidth, fallbackHeight
	if p.terminal == nil {
		return width, height
	}
	w, h, err := p.terminal.Size()
	if err != nil {
		return width, height
	}
	if w > 0 {
		width = w
	}
	if h > 0 {
		height = h
	}
	return width, height
}

// clearHistorySearch erases the rows a previous search render left on screen and
// returns the cursor to the row that render started on. Rendering ends one row
// below the block, because every line it writes ends with a line break, so the
// move up covers the whole block.
func (p *Prompt) clearHistorySearch(rows int) error {
	if rows <= 0 {
		return nil
	}
	_, err := fmt.Fprintf(p.output, "\x1b[%dA\r\x1b[0J", rows)
	return err
}

// searchBlockRows returns how many terminal rows the given lines occupy. A
// history entry can be longer than the terminal is wide, and a wrapped line is
// two rows to erase rather than one.
func (p *Prompt) searchBlockRows(lines []string) int {
	width, _ := p.searchTerminalSize()

	rows := 0
	for _, line := range lines {
		rows += rowsOf(line, width)
	}
	return rows
}

// rowsOf returns how many terminal rows one line occupies at the given width.
func rowsOf(line string, width int) int {
	wrapped, _ := layout(line, width)
	return wrapped + 1
}

// truncateToRows returns the longest prefix of s that a terminal width columns
// wide draws in at most the given number of rows. It steps the way layout
// measures, tab stops included, so the answer is in the same terms as every
// other height here.
func truncateToRows(s string, width, rows int) string {
	if rows <= 0 {
		return ""
	}
	used, col := 0, 0
	for i, r := range s {
		nextUsed, nextCol := used, col
		if r == '\t' {
			if nextCol >= width {
				nextUsed++
				nextCol = 0
			}
			nextCol = min(nextCol+tabWidth-nextCol%tabWidth, width-1)
		} else if w := runewidth.RuneWidth(r); w > 0 {
			if nextCol+w > width && nextCol > 0 {
				nextUsed++
				nextCol = 0
			}
			nextCol += w
		}
		if nextUsed >= rows {
			return s[:i]
		}
		used, col = nextUsed, nextCol
	}
	return s
}

// syncHistoryAfterAdd synchronizes in-memory history with history manager after adding an entry.
func (p *Prompt) syncHistoryAfterAdd() {
	if p.historyManager != nil && p.historyManager.isEnabled() {
		// The manager applies MaxEntries itself now, so there is nothing to cut
		// and push back down: what it holds is what the prompt shows.
		p.history = p.historyManager.getHistory()
	}
}

// getMaxHistoryEntries returns the configured maximum history entries or default
func (p *Prompt) getMaxHistoryEntries() int {
	if p.config.HistoryConfig != nil && p.config.HistoryConfig.MaxEntries > 0 {
		return p.config.HistoryConfig.MaxEntries
	}
	return 1000 // Default max entries
}

// addToHistory adds text to history, handling both historyManager and in-memory fallback
func (p *Prompt) addToHistory(text string) {
	if text == "" {
		return
	}

	if p.historyManager != nil {
		if p.historyManager.isEnabled() {
			p.historyManager.addEntry(text)
			p.syncHistoryAfterAdd()
		}
		// Do nothing when history manager exists but is disabled
		return
	}

	// Fallback to in-memory only (when no history manager)
	if len(p.history) > 0 && p.history[len(p.history)-1] == text {
		return // Avoid duplicate consecutive entries
	}
	p.history = append(p.history, text)

	// Trim history if it exceeds max size
	maxEntries := p.getMaxHistoryEntries()
	if len(p.history) > maxEntries {
		p.history = p.history[len(p.history)-maxEntries:]
	}
}
