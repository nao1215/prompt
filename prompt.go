// Package prompt provides a library for building powerful interactive terminal prompts.
// This is a modern replacement for go-prompt with better cross-platform support,
// simpler API, and built-in color scheme support.
package prompt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/mattn/go-colorable"
)

// Windows OS name constant
const windowsOS = "windows"

// Common errors
var (
	// ErrEOF is returned when the user presses Ctrl+D or EOF is encountered
	ErrEOF = errors.New("EOF")
	// ErrInterrupted is returned when the user presses Ctrl+C
	ErrInterrupted = errors.New("interrupted")
)

// Prompt represents an interactive terminal prompt.
type Prompt struct {
	config         Config
	output         io.Writer
	history        []string
	historyManager *historyManager
	buffer         []rune
	cursor         int
	renderer       *renderer
	terminal       terminalInterface
	keyMap         *KeyMap
	rawActive      bool // Whether the terminal is currently in raw mode

	// pending holds runes that were read before they were needed and must be
	// delivered before anything else: a rune read past the end of an escape
	// sequence, or input typed while WatchInterrupt was watching. It is read from
	// the front, so the order the user typed in is the order they come back.
	pendingMu sync.Mutex
	pending   []rune

	// reads is the shared input channel, started by the first WatchInterrupt and
	// used from then on so a watcher and the line editor cannot each take half of
	// what was typed. It is nil until then, leaving the classic path a direct
	// terminal read.
	readerOnce     sync.Once
	reads          chan readResult
	readErr        error // the error that ended the reader; read after reads is closed
	readerStop     chan struct{}
	readerStopOnce sync.Once
	// readerDone is closed when the reader goroutine returns, so its release can
	// be observed rather than assumed.
	readerDone chan struct{}
}

// readResult is one rune from the shared input reader.
type readResult struct {
	r rune
}

// KeyAction represents the action to perform when a key is pressed
type KeyAction int

// Key action constants define the actions that can be performed when keys are pressed
const (
	ActionNone KeyAction = iota
	ActionSubmit
	ActionCancel
	ActionMoveLeft
	ActionMoveRight
	ActionMoveUp
	ActionMoveDown
	ActionMoveHome
	ActionMoveEnd
	ActionMoveWordLeft
	ActionMoveWordRight
	ActionDeleteChar
	ActionDeleteLine
	ActionDeleteToEnd
	ActionDeleteWordBack
	ActionComplete
	ActionHistoryUp
	ActionHistoryDown
	ActionHistorySearch
	ActionNewLine
	ActionPasteStart
	ActionPasteEnd
	// ActionClearScreen clears the terminal screen and redraws the prompt with
	// the current input preserved, like Ctrl+L in a typical shell.
	ActionClearScreen
)

const (
	bracketedPasteEnableSequence  = "\x1b[?2004h"
	bracketedPasteDisableSequence = "\x1b[?2004l"
	// maxCSILength bounds how much of a CSI sequence is remembered while looking
	// for its final byte. It is far longer than any key sequence a terminal
	// sends; a sequence that outgrows it is read to its end and reported as no
	// sequence at all, since nothing could be bound to it anyway.
	maxCSILength = 32
	// csiParamFirst and csiParamLast bracket the parameter and intermediate
	// bytes of a CSI sequence, and csiFinalFirst and csiFinalLast its final
	// byte. A byte outside both ranges aborts the sequence.
	csiParamFirst = ' '
	csiParamLast  = '?'
	csiFinalFirst = '@'
	csiFinalLast  = '~'
)

// KeyMap holds the key binding configuration
type KeyMap struct {
	bindings  map[rune]KeyAction
	sequences map[string]KeyAction
}

// NewDefaultKeyMap creates the default key bindings for the prompt.
//
// The default key map includes common terminal shortcuts and navigation keys.
// You can create a custom key map by modifying the returned KeyMap or by
// creating a new one and using the Bind and BindSequence methods.
//
// Default key bindings:
//   - Enter/Return: Submit input
//   - Ctrl+C: Cancel (interrupt)
//   - Ctrl+A: Move to beginning of line
//   - Ctrl+E: Move to end of line
//   - Ctrl+K: Delete from cursor to end of line
//   - Ctrl+U: Delete entire line
//   - Ctrl+W: Delete word backwards
//   - Ctrl+R: Reverse history search
//   - Ctrl+L: Clear the screen
//   - Tab: Auto-completion
//   - Backspace: Delete character backwards
//   - Arrow keys: Navigate history and move cursor
//   - Home/End: Move to line beginning/end
//   - Delete: Delete character forwards
//   - Ctrl+Left/Right: Move by word
//
// Example:
//
//	keyMap := prompt.NewDefaultKeyMap()
//	// Add custom binding for Ctrl+L to clear screen
//	keyMap.Bind('\x0C', prompt.ActionNewLine)
//
//	config := prompt.Config{
//		Prefix: "$ ",
//		KeyMap: keyMap,
//	}
func NewDefaultKeyMap() *KeyMap {
	km := &KeyMap{
		bindings:  make(map[rune]KeyAction),
		sequences: make(map[string]KeyAction),
	}

	// Default key bindings
	km.bindings['\r'] = ActionSubmit
	km.bindings['\n'] = ActionSubmit
	km.bindings['\x03'] = ActionCancel         // Ctrl+C
	km.bindings['\x01'] = ActionMoveHome       // Ctrl+A
	km.bindings['\x05'] = ActionMoveEnd        // Ctrl+E
	km.bindings['\x0B'] = ActionDeleteToEnd    // Ctrl+K
	km.bindings['\x15'] = ActionDeleteLine     // Ctrl+U
	km.bindings['\x17'] = ActionDeleteWordBack // Ctrl+W
	km.bindings['\x12'] = ActionHistorySearch  // Ctrl+R
	km.bindings['\x0C'] = ActionClearScreen    // Ctrl+L
	km.bindings['\t'] = ActionComplete
	km.bindings['\x7f'] = ActionDeleteChar // Backspace
	km.bindings['\b'] = ActionDeleteChar   // Backspace

	// Escape sequences
	km.sequences["[A"] = ActionMoveUp
	km.sequences["[B"] = ActionMoveDown
	km.sequences["[C"] = ActionMoveRight
	km.sequences["[D"] = ActionMoveLeft
	km.sequences["[H"] = ActionMoveHome
	km.sequences["[F"] = ActionMoveEnd
	km.sequences["[1;5C"] = ActionMoveWordRight // Ctrl+Right
	km.sequences["[1;5D"] = ActionMoveWordLeft  // Ctrl+Left
	km.sequences["[3~"] = ActionDeleteChar      // Delete
	km.sequences["[200~"] = ActionPasteStart
	km.sequences["[201~"] = ActionPasteEnd

	return km
}

// Bind adds or updates a key binding for a single character.
//
// Use this method to bind actions to control characters, printable characters,
// or special keys that can be represented as a single rune.
//
// Example:
//
//	keyMap := prompt.NewDefaultKeyMap()
//	// Bind Ctrl+L (\x0C) to clear the current line
//	keyMap.Bind('\x0C', prompt.ActionDeleteLine)
//	// Bind F1 key (if represented as a single rune)
//	keyMap.Bind('\x91', prompt.ActionComplete)
func (km *KeyMap) Bind(key rune, action KeyAction) {
	km.bindings[key] = action
}

// BindSequence adds or updates an escape sequence binding.
//
// Use this method to bind actions to escape sequences like function keys,
// arrow keys, or other multi-character key combinations that start with ESC.
// The sequence should not include the initial ESC character.
//
// Example:
//
//	keyMap := prompt.NewDefaultKeyMap()
//	// Bind F1 key (ESC + OP)
//	keyMap.BindSequence("OP", prompt.ActionComplete)
//	// Bind Shift+Tab (ESC + [Z)
//	keyMap.BindSequence("[Z", prompt.ActionHistoryUp)
//	// Bind Page Up (ESC + [5~)
//	keyMap.BindSequence("[5~", prompt.ActionHistoryUp)
func (km *KeyMap) BindSequence(seq string, action KeyAction) {
	km.sequences[seq] = action
}

// GetAction returns the action for a key, or ActionNone if not bound
func (km *KeyMap) GetAction(key rune) KeyAction {
	if km == nil || km.bindings == nil {
		return ActionNone
	}
	if action, exists := km.bindings[key]; exists {
		return action
	}
	return ActionNone
}

// GetSequenceAction returns the action for an escape sequence, or ActionNone if not bound
func (km *KeyMap) GetSequenceAction(seq string) KeyAction {
	if km == nil || km.sequences == nil {
		return ActionNone
	}
	if action, exists := km.sequences[seq]; exists {
		return action
	}
	return ActionNone
}

// HistoryConfig holds all history-related configuration.
//
// This struct consolidates all history settings for memory limits
// and file persistence options. History data is loaded from files
// or accumulated during runtime usage.
//
// File path supports multiple formats:
// - Empty string: Memory-only history (no persistence)
// - Absolute path: "/home/user/.app_history"
// - Home directory: "~/.app_history"
// - Relative path: "./app_history" (converted to absolute)
// - XDG compliant: Use GetDefaultHistoryFile() for "~/.config/prompt/history"
//
// The implementation follows XDG Base Directory Specification when possible.
type HistoryConfig struct {
	Enabled     bool   // Enable/disable history functionality
	MaxEntries  int    // Maximum number of entries to keep in memory (default: 1000)
	File        string // File path for history persistence (empty = memory only)
	MaxFileSize int64  // Maximum file size in bytes before rotation (default: 1MB)
	MaxBackups  int    // Maximum number of backup files to keep (default: 3)
}

// Config holds the configuration for a prompt.
type Config struct {
	Prefix        string                      // Prompt prefix (e.g., "$ ")
	Completer     func(Document) []Suggestion // Completion function (accepts Document for context)
	HistoryConfig *HistoryConfig              // History configuration (nil for default)
	ColorScheme   *ColorScheme                // Color scheme (nil for default)
	KeyMap        *KeyMap                     // Key bindings (nil for default)
	Theme         *ColorScheme                // Alias for ColorScheme for compatibility
	Multiline     bool                        // Enable multiline input mode
	IsComplete    func(input string) bool     // Decides whether Enter submits in multiline mode (nil = always submit)
	AutoIndent    func(before string) string  // Decides what a new line opens with (nil = nothing)
	WordEscape    bool                        // Treat backslash-escaped whitespace as part of a word during completion
	// Highlighter colors runs of the input as it is drawn. See WithHighlighter.
	Highlighter func(input string) []StyleSpan
	// ContinuationPrefix is drawn in front of every line after the first while a
	// multiline entry is still being typed. See WithContinuationPrefix.
	ContinuationPrefix string
	// PersistentRawMode keeps the terminal in raw mode across consecutive Run
	// calls instead of re-acquiring it on every call. See WithPersistentRawMode
	// for the rationale and caveats.
	PersistentRawMode bool
}

// Option represents a configuration option for prompt
type Option func(*Config)

// WithCompleter sets the completion function
func WithCompleter(completer func(Document) []Suggestion) Option {
	return func(c *Config) {
		c.Completer = completer
	}
}

// WithHistory configures history settings with the provided configuration.
// This is the recommended way to configure all history-related options.
//
// Example:
//
//	prompt.New("$ ", prompt.WithHistory(&prompt.HistoryConfig{
//		Enabled:     true,
//		MaxEntries:  100,
//		File:        "~/.myapp_history",
//	}))
func WithHistory(historyConfig *HistoryConfig) Option {
	return func(c *Config) {
		c.HistoryConfig = historyConfig
	}
}

// WithMemoryHistory is a convenience function for memory-only history setup.
//
// Example:
//
//	prompt.New("$ ", prompt.WithMemoryHistory(100))
func WithMemoryHistory(maxEntries int) Option {
	return func(c *Config) {
		if maxEntries <= 0 {
			maxEntries = 1000 // Default
		}
		c.HistoryConfig = &HistoryConfig{
			Enabled:    true,
			MaxEntries: maxEntries,
			File:       "", // Memory only
		}
	}
}

// WithFileHistory is a convenience function for history with file persistence.
//
// Example:
//
//	prompt.New("$ ", prompt.WithFileHistory("~/.myapp_history", 100))
func WithFileHistory(file string, maxEntries int) Option {
	return func(c *Config) {
		if maxEntries <= 0 {
			maxEntries = 1000 // Default
		}
		c.HistoryConfig = &HistoryConfig{
			Enabled:     true,
			MaxEntries:  maxEntries,
			File:        file,
			MaxFileSize: 1024 * 1024, // 1MB default
			MaxBackups:  3,           // Default
		}
	}
}

// WithColorScheme sets the color scheme
func WithColorScheme(colorScheme *ColorScheme) Option {
	return func(c *Config) {
		c.ColorScheme = colorScheme
	}
}

// WithKeyMap sets the key bindings
func WithKeyMap(keyMap *KeyMap) Option {
	return func(c *Config) {
		c.KeyMap = keyMap
	}
}

// WithTheme sets the color scheme (alias for compatibility)
func WithTheme(theme *ColorScheme) Option {
	return func(c *Config) {
		c.Theme = theme
	}
}

// WithMultiline enables or disables multiline input mode
func WithMultiline(multiline bool) Option {
	return func(c *Config) {
		c.Multiline = multiline
	}
}

// WithIsComplete sets a predicate that decides, in multiline mode, whether
// pressing Enter submits the buffer or inserts a newline. The predicate receives
// the whole buffer and returns true when the input is a complete statement ready
// to run; when false, Enter inserts a newline so an embedding app can buffer
// multi-line input (for example SQL until a trailing ";"). A trailing backslash
// and bracketed paste still force a newline regardless of the predicate. When nil
// (default) or when multiline mode is off, Enter always submits.
func WithIsComplete(isComplete func(input string) bool) Option {
	return func(c *Config) {
		c.IsComplete = isComplete
	}
}

// WithAutoIndent sets what a new line opens with while a multiline entry is
// being typed.
//
// It is called whenever a newline is inserted -- because the input is
// incomplete, because a newline key was pressed, or because the line ended in a
// backslash -- and is given the input up to where the line breaks, without the
// newline itself. What it returns is inserted at the start of the new line.
//
// The prompt has no opinion about what that should be. An indenter for a
// bracketed language returns one unit per bracket left open; one for a language
// with none can return the leading whitespace of the line it was given, so a
// continuation stays where the writer put it. Returning "" indents nothing,
// which is what a prompt without this option does.
//
// What it returns is part of the input, so it is submitted, recorded in
// history, and given to the completer like anything else typed. An indenter
// that returns something other than whitespace is therefore writing input, not
// decorating it.
//
// Example:
//
//	// Keep the indentation of the line being continued.
//	prompt.WithAutoIndent(func(before string) string {
//		line := before[strings.LastIndex(before, "\n")+1:]
//		return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
//	})
func WithAutoIndent(indent func(before string) string) Option {
	return func(c *Config) {
		c.AutoIndent = indent
	}
}

// WithHighlighter sets the function that colors the input as it is drawn.
//
// It is given the whole input and returns the runs to draw in a color other
// than the scheme's Input color, as rune offsets into that input. Everything
// no run covers keeps the scheme's color. It is called on every render, which
// is once per keystroke, so it should be cheap over a line's worth of text.
//
// The prompt has no opinion about what the runs mean, which is what keeps this
// out of any one language's business: a highlighter for SQL colors keywords
// and string literals, one for a shell colors the command and its flags.
//
// It decides colors and nothing else. The input is drawn exactly as it is
// whatever the highlighter returns, and the prompt measures its layout from
// that text rather than from what is written to the terminal, so a highlighter
// cannot move the cursor away from the character under it or wrap a line early.
// A run reaching outside the input, an inverted one, and two that overlap are
// all drawn over rather than rejected: a color is a decoration, and getting
// one wrong must not cost the user the line they are typing.
//
// Example:
//
//	prompt.WithHighlighter(func(input string) []prompt.StyleSpan {
//		var spans []prompt.StyleSpan
//		for _, kw := range keywordRuns(input) {
//			spans = append(spans, prompt.StyleSpan{
//				Start: kw.start, End: kw.end,
//				Color: prompt.Color{R: 198, G: 120, B: 221, Bold: true},
//			})
//		}
//		return spans
//	})
func WithHighlighter(highlighter func(input string) []StyleSpan) Option {
	return func(c *Config) {
		c.Highlighter = highlighter
	}
}

// WithContinuationPrefix sets the string drawn in front of every line after the
// first while a multiline entry is still being typed.
//
// Without one, a buffer that WithIsComplete declined leaves the cursor on a bare
// line with nothing in front of it, which is indistinguishable from a hung
// program: the user cannot tell that the prompt is waiting for the rest of a
// statement. A continuation prefix is how sqlite3 ("   ...> "), psql ("db-# "),
// and mysql ("    -> ") show the same state.
//
// The prefix is drawn in the prompt's own color and is counted when positioning
// the cursor and when measuring how many terminal rows the input occupies, so
// editing on a continuation line lands where the character is. It has no effect
// on the returned input, which never contains it, and none outside multiline
// mode, where there is only ever one line. The default is empty, which preserves
// the previous appearance.
func WithContinuationPrefix(prefix string) Option {
	return func(c *Config) {
		c.ContinuationPrefix = prefix
	}
}

// WithPersistentRawMode keeps the terminal in raw mode across consecutive Run
// (RunWithContext) calls instead of entering raw mode at the start of every call
// and restoring it when the call returns.
//
// A REPL that calls Run once per line otherwise toggles the terminal between raw
// and cooked around every command. Between one line's restore and the next line's
// re-acquisition the read loop is not consuming input, so bytes that a fast or
// automated driver (a pipe or pseudo-terminal) sends right after the prompt is
// re-rendered can be lost. Entering raw mode once for the whole session closes
// that window and makes input handling deterministic regardless of timing or
// machine load.
//
// When enabled, raw mode is acquired on the first Run call and released once, by
// Close or when input reaches EOF. An interrupt (Ctrl+C) does not release it: it
// ends the line rather than the session, and a REPL that calls Run again would
// otherwise pay the mode switch on every Ctrl+C. An application that treats
// ErrInterrupted as fatal must therefore call Close, as it should anyway.
//
// Because the terminal stays in raw mode between calls, an embedding application
// that prints its own output between prompts must terminate lines with "\r\n"
// rather than "\n". It is off by default so the classic single-shot usage keeps
// cooked-mode output after each Run.
func WithPersistentRawMode() Option {
	return func(c *Config) {
		c.PersistentRawMode = true
	}
}

// WithWordEscape makes completion treat backslash-escaped whitespace as part of
// the word before the cursor. A shell-style path like "my\ data.csv" is then
// completed and accepted as one word instead of breaking at the escaped space.
// Enable it when the embedding app accepts escaped paths (for example a SQL shell
// whose .import command honors backslash escapes). Off by default; default word
// boundaries are unchanged.
func WithWordEscape() Option {
	return func(c *Config) {
		c.WordEscape = true
	}
}

// Range is a half-open span [Start, End) of a Document's text, counted in
// runes, which is the unit Document.CursorPosition is counted in.
type Range struct {
	// Start is the first rune of the span.
	Start int
	// End is one past the last rune of the span. Start == End names an empty
	// span, i.e. a position to insert at.
	End int
}

// Suggestion represents a completion suggestion.
type Suggestion struct {
	Text        string // The text to complete
	Description string // Description of the suggestion
	// Replace, when non-nil, is the span of the input that accepting this
	// suggestion overwrites with Text. It lets a completer that knows more about
	// the input than a word boundary can express — a qualified name, a
	// case-insensitive match, a token the prompt would not have split there —
	// say exactly what its suggestion stands for.
	//
	// A completer that sets it takes over matching: the prompt applies the span
	// literally and skips the prefix filter it otherwise runs over the returned
	// suggestions, which is a case-sensitive test against the word before the
	// cursor. Leave it nil to keep that behavior. A span outside the buffer, or
	// an inverted one, is clamped rather than rejected.
	Replace *Range
}

// Suggest is an alias for Suggestion for compatibility
type Suggest = Suggestion

// New creates a new prompt with the specified prefix and optional configuration.
//
// This is the recommended way to create a new prompt as it provides a clean API
// with sensible defaults and allows for flexible configuration through options.
//
// Example:
//
//	// Basic prompt with just a prefix
//	p, err := prompt.New("$ ")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer p.Close()
//
//	// Prompt with completion and history
//	p, err := prompt.New("$ ",
//		prompt.WithCompleter(func(d prompt.Document) []prompt.Suggestion {
//			if strings.HasPrefix(d.Text, "git") {
//				return []prompt.Suggestion{
//					{Text: "git status", Description: "Show working tree status"},
//					{Text: "git commit", Description: "Record changes to repository"},
//				}
//			}
//			return nil
//		}),
//		prompt.WithMemoryHistory(100),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer p.Close()
//
//	result, err := p.Run()
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("You entered: %s\n", result)
func New(prefix string, options ...Option) (*Prompt, error) {
	config := Config{
		Prefix: prefix,
	}

	// Apply options
	for _, option := range options {
		option(&config)
	}

	return newFromConfig(config)
}

func newFromConfig(config Config) (*Prompt, error) {
	// Set defaults for history config
	if config.HistoryConfig == nil {
		config.HistoryConfig = defaultHistoryConfig()
	} else {
		// Set defaults for incomplete history config
		if config.HistoryConfig.MaxEntries <= 0 {
			config.HistoryConfig.MaxEntries = 1000
		}
		if config.HistoryConfig.MaxFileSize <= 0 {
			config.HistoryConfig.MaxFileSize = 1024 * 1024 // 1MB
		}
		if config.HistoryConfig.MaxBackups <= 0 {
			config.HistoryConfig.MaxBackups = 3
		}
	}
	// Handle Theme alias
	if config.Theme != nil && config.ColorScheme == nil {
		config.ColorScheme = config.Theme
	}
	if config.ColorScheme == nil {
		config.ColorScheme = ThemeDefault
	}
	if config.KeyMap == nil {
		config.KeyMap = NewDefaultKeyMap()
	}

	// Setup output writer with color support
	var output io.Writer = os.Stdout
	if runtime.GOOS == windowsOS {
		// Use colorable for Windows ANSI color support
		output = colorable.NewColorableStdout()
	}

	// Create terminal interface using external libraries
	terminal, err := newRealTerminal()
	if err != nil {
		return nil, fmt.Errorf("failed to create terminal: %w", err)
	}

	// Initialize history manager
	historyManager := newHistoryManager(config.HistoryConfig)

	// Load history from file if configured
	if err := historyManager.loadHistory(); err != nil {
		return nil, fmt.Errorf("failed to load history: %w", err)
	}

	// History manager is ready with either loaded history or empty history

	// Initialize prompt
	p := &Prompt{
		config:         config,
		output:         output,
		history:        historyManager.getHistory(),
		historyManager: historyManager,
		terminal:       terminal,
		keyMap:         config.KeyMap,
	}

	// Initialize renderer
	p.renderer = newRenderer(output, config.ColorScheme, p.terminal)

	return p, nil
}

// Run starts the interactive prompt and returns the user input.
//
// This is a convenience method that calls RunWithContext with a background context.
// The prompt will accept user input until Enter is pressed or an error occurs.
//
// Example:
//
//	p, _ := prompt.New(prompt.Config{Prefix: "Enter command: "})
//	defer p.Close()
//
//	input, err := p.Run()
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("User entered: %s\n", input)
func (p *Prompt) Run() (string, error) {
	return p.RunWithContext(context.Background())
}

// RunWithContext starts the interactive prompt with context support.
//
// The prompt can be canceled via the provided context, allowing for timeouts
// or cancellation from other goroutines. The function supports all configured
// key bindings, multi-line input, completion, and history navigation.
//
// Supported key bindings include:
//   - Enter: Submit input (or add newline in multi-line mode)
//   - Ctrl+C: Discard the current line and return ErrInterrupted
//   - Ctrl+D: EOF when buffer is empty
//   - Esc: Close the completion popup
//   - Arrow keys: Navigate history or move cursor
//   - Ctrl+A/Home: Move to beginning of line
//   - Ctrl+E/End: Move to end of line
//   - Ctrl+K: Delete from cursor to end of line
//   - Ctrl+U: Delete entire line
//   - Ctrl+W: Delete word backwards
//   - Ctrl+R: Reverse history search
//   - Tab: Auto-completion
//
// Example with timeout:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	p, _ := prompt.New(prompt.Config{Prefix: "Command: "})
//	defer p.Close()
//
//	input, err := p.RunWithContext(ctx)
//	if err == context.DeadlineExceeded {
//		fmt.Println("Timeout reached")
//		return
//	}
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Input: %s\n", input)
func (p *Prompt) RunWithContext(ctx context.Context) (string, error) {
	if err := p.enterRawMode(); err != nil {
		return "", fmt.Errorf("failed to enter raw mode: %w", err)
	}

	defer func() {
		// In persistent mode the terminal stays in raw mode across calls; it is
		// restored by Close, on interrupt, or on EOF instead of per call. In the
		// default mode restore on every return. exitRawMode is idempotent, so the
		// interrupt and EOF paths that restore early make this a no-op.
		if p.config.PersistentRawMode {
			return
		}
		if err := p.exitRawMode(); err != nil {
			// Log error but don't return it as we're in defer
			fmt.Fprintf(os.Stderr, "Warning: failed to exit raw mode: %v\n", err)
		}
	}()

	// Initialize buffer and display. The renderer starts each line knowing
	// nothing about what is on screen: the previous line is finished, and
	// whatever the application printed after it is not this prompt's to erase.
	p.buffer = []rune{}
	p.cursor = 0
	p.renderer.forgetBlock()
	if err := p.render(); err != nil {
		return "", fmt.Errorf("failed to render prompt: %w", err)
	}

	historyIndex := len(p.history)
	inPaste := false
	lastPasted := rune(0)
	var suggestions []Suggestion
	selectedSuggestion := 0
	suggestionOffset := 0 // Track the offset for scrolling through suggestions

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Read key input
		r, err := p.readRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Input reached EOF; the session is over, so restore the terminal
				// even in persistent mode (the deferred cleanup skips it there).
				p.restoreOnExit()
				return "", ErrEOF
			}
			return "", fmt.Errorf("failed to read input: %w", err)
		}

		var action KeyAction

		// Handle escape sequences
		switch {
		case r == '\x1b':
			seq, err := p.readEscapeSequence()
			if err != nil {
				if errors.Is(err, io.EOF) {
					p.restoreOnExit()
					return "", ErrEOF
				}
				return "", fmt.Errorf("failed to read input: %w", err)
			}
			if seq == "" {
				// A bare Escape closes the completion popup, as it does in an
				// editor. Enter accepts a suggestion while one is displayed, so a
				// popup with no way out left the line unrunnable.
				suggestions = nil
			}
			action = p.keyMap.GetSequenceAction(seq)
			// Inside a paste only the end marker is a signal. Anything else is
			// content the terminal passed through: the ESC that introduced it is a
			// control byte and is dropped, while the rest of the sequence is text
			// and is kept, so pasting a colored log does not lose its words.
			if inPaste && action != ActionPasteEnd {
				for _, pasted := range seq {
					lastPasted = p.insertPastedRune(pasted, lastPasted)
				}
				suggestions = nil
				action = ActionNone
			}
		case inPaste:
			// Pasted content is data, not keystrokes: it goes into the buffer as
			// written instead of running completion (TAB), ending the prompt
			// (Ctrl+C), or submitting (Enter).
			lastPasted = p.insertPastedRune(r, lastPasted)
			suggestions = nil
			if err := p.renderWithSuggestionsOffset(nil, 0, 0); err != nil {
				return "", fmt.Errorf("failed to render: %w", err)
			}
			continue
		default:
			action = p.keyMap.GetAction(r)
		}

		// Execute action
		switch action {
		case ActionSubmit:
			// If suggestions are displayed, accept the selected one and continue editing
			if len(suggestions) > 0 {
				p.acceptSuggestion(suggestions[selectedSuggestion])
				suggestions = nil
				// Clear suggestions and continue editing without submitting
			} else {
				if p.isShiftEnter() {
					p.insertNewline()
					suggestions = nil
				} else if p.config.Multiline && p.config.IsComplete != nil && !p.config.IsComplete(string(p.buffer)) {
					// The app reports the statement is incomplete, so keep editing on a
					// new line instead of submitting (e.g. SQL buffered until ";").
					p.insertNewline()
					suggestions = nil
				} else {
					result := string(p.buffer)
					if result != "" && (len(p.history) == 0 || p.history[len(p.history)-1] != result) {
						p.addToHistory(result)
					}
					fmt.Fprint(p.output, "\r\n")
					// The deferred cleanup restores the terminal in the default
					// mode; in persistent mode it is kept for the next Run.
					return result, nil
				}
			}

		case ActionCancel:
			// The interrupt discards the line, not the session: a REPL reports
			// ErrInterrupted and calls Run again. The terminal is therefore
			// released only when this call owns it. A persistent session keeps raw
			// mode, so the mode-switch window that loses input does not reopen on
			// every Ctrl+C; Close and EOF restore it.
			if !p.config.PersistentRawMode {
				p.restoreOnExit()
			}
			fmt.Fprint(p.output, "^C\r\n")
			return "", ErrInterrupted

		case ActionMoveLeft:
			if p.cursor > 0 {
				p.cursor--
			}

		case ActionMoveRight:
			if len(suggestions) > 0 {
				// Accept current suggestion and continue editing
				p.acceptSuggestion(suggestions[selectedSuggestion])
				suggestions = nil
			} else if p.cursor < len(p.buffer) {
				p.cursor++
			}

		case ActionMoveUp:
			if len(suggestions) > 0 {
				// Navigate suggestions with scrolling
				if selectedSuggestion > 0 {
					selectedSuggestion--
					// Scroll up if needed
					if selectedSuggestion < suggestionOffset {
						suggestionOffset = selectedSuggestion
					}
				}
			} else if p.isMultiLine() {
				// Navigate up within multi-line input
				p.cursor = p.findCursorUp()
			} else {
				// Navigate history
				if historyIndex > 0 {
					historyIndex--
					p.setBuffer(p.history[historyIndex])
					suggestions = nil
				}
			}

		case ActionMoveDown:
			if len(suggestions) > 0 {
				// Navigate suggestions with scrolling
				maxDisplayed := 10 // Maximum suggestions to display at once
				if selectedSuggestion < len(suggestions)-1 {
					selectedSuggestion++
					// Scroll down if needed
					if selectedSuggestion >= suggestionOffset+maxDisplayed {
						suggestionOffset = selectedSuggestion - maxDisplayed + 1
					}
				}
			} else if p.isMultiLine() {
				// Navigate down within multi-line input
				p.cursor = p.findCursorDown()
			} else {
				// Navigate history
				if historyIndex < len(p.history) {
					historyIndex++
					if historyIndex == len(p.history) {
						p.setBuffer("")
					} else {
						p.setBuffer(p.history[historyIndex])
					}
					suggestions = nil
				}
			}

		case ActionMoveHome:
			if p.isMultiLine() {
				p.cursor = p.findLineStart()
			} else {
				p.cursor = 0
			}

		case ActionMoveEnd:
			if p.isMultiLine() {
				p.cursor = p.findLineEnd()
			} else {
				p.cursor = len(p.buffer)
			}

		case ActionMoveWordLeft:
			p.cursor = p.findWordBoundary(-1)

		case ActionMoveWordRight:
			p.cursor = p.findWordBoundary(1)

		case ActionDeleteChar:
			if r == '\x7f' || r == '\b' {
				// Backspace
				if p.cursor > 0 {
					p.buffer = append(p.buffer[:p.cursor-1], p.buffer[p.cursor:]...)
					p.cursor--
					suggestions = nil
				}
			} else {
				// Delete key
				if p.cursor < len(p.buffer) {
					p.buffer = append(p.buffer[:p.cursor], p.buffer[p.cursor+1:]...)
					suggestions = nil
				}
			}

		case ActionDeleteLine:
			p.buffer = []rune{}
			p.cursor = 0

		case ActionDeleteToEnd:
			if p.isMultiLine() {
				lineEnd := p.findLineEnd()
				p.buffer = append(p.buffer[:p.cursor], p.buffer[lineEnd:]...)
			} else {
				p.buffer = p.buffer[:p.cursor]
			}

		case ActionDeleteWordBack:
			if p.cursor > 0 {
				newPos := p.findWordBoundary(-1)
				p.buffer = append(p.buffer[:newPos], p.buffer[p.cursor:]...)
				p.cursor = newPos
				suggestions = nil
			}

		case ActionComplete:
			if p.config.Completer != nil {
				if len(suggestions) > 0 {
					// TAB accepts the currently selected suggestion
					p.acceptSuggestion(suggestions[selectedSuggestion])
					suggestions = nil
				} else {
					// Generate new suggestions
					doc := Document{
						Text:           string(p.buffer),
						CursorPosition: p.cursor,
					}
					suggestions = p.config.Completer(doc)
					selectedSuggestion = 0
					suggestionOffset = 0 // Reset scroll position

					// Smart matching: filter suggestions based on current input.
					// A completer that names the span each suggestion replaces has
					// already decided what matches, and by a rule this filter cannot
					// reproduce, so it is skipped for such a set.
					currentWord := p.completionWord(doc)
					if currentWord != "" && !hasReplaceRange(suggestions) {
						// Filter suggestions to only show those that match the current input
						filteredSuggestions := make([]Suggestion, 0)
						for _, suggestion := range suggestions {
							if strings.HasPrefix(suggestion.Text, currentWord) {
								filteredSuggestions = append(filteredSuggestions, suggestion)
							}
						}
						suggestions = filteredSuggestions

						// If no suggestions match, don't show anything
						if len(suggestions) == 0 {
							suggestions = nil
						} else if len(suggestions) == 1 {
							// If only one suggestion matches, auto-complete
							p.acceptSuggestion(suggestions[0])
							suggestions = nil
						}
						// Multiple filtered suggestions: show them for user selection
					} else {
						// No current word (at space or beginning)
						// Show all suggestions for user selection
						if len(suggestions) == 1 {
							// Single suggestion: auto-complete
							p.acceptSuggestion(suggestions[0])
							suggestions = nil
						}
						// Multiple suggestions: show them for user selection
					}
				}
			}

		case ActionHistorySearch:
			if result, err := p.searchHistory(); err == nil && result != "" {
				p.setBuffer(result)
				historyIndex = len(p.history)
			}
			// Re-render after search
			if err := p.render(); err != nil {
				return "", fmt.Errorf("failed to render prompt: %w", err)
			}

		case ActionNewLine:
			p.insertNewline()
			suggestions = nil

		case ActionPasteStart:
			inPaste = true
			// Each paste starts its own CRLF state: a paste ending in CR must not
			// swallow the newline a later paste begins with.
			lastPasted = 0
			suggestions = nil

		case ActionPasteEnd:
			inPaste = false

		case ActionClearScreen:
			// Clear the whole screen and redraw the prompt at the top with the
			// current input preserved. The trailing render below repaints it.
			p.renderer.clearScreen()
			suggestions = nil

		default:
			// Handle regular character input
			if r >= 32 && r < 127 || r > 127 { // Printable characters
				// Don't insert TAB as regular character (should be handled by ActionComplete)
				if r == '\t' {
					// TAB should have been handled as ActionComplete, ignore
					continue
				}
				p.insertRune(r)
				suggestions = nil             // Clear suggestions on new input
				historyIndex = len(p.history) // Reset history position
			} else if r == '\x04' { // Ctrl+D (EOF)
				if len(p.buffer) == 0 {
					// Ctrl+D on an empty buffer ends the session; restore the
					// terminal even in persistent mode before returning.
					p.restoreOnExit()
					return "", io.EOF
				}
			}
		}

		// Re-render with suggestions if any
		if err := p.renderWithSuggestionsOffset(suggestions, selectedSuggestion, suggestionOffset); err != nil {
			return "", fmt.Errorf("failed to render: %w", err)
		}
	}
}

// Close closes the prompt and cleans up resources.
//
// This method should be called when the prompt is no longer needed
// to prevent resource leaks. It's safe to call Close multiple times.
// It's recommended to use defer for automatic cleanup.
//
// Example:
//
//	p, err := prompt.New(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer p.Close() // Ensure cleanup
//
//	// Use the prompt...
//	result, err := p.Run()
func (p *Prompt) Close() error {
	// Release the shared reader before the terminal: closing the terminal ends a
	// read in progress, and this ends a goroutine waiting to hand over a rune
	// nobody is collecting.
	p.stopInputReader()

	// Restore raw mode if a persistent session (WithPersistentRawMode) left the
	// terminal in raw mode. Idempotent, so it is a no-op in the default mode.
	if err := p.exitRawMode(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to exit raw mode: %v\n", err)
	}

	// Restore cursor visibility before closing
	if p.output != nil {
		fmt.Fprint(p.output, "\x1b[?25h") // Show cursor
		fmt.Fprint(p.output, "\n")        // Move to new line
	}

	// Save history before closing
	if p.historyManager != nil {
		if err := p.historyManager.saveHistory(); err != nil {
			// Log error but continue with cleanup
			fmt.Fprintf(os.Stderr, "Warning: failed to save history: %v\n", err)
		}
	}

	// Close terminal resources to prevent file descriptor leaks
	if p.terminal != nil {
		err := p.terminal.Close()
		p.awaitReaderExit()
		return err
	}
	return nil
}

// Helper methods

func (p *Prompt) insertRune(r rune) {
	p.buffer = append(p.buffer[:p.cursor], append([]rune{r}, p.buffer[p.cursor:]...)...)
	p.cursor++
}

// insertPastedRune inserts one rune of bracketed-paste content and returns it,
// so the caller can pass it back as prev on the next call.
//
// A line break becomes exactly one "\n" however the terminal spells it: pasting
// Windows text delivers CR LF, and inserting a newline for each of them turned
// every line break into a blank line. Control bytes other than TAB are dropped
// rather than inserted, because they are neither text the user pasted nor
// commands they pressed.
func (p *Prompt) insertPastedRune(r, prev rune) rune {
	switch {
	case r == '\r':
		p.insertRune('\n')
	case r == '\n':
		if prev != '\r' {
			p.insertRune('\n')
		}
	case r == '\t' || (r >= 32 && r != 0x7f):
		p.insertRune(r)
	}
	return r
}

// insertNewline breaks the line at the cursor and opens the next one with
// whatever the auto-indent hook asks for. Every way a line can break goes
// through here, so a continuation looks the same however it was asked for.
func (p *Prompt) insertNewline() {
	// The indenter is given the line it is continuing, so it is asked before the
	// newline is inserted rather than after.
	var indent string
	if p.config.AutoIndent != nil {
		indent = p.config.AutoIndent(string(p.buffer[:p.cursor]))
	}
	p.insertRune('\n')
	if indent != "" {
		p.insertText(indent)
	}
}

func (p *Prompt) insertText(text string) {
	runes := []rune(text)
	p.buffer = append(p.buffer[:p.cursor], append(runes, p.buffer[p.cursor:]...)...)
	p.cursor += len(runes)
}

func (p *Prompt) setBuffer(text string) {
	p.buffer = []rune(text)
	p.cursor = len(p.buffer)
}

// completionWord returns the word before the cursor used for completion matching
// and acceptance. It honors backslash-escaped whitespace when WithWordEscape is
// set so space-containing paths complete as one word.
func (p *Prompt) completionWord(doc Document) string {
	if p.config.WordEscape {
		return doc.GetWordBeforeCursorEscaped()
	}
	return doc.GetWordBeforeCursor()
}

// hasReplaceRange reports whether any suggestion names the span it replaces,
// which is how a completer says it owns matching for this set.
func hasReplaceRange(suggestions []Suggestion) bool {
	for _, s := range suggestions {
		if s.Replace != nil {
			return true
		}
	}
	return false
}

func (p *Prompt) acceptSuggestion(suggestion Suggestion) {
	// A suggestion that names the span it stands for is applied literally: the
	// completer knows what it matched, and the word-boundary guesswork below
	// cannot express a qualified name or a case-insensitive match.
	if suggestion.Replace != nil {
		p.replaceRange(*suggestion.Replace, suggestion.Text)
		return
	}

	// Get current document state for context
	doc := Document{
		Text:           string(p.buffer),
		CursorPosition: p.cursor,
	}

	// Determine how to apply the suggestion based on context
	beforeCursor := doc.TextBeforeCursor()
	currentWord := p.completionWord(doc)

	if currentWord == "" {
		// Cursor is at space or beginning, just insert the suggestion
		p.insertText(suggestion.Text)
	} else if strings.HasPrefix(suggestion.Text, currentWord) {
		// Suggestion is a completion of current word (e.g., "cre" -> "create")
		suffix := suggestion.Text[len(currentWord):]
		p.insertText(suffix)
	} else {
		// Suggestion is a replacement or subcommand
		// Check if we're at the end of a word (subcommand scenario)
		if p.cursor == len(p.buffer) || !isWordChar(p.buffer[p.cursor]) {
			// At end of word or at space, add space + suggestion
			if beforeCursor != "" && !strings.HasSuffix(beforeCursor, " ") {
				p.insertText(" ")
			}
			p.insertText(suggestion.Text)
		} else {
			// In middle of word, replace current word
			wordStart, wordEnd := p.getCurrentWordBounds()
			p.buffer = append(p.buffer[:wordStart], append([]rune(suggestion.Text), p.buffer[wordEnd:]...)...)
			p.cursor = wordStart + len([]rune(suggestion.Text))
		}
	}
}

// replaceRange overwrites the buffer's runes in r with text and leaves the
// cursor after it. A span outside the buffer, or an inverted one, is clamped
// instead of panicking: a completer's arithmetic mistake should not take the
// line editor down with it.
func (p *Prompt) replaceRange(r Range, text string) {
	start := min(max(r.Start, 0), len(p.buffer))
	end := min(max(r.End, start), len(p.buffer))

	replacement := []rune(text)
	tail := append(replacement, p.buffer[end:]...)
	p.buffer = append(p.buffer[:start:start], tail...)
	p.cursor = start + len(replacement)
}

// getCurrentWordBounds finds the start and end positions of the current word at cursor
func (p *Prompt) getCurrentWordBounds() (start, end int) {
	// Find word start (scan backwards from cursor)
	start = p.cursor
	for start > 0 && isWordChar(p.buffer[start-1]) {
		start--
	}

	// Find word end (scan forwards from cursor)
	end = p.cursor
	for end < len(p.buffer) && isWordChar(p.buffer[end]) {
		end++
	}

	return start, end
}

// History management methods

// GetHistory returns the current command history
func (p *Prompt) GetHistory() []string {
	if p.historyManager != nil && p.historyManager.isEnabled() {
		return p.historyManager.getHistory()
	}
	if p.historyManager != nil && !p.historyManager.isEnabled() {
		return []string{} // Return empty when disabled
	}
	return append([]string{}, p.history...)
}

// AddHistory adds a command to the history
func (p *Prompt) AddHistory(command string) {
	p.addToHistory(command)
}

// ClearHistory clears the command history
func (p *Prompt) ClearHistory() {
	if p.historyManager != nil && p.historyManager.isEnabled() {
		p.historyManager.clearHistory()
	}
	p.history = []string{}
}

// SetHistory replaces the entire history
func (p *Prompt) SetHistory(history []string) {
	if p.historyManager != nil && p.historyManager.isEnabled() {
		p.historyManager.setHistory(history)
		p.history = p.historyManager.getHistory()
	} else {
		p.history = append([]string{}, history...)
	}
	// Trim history if it exceeds max size
	maxEntries := p.getMaxHistoryEntries()
	if len(p.history) > maxEntries {
		p.history = p.history[len(p.history)-maxEntries:]
		if p.historyManager != nil && p.historyManager.isEnabled() {
			p.historyManager.setHistory(p.history)
		}
	}
}

// Configuration update methods

// SetTheme changes the color theme of the prompt
func (p *Prompt) SetTheme(theme *ColorScheme) {
	p.config.ColorScheme = theme
	p.config.Theme = theme
	p.renderer = newRenderer(p.output, theme, p.terminal)
}

// SetPrefix changes the prompt prefix
func (p *Prompt) SetPrefix(prefix string) {
	p.config.Prefix = prefix
}

// SetContinuationPrefix changes the string drawn in front of every line after
// the first while a multiline entry is still being typed. See
// WithContinuationPrefix.
func (p *Prompt) SetContinuationPrefix(prefix string) {
	p.config.ContinuationPrefix = prefix
}

// SetCompleter changes the completion function
func (p *Prompt) SetCompleter(completer func(Document) []Suggestion) {
	p.config.Completer = completer
}

// fuzzyMatcher provides reusable fuzzy matching logic for completions and history search
type fuzzyMatcher struct {
	items []string
}

// NewFuzzyCompleter creates a new fuzzy completer with the given candidates.
//
// The fuzzy completer provides intelligent auto-completion by matching
// user input against a list of candidates using fuzzy string matching.
// It supports partial matches, substring matches, and character-by-character
// fuzzy matching with scoring.
//
// This is a convenience function that returns a completer function that can be
// used directly in Config.Completer. The returned function implements fuzzy
// matching and scoring automatically.
//
// Example:
//
//	candidates := []string{
//		"git status", "git commit", "git push", "git pull",
//		"docker run", "docker build", "docker ps",
//		"kubectl get", "kubectl apply", "kubectl delete",
//	}
//
//	config := prompt.Config{
//		Prefix: "$ ",
//		Completer: prompt.NewFuzzyCompleter(candidates),
//	}
//
//	p, _ := prompt.New(config)
//	defer p.Close()
//	result, _ := p.Run()
func NewFuzzyCompleter(candidates []string) func(Document) []Suggestion {
	fm := &fuzzyMatcher{
		items: candidates,
	}
	return fm.completionFunc
}

// completionFunc returns fuzzy-matched suggestions for the given document context
func (f *fuzzyMatcher) completionFunc(d Document) []Suggestion {
	input := d.TextBeforeCursor()
	if input == "" {
		// Return all items if no input
		suggestions := make([]Suggestion, len(f.items))
		for i, item := range f.items {
			suggestions[i] = Suggestion{
				Text:        item,
				Description: "",
			}
		}
		return suggestions
	}

	matches := f.fuzzySearch(input)
	// Convert to suggestions
	suggestions := make([]Suggestion, len(matches))
	for i, match := range matches {
		suggestions[i] = Suggestion{
			Text:        match.text,
			Description: fmt.Sprintf("score: %d", match.score),
		}
	}
	return suggestions
}

type fuzzyMatch struct {
	text  string
	score int
}

// fuzzySearch performs fuzzy matching against items and returns sorted matches
func (f *fuzzyMatcher) fuzzySearch(query string) []fuzzyMatch {
	if query == "" {
		return nil
	}

	var matches []fuzzyMatch
	queryLower := strings.ToLower(query)

	for _, item := range f.items {
		if score := calculateFuzzyScore(queryLower, strings.ToLower(item), false); score > 0 {
			matches = append(matches, fuzzyMatch{
				text:  item,
				score: score,
			})
		}
	}

	// Sort by score (descending)
	for i := range len(matches) - 1 {
		for j := i + 1; j < len(matches); j++ {
			if matches[i].score < matches[j].score {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	return matches
}

// findWordBoundary finds the next word boundary in the given direction for word-based navigation.
//
// This function implements word-based cursor movement similar to text editors:
//
//	direction > 0 (Ctrl+Right): Moves to the start of the next word
//	  1. Skip any non-word characters from current position
//	  2. Skip through the current word to find its end
//	  3. Return position at the start of the next word
//
//	direction < 0 (Ctrl+Left): Moves to the start of the previous word
//	  1. Move back one position from cursor
//	  2. Skip any trailing non-word characters
//	  3. Skip back through the previous word
//	  4. Return position at the start of that word
//
// Word boundaries are defined by isWordChar() - alphanumeric characters and
// underscores are considered part of words, everything else is a separator.
//
// Used for implementing Ctrl+Left/Right navigation and Ctrl+W word deletion.
func (p *Prompt) findWordBoundary(direction int) int {
	if direction > 0 {
		// Find next word start (Ctrl+Right)
		pos := p.cursor
		for pos < len(p.buffer) && !isWordChar(p.buffer[pos]) {
			pos++ // Skip non-word characters
		}
		for pos < len(p.buffer) && isWordChar(p.buffer[pos]) {
			pos++ // Skip word characters
		}
		return pos
	}
	// Find previous word start (Ctrl+Left)
	pos := p.cursor
	if pos > 0 {
		pos-- // Move back one position
	}
	for pos > 0 && !isWordChar(p.buffer[pos]) {
		pos-- // Skip non-word characters
	}
	for pos > 0 && isWordChar(p.buffer[pos-1]) {
		pos-- // Skip word characters
	}
	return pos
}

// isWordChar determines if a character is part of a word for navigation and editing operations.
//
// This function defines word boundaries for word-based navigation (Ctrl+Left/Right)
// and word deletion operations (Ctrl+W). The implementation follows common text
// editor conventions:
//
//   - Letters: Always considered part of a word
//   - Digits: Always considered part of a word
//   - Underscore (_): Considered part of a word (programming convention)
//   - All other characters: Considered word separators (spaces, punctuation, etc.)
//
// This character classification enables intuitive text navigation in programming
// contexts where identifiers commonly contain underscores.
//
// A letter is a letter in any script. Testing for a-z alone made every other
// alphabet a separator, so word navigation walked over a word written in
// Japanese as if it were whitespace and carried on into the word before it,
// and a letter with a diacritic split its own word in two.
//
// Used by findWordBoundary() for word-based cursor movement operations.
func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// newHistorySearcher returns the search function Ctrl+R calls for matches. It
// ranks the history by fuzzy match against the query, closest first.
func newHistorySearcher(history []string) func(string) []string {
	fm := &fuzzyMatcher{
		items: history,
	}
	return fm.searchFunc
}

// searchFunc returns items that match the query using fuzzy matching
func (f *fuzzyMatcher) searchFunc(query string) []string {
	if query == "" {
		return f.items
	}

	matches := f.fuzzySearch(query)
	// Convert to string slice
	results := make([]string, len(matches))
	for i, match := range matches {
		results[i] = match.text
	}
	return results
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
			return string(searchBuffer), nil

		case '\x03', '\x1b': // Ctrl+C or Escape - cancel search
			return "", nil

		case '\x7f', '\b': // Backspace
			if len(searchBuffer) > 0 {
				searchBuffer = searchBuffer[:len(searchBuffer)-1]
				searchResults = search(string(searchBuffer))
				selectedIndex = 0
			}

		case '\t': // Tab - next result
			if len(searchResults) > 0 {
				selectedIndex = (selectedIndex + 1) % len(searchResults)
			}

		default:
			if r >= 32 && r < 127 || r > 127 { // Printable characters
				searchBuffer = append(searchBuffer, r)
				searchResults = search(string(searchBuffer))
				selectedIndex = 0
			}
		}
	}
}

// renderHistorySearch renders the history search interface and returns how many
// terminal rows it occupies, which is what the next render has to erase.
func (p *Prompt) renderHistorySearch(query string, results []string, selected int) (int, error) {
	// Every line of the block is one terminal row, so what is drawn is flattened
	// to one: a history entry can hold newlines -- a statement entered across
	// several lines is stored as one entry, which is what the file's escaping is
	// for -- and drawing one raw took rows the block never counted, leaving them
	// on screen when the search closed.
	header := "reverse-i-search: " + singleLine(query)
	if selected < len(results) && len(results) > 0 {
		header += " -> " + singleLine(results[selected])
	}

	// Show top 5 results. The header names the selection even when Tab has
	// cycled past what is listed, so it is built before this cut.
	const maxResults = 5
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	lines := make([]string, 0, len(results)+1)
	lines = append(lines, header)
	for i, result := range results {
		if i == selected {
			lines = append(lines, "  > "+singleLine(result))
			continue
		}
		lines = append(lines, "    "+singleLine(result))
	}

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
	width := defaultTerminalWidth
	if p.terminal != nil {
		if w, _, err := p.terminal.Size(); err == nil && w > 0 {
			width = w
		}
	}

	rows := 0
	for _, line := range lines {
		wrapped, _ := layout(line, width)
		rows += wrapped + 1
	}
	return rows
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

// isShiftEnter detects if we should add a newline instead of submitting
func (p *Prompt) isShiftEnter() bool {
	currentLine := p.getCurrentLineText()

	// Check for backslash continuation - if present, add newline
	if strings.HasSuffix(strings.TrimRight(currentLine, " \t"), "\\") {
		// Remove the backslash and add newline for continuation
		p.removeTrailingBackslash()
		return true // Add newline for continuation
	}

	// If no backslash, Enter submits (both single-line and multiline modes)
	return false
}

// isMultiLine checks if the current buffer contains newline characters
func (p *Prompt) isMultiLine() bool {
	return slices.Contains(p.buffer, '\n')
}

// findLineStart finds the start of the current line
func (p *Prompt) findLineStart() int {
	return p.findLineBoundary(p.cursor, -1)
}

// findLineEnd finds the end of the current line
func (p *Prompt) findLineEnd() int {
	return p.findLineBoundary(p.cursor, 1)
}

// findLineBoundary finds the line boundary in the given direction
// direction < 0: finds line start, direction > 0: finds line end
func (p *Prompt) findLineBoundary(start int, direction int) int {
	pos := start
	if direction < 0 {
		// Find line start
		for pos > 0 && p.buffer[pos-1] != '\n' {
			pos--
		}
	} else {
		// Find line end
		for pos < len(p.buffer) && p.buffer[pos] != '\n' {
			pos++
		}
	}
	return pos
}

// findCursorUp moves cursor to the same column on the previous line
func (p *Prompt) findCursorUp() int {
	return p.findCursorVertical(-1)
}

// findCursorDown moves cursor to the same column on the next line
func (p *Prompt) findCursorDown() int {
	return p.findCursorVertical(1)
}

// findCursorVertical moves cursor vertically maintaining column position
// direction < 0: move up, direction > 0: move down
func (p *Prompt) findCursorVertical(direction int) int {
	lineStart := p.findLineStart()
	lineEnd := p.findLineEnd()
	column := p.cursor - lineStart

	if direction < 0 {
		// Move up
		if lineStart == 0 {
			return p.cursor // Already at first line
		}

		// Find start of previous line
		prevLineEnd := lineStart - 1 // Skip the newline
		prevLineStart := 0
		for i := prevLineEnd - 1; i >= 0; i-- {
			if p.buffer[i] == '\n' {
				prevLineStart = i + 1
				break
			}
		}

		// Calculate new cursor position
		prevLineLength := prevLineEnd - prevLineStart
		if column < prevLineLength {
			return prevLineStart + column
		}
		return prevLineEnd
	}

	// Move down
	if lineEnd >= len(p.buffer) {
		return p.cursor // Already at last line
	}

	// Find end of next line
	nextLineStart := lineEnd + 1 // Skip the newline
	nextLineEnd := len(p.buffer)
	for i := nextLineStart; i < len(p.buffer); i++ {
		if p.buffer[i] == '\n' {
			nextLineEnd = i
			break
		}
	}

	// Calculate new cursor position
	nextLineLength := nextLineEnd - nextLineStart
	if column < nextLineLength {
		return nextLineStart + column
	}
	return nextLineEnd
}

// getCurrentLineText returns the text of the current line where the cursor is positioned
func (p *Prompt) getCurrentLineText() string {
	lineStart := p.findLineStart()
	lineEnd := p.findLineEnd()
	return string(p.buffer[lineStart:lineEnd])
}

// removeTrailingBackslash removes the trailing backslash from the current line
func (p *Prompt) removeTrailingBackslash() {
	lineStart := p.findLineStart()
	line := p.buffer[lineStart:p.findLineEnd()]

	// The backslash is found by walking the runes rather than by measuring a
	// string. The buffer is a []rune and lineStart indexes it, so adding the byte
	// length of the line's text put the position three cells further along for
	// every multi-byte rune: the slice went past the end of the buffer and the
	// prompt panicked, or, when the buffer's capacity happened to reach that far,
	// deleted a rune that was not the backslash.
	end := len(line)
	for end > 0 && (line[end-1] == ' ' || line[end-1] == '\t') {
		end--
	}
	if end == 0 || line[end-1] != '\\' {
		return
	}

	backslashPos := lineStart + end - 1
	p.buffer = append(p.buffer[:backslashPos], p.buffer[backslashPos+1:]...)
	// The cursor takes the backslash's place, which is where the newline goes.
	p.cursor = backslashPos
}

// enterRawMode puts the terminal into raw mode and enables bracketed paste. It is
// idempotent: when the terminal is already in raw mode it does nothing, so a
// persistent session (see WithPersistentRawMode) acquires raw mode exactly once
// across many Run calls.
func (p *Prompt) enterRawMode() error {
	if p.rawActive {
		return nil
	}
	if err := p.terminal.SetRaw(); err != nil {
		return err
	}
	p.rawActive = true
	if p.output != nil {
		if _, err := fmt.Fprint(p.output, bracketedPasteEnableSequence); err != nil {
			return errors.Join(err, p.exitRawMode())
		}
	}
	return nil
}

// restoreOnExit restores the terminal from a return path that ends the session
// (interrupt or EOF), logging any failure. It is used so a persistent session is
// restored on those paths even though the deferred per-call cleanup skips restore.
func (p *Prompt) restoreOnExit() {
	if err := p.exitRawMode(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to restore terminal state: %v\n", err)
	}
}

// exitRawMode disables bracketed paste and restores the original terminal state.
// It is idempotent: when the terminal is not in raw mode it does nothing, so it is
// safe to call from multiple cleanup paths (defer, interrupt, EOF, Close).
func (p *Prompt) exitRawMode() error {
	if !p.rawActive {
		return nil
	}
	p.rawActive = false
	var errs []error
	if p.output != nil {
		if _, err := fmt.Fprint(p.output, bracketedPasteDisableSequence); err != nil {
			errs = append(errs, err)
		}
	}
	if err := p.terminal.Restore(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (p *Prompt) render() error {
	p.renderer.setContinuationPrefix(p.config.ContinuationPrefix)
	p.renderer.setHighlighter(p.config.Highlighter)
	return p.renderer.render(p.config.Prefix, string(p.buffer), p.cursor)
}

func (p *Prompt) renderWithSuggestionsOffset(suggestions []Suggestion, selected int, offset int) error {
	p.renderer.setContinuationPrefix(p.config.ContinuationPrefix)
	p.renderer.setHighlighter(p.config.Highlighter)
	return p.renderer.renderWithSuggestionsOffset(p.config.Prefix, string(p.buffer), p.cursor, suggestions, selected, offset)
}

func (p *Prompt) readRune() (rune, error) {
	if r, ok := p.takePending(); ok {
		return r, nil
	}
	// Once a watcher has started the shared reader, every rune must come from it:
	// a second reader on the same terminal would take bytes the other was waiting
	// for.
	if p.reads != nil {
		res, ok := <-p.reads
		if !ok {
			return 0, p.readErr
		}
		return res.r, nil
	}
	r, _, err := p.terminal.ReadRune()
	return r, err
}

// takePending removes and returns the oldest rune held back, if any.
func (p *Prompt) takePending() (rune, bool) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	if len(p.pending) == 0 {
		return 0, false
	}
	r := p.pending[0]
	p.pending = p.pending[1:]
	return r, true
}

// unreadRune pushes a rune back so the next readRune returns it. It is used when
// a rune read ahead turns out to be input rather than part of a key sequence, so
// it goes to the front: it was read before everything already held.
func (p *Prompt) unreadRune(r rune) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	p.pending = append([]rune{r}, p.pending...)
}

// stashTypeAhead holds a rune read while WatchInterrupt was watching, to be
// delivered to the next Run. It goes to the back, because it was typed after
// everything already held.
func (p *Prompt) stashTypeAhead(r rune) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	p.pending = append(p.pending, r)
}

// errReaderClosed is what a read reports once Close has ended the session.
var errReaderClosed = errors.New("prompt: input closed")

// startInputReader starts the goroutine that reads the terminal into a channel,
// and returns that channel. Reading in one place is what lets a watcher and the
// line editor take turns without either losing a keystroke to the other.
//
// The goroutine has two ways out, because it has two ways to wait. Blocked on the
// terminal it ends when the read fails, which closing the terminal causes.
// Blocked on the channel — nothing is consuming keystrokes between a stopped
// watch and the next Run, and someone can hold a key down — it ends on the signal
// Close sends, which no amount of closing the terminal would deliver. Either way
// the channel is closed and every later read reports why.
//
// It cannot be stopped while a read is in progress, which is why there is no
// pause: a terminal read cannot be canceled on every platform, and a goroutine
// abandoned mid-read would eat the next key.
func (p *Prompt) startInputReader() <-chan readResult {
	p.readerOnce.Do(func() {
		// Buffered so a burst typed during long work does not block the reader,
		// which would leave the terminal unread and lose the keys after it.
		reads := make(chan readResult, 1024)
		stop := make(chan struct{})
		done := make(chan struct{})
		p.reads = reads
		p.readerStop = stop
		p.readerDone = done
		go func() {
			defer close(done)
			for {
				r, _, err := p.terminal.ReadRune()
				if err != nil {
					p.readErr = err
					close(reads)
					return
				}
				select {
				case reads <- readResult{r: r}:
				case <-stop:
					p.readErr = errReaderClosed
					close(reads)
					return
				}
			}
		}()
	})
	return p.reads
}

// readInterrupter is implemented by a terminal whose Close ends a read in
// progress. Only such a terminal can be waited on: where a read cannot be
// interrupted, waiting for the reader to notice would hang Close forever.
type readInterrupter interface {
	interruptsReads() bool
}

// awaitReaderExit waits for the shared reader goroutine to finish, but only
// where closing the terminal is known to have ended the read it was in.
//
// Waiting matters because a reader that outlives Close is not idle. It is
// blocked on a descriptor the process has closed, and once that descriptor
// number is reused — running a child process is enough to cause that — the
// goroutine is reading whatever now holds it, taking input meant for something
// else. A prompt opened after one was closed received nothing at all, because
// the previous session's reader had the new session's terminal.
//
// Where the terminal cannot be interrupted the goroutine is left to end when
// its read returns, which is what happened before this and is still better than
// a Close that never returns.
func (p *Prompt) awaitReaderExit() {
	if p.readerDone == nil {
		return
	}
	if ri, ok := p.terminal.(readInterrupter); !ok || !ri.interruptsReads() {
		return
	}
	<-p.readerDone
}

// stopInputReader releases the shared reader, if one was started. A goroutine
// waiting to hand over a rune has no other way out: closing the terminal ends a
// read in progress, but says nothing to one blocked on the channel.
func (p *Prompt) stopInputReader() {
	if p.readerStop == nil {
		return
	}
	p.readerStopOnce.Do(func() { close(p.readerStop) })
}

// readEscapeSequence reads what follows ESC and returns the key sequence it
// forms, without the ESC. It returns "" when ESC introduced no sequence at all:
// a bare Escape key, or Alt+key, which every terminal sends as ESC followed by
// the plain character.
//
// Only CSI ("[") and SS3 ("O") introduce a sequence, so any other rune is input
// in its own right and is pushed back for the read loop. Consuming it swallowed
// whatever the user typed right after pressing Escape.
func (p *Prompt) readEscapeSequence() (string, error) {
	r, err := p.readRune()
	if err != nil {
		return "", err
	}
	if r != '[' && r != 'O' {
		p.unreadRune(r)
		return "", nil
	}

	// SS3 is ESC O plus exactly one final character (F1-F4, keypad keys).
	if r == 'O' {
		final, err := p.readRune()
		if err != nil {
			return "", err
		}
		return string([]rune{r, final}), nil
	}

	// CSI is ESC [ parameters intermediates final, and ends at the first byte in
	// the final range. Reading to that byte keeps a long sequence (bracketed
	// paste markers, Ctrl+arrow, a mouse or paste report with many parameters)
	// whole instead of cutting it after three runes.
	//
	// The grammar decides where the sequence ends, not a rune count. A count can
	// only stop reading, and a sequence the terminal is still sending does not
	// stop with it: everything past the count stayed in the input and reached
	// the read loop as keystrokes, so a long terminal reply appeared in the
	// user's line one parameter at a time. maxCSILength bounds what is
	// remembered instead, so a sequence too long to name is still consumed
	// whole.
	seq := make([]rune, 0, 8)
	seq = append(seq, r)
	overlong := false
	for {
		r, err := p.readRune()
		if err != nil {
			return "", err
		}
		switch {
		case r >= csiFinalFirst && r <= csiFinalLast:
			if overlong {
				// The terminal is sending something this prompt cannot name. It
				// has been read to its end, so nothing is left to be mistaken
				// for typing.
				return "", nil
			}
			return string(append(seq, r)), nil
		case r >= csiParamFirst && r <= csiParamLast:
			if len(seq) >= maxCSILength {
				overlong = true
				continue
			}
			seq = append(seq, r)
		default:
			// A byte outside the grammar aborts the sequence and is input in its
			// own right. Counting it as a parameter swallowed it: ESC [ followed
			// by Enter left the line unsubmittable.
			p.unreadRune(r)
			return "", nil
		}
	}
}
