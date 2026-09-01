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
	"sync/atomic"
	"unicode"

	"github.com/mattn/go-colorable"
)

// Windows OS name constant
const windowsOS = "windows"

// Common errors
var (
	// ErrEOF is returned when the user presses Ctrl+D on an empty line, and when
	// the input the prompt reads reaches its end. Both are the same thing to a
	// caller: there is no more input, so the session is over.
	//
	// It matches io.EOF as well as itself, so a REPL that breaks its loop on
	// either is right. The two used to be told apart -- Ctrl+D answered to io.EOF
	// and nothing else -- and a loop written the way this package documents it
	// therefore never ended.
	ErrEOF error = eofError{}
	// ErrInterrupted is returned when the user presses Ctrl+C
	ErrInterrupted = errors.New("interrupted")
)

// eofError is what ErrEOF is. It exists to wrap io.EOF without changing what
// ErrEOF prints or which value it is.
type eofError struct{}

func (eofError) Error() string { return "EOF" }

func (eofError) Unwrap() error { return io.EOF }

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
	// rawActive says whether the terminal is currently in raw mode. Close can be
	// called while a Run waits for a key, and both of them restore the terminal,
	// so each transition is made by whichever caller gets there first and the
	// other one does nothing.
	rawActive atomic.Bool

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

	// closed is set by Close and says the session is over. Every entry point
	// that would touch the terminal has to know that, because the terminal has
	// been given up while its settings live on: raw mode is set on a descriptor
	// Close never touches, so entering it again succeeds and nothing is left
	// that would restore it. It is read from another goroutine -- Close while a
	// Run waits for a key is a supported order -- so it is atomic.
	closed atomic.Bool
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

// WithCompleter sets the completion function.
//
// It is asked on Tab, and given the whole input and where the cursor is. What it
// answers stands for the word before that cursor, so the menu it opens lasts
// only as long as that word does: editing the line, discarding it, or moving the
// cursor off the word ends the completion, and the next Tab asks again. Tab,
// Enter, and Right accept the highlighted suggestion; Up and Down move through
// them; Escape closes the menu.
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
	return newFromConfigOn(config, terminal, output)
}

// newFromConfigOn builds a prompt over a terminal that is already open. It owns
// that terminal from here on: nothing that fails below hands the caller a
// prompt, so nothing would be left to close it, and the go-tty handle, the
// descriptor the reader polls, and the pipe that wakes it would all be leaked --
// three per attempt for a caller that retries.
//
// The close is deferred rather than written into each path so that a fallible
// step added later is covered too, which is how the history load came to leak.
func newFromConfigOn(config Config, terminal terminalInterface, output io.Writer) (_ *Prompt, err error) {
	defer func() {
		if err != nil {
			if cerr := terminal.Close(); cerr != nil {
				err = errors.Join(err, cerr)
			}
		}
	}()

	// Initialize history manager
	historyManager := newHistoryManager(config.HistoryConfig)

	// Load history from file if configured
	if err = historyManager.loadHistory(); err != nil {
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
	// A closed prompt has no input left to read, which is what ErrEOF says. This
	// is answered before the read rather than by it, because reaching the read
	// means entering raw mode first: doing that on a terminal the session has
	// given up leaves the terminal raw with nothing left to restore it.
	if p.closed.Load() {
		return "", ErrEOF
	}
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
	// pendingLine holds the line being typed while the history is walked. The
	// index past the newest entry is where that line belongs, and it has to be
	// kept somewhere: replacing the buffer with a history entry is what would
	// otherwise destroy it, and coming back had nothing to come back to.
	pendingLine := ""
	inPaste := false
	lastPasted := rune(0)
	var suggestions []Suggestion
	selectedSuggestion := 0
	suggestionOffset := 0 // Track the offset for scrolling through suggestions

	for {
		// Read key input
		r, err := p.readRuneContext(ctx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		if err != nil {
			if p.endOfInput(err) {
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
				if p.endOfInput(err) {
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
			// A suggestion stands for what the word before the cursor becomes.
			// The cursor has just moved, so it is no longer where that word was
			// measured: keeping the menu open let acceptSuggestion work the word
			// out again from the new position and insert part of the suggestion
			// into the middle of the one already there.
			suggestions = nil

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
					if historyIndex == len(p.history) {
						// Leaving the line being typed, rather than moving
						// between entries: this is the last chance to keep it.
						pendingLine = string(p.buffer)
					}
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
						// Back past the newest entry, which is the line that was
						// being typed. An edit made to a history entry along the
						// way is dropped rather than carried here, the way a
						// shell drops it.
						p.setBuffer(pendingLine)
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
			suggestions = nil

		case ActionMoveEnd:
			if p.isMultiLine() {
				p.cursor = p.findLineEnd()
			} else {
				p.cursor = len(p.buffer)
			}
			suggestions = nil

		case ActionMoveWordLeft:
			p.cursor = p.findWordBoundary(-1)
			suggestions = nil

		case ActionMoveWordRight:
			p.cursor = p.findWordBoundary(1)
			suggestions = nil

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
			// The line the menu was built for is gone. Leaving it open drew
			// completions under an empty prompt, and the next accept put the
			// discarded text back -- which is the opposite of what the key is for.
			suggestions = nil

		case ActionDeleteToEnd:
			if p.isMultiLine() {
				lineEnd := p.findLineEnd()
				p.buffer = append(p.buffer[:p.cursor], p.buffer[lineEnd:]...)
			} else {
				p.buffer = p.buffer[:p.cursor]
			}
			suggestions = nil

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
			// Whatever the search left on the line, it is not the line the menu
			// was built for.
			suggestions = nil
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
					// terminal even in persistent mode before returning. It is
					// the same ending as input running out, and says so with the
					// same error.
					p.restoreOnExit()
					return "", ErrEOF
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
// Close ends the session, so it is the last thing the prompt does with the
// terminal: on Unix it ends a read in progress and waits for the reader to
// finish, and a Run that was waiting for a key returns ErrEOF. A Run called
// afterwards returns ErrEOF without touching the terminal, rather than taking
// raw mode back on a terminal the session no longer owns. Open a new prompt for
// a new session.
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
	// Say the session is over before anything is torn down, so a Run waiting for
	// a key reports the ending it knows rather than the error the terminal gives
	// up with.
	p.closed.Store(true)

	// Release the shared reader before the terminal: closing the terminal ends a
	// read in progress, and this ends a goroutine waiting to hand over a rune
	// nobody is collecting.
	p.stopInputReader()

	// Restore raw mode if a persistent session (WithPersistentRawMode) left the
	// terminal in raw mode. Idempotent, so it is a no-op in the default mode.
	if err := p.exitRawMode(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to exit raw mode: %v\n", err)
	}

	// Restore cursor visibility before closing. It is done here as well as in
	// restoreOnExit because a prompt can be closed without ever having run.
	if p.output != nil {
		p.showCursor()
		fmt.Fprint(p.output, "\n") // Move to new line
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
	if p.historyManager != nil {
		// A disabled manager holds nothing, and getHistory says so.
		return p.historyManager.getHistory()
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
	// Emptying it is right whether or not there is a manager: with one, this
	// mirrors what the manager now holds; without one, this is the history.
	p.history = []string{}
}

// SetHistory replaces the entire history
func (p *Prompt) SetHistory(history []string) {
	if p.historyManager != nil {
		// A manager that is disabled holds nothing and is given nothing, the way
		// AddHistory already treats it. Falling through to the branch below --
		// which exists for a prompt with no manager at all, where p.history is
		// the only place a history can live -- put the entries in the list the
		// arrow keys walk while GetHistory reported none.
		if !p.historyManager.isEnabled() {
			return
		}
		p.historyManager.setHistory(history)
		p.history = p.historyManager.getHistory()
		return
	}

	p.history = append([]string{}, history...)
	// Trim history if it exceeds max size
	maxEntries := p.getMaxHistoryEntries()
	if len(p.history) > maxEntries {
		p.history = p.history[len(p.history)-maxEntries:]
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
	if !p.rawActive.CompareAndSwap(false, true) {
		return nil
	}
	if err := p.terminal.SetRaw(); err != nil {
		p.rawActive.Store(false)
		return err
	}
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
	// The completion menu hides the cursor while it is drawn and shows it again
	// on the next render without one, which an ending never reaches. Handing the
	// terminal back with no cursor outlives the program, so it is shown here,
	// where both endings that give the terminal up already pass. Showing a
	// cursor that is already visible does nothing.
	p.showCursor()
	if err := p.exitRawMode(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to restore terminal state: %v\n", err)
	}
}

// showCursor makes the terminal cursor visible again.
func (p *Prompt) showCursor() {
	if p.output == nil {
		return
	}
	fmt.Fprint(p.output, showCursorSequence)
}

// exitRawMode disables bracketed paste and restores the original terminal state.
// It is idempotent: when the terminal is not in raw mode it does nothing, so it is
// safe to call from multiple cleanup paths (defer, interrupt, EOF, Close).
func (p *Prompt) exitRawMode() error {
	if !p.rawActive.CompareAndSwap(true, false) {
		return nil
	}
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
