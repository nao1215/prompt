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
	"strings"
	"sync"
	"sync/atomic"

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

	// The watch, which is one goroutine per prompt however many watches are
	// active: watchers holds what each of them cancels, keyed so a watch can
	// take its own out again. A goroutine per watch meant a receiver per watch
	// on the shared reader's channel, and two receivers stash what they took in
	// whichever order the scheduler gives them, which is not the order it was
	// typed in. See WatchInterrupt.
	watchMu   sync.Mutex
	watchers  map[uint64]context.CancelFunc
	watchNext uint64
	watchStop chan struct{}
	watchDone chan struct{}

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
	// Enabled turns history on. A disabled history holds nothing: it is not
	// loaded, not added to, and not saved.
	Enabled bool
	// MaxEntries is how many entries the session keeps and the arrow keys walk.
	// Zero means 1000, because a history of no entries is not a setting anyone
	// wants.
	//
	// It bounds the file as well, since a save writes what the session holds.
	// What it leaves out is not deleted: a save that would write fewer entries
	// than the file already holds moves the file aside as a backup first.
	MaxEntries int
	// File is where the history is kept between sessions. Empty keeps it in
	// memory for the life of the process. The file is created readable by its
	// owner alone, because it holds what the user typed.
	File string
	// MaxFileSize is how many bytes of encoded entries a save writes before the
	// oldest are left out and the file is rotated. Zero means 1MB. The newest
	// entry is written whatever it costs, because a limit that saved nothing
	// would be worse than a file slightly over it.
	MaxFileSize int64
	// MaxBackups is how many generations of the history file are kept beside it,
	// as File.1 through File.MaxBackups.
	//
	// Zero keeps none, and is a setting rather than an omission: what the
	// history file holds is what the user typed, and an application may not want
	// copies of it left on disk. A HistoryConfig built with this field left out
	// therefore keeps no backups; passing no HistoryConfig at all is the case
	// that takes the defaults, backups included, and WithFileHistory asks for
	// three.
	MaxBackups int
}

// Config holds the configuration for a prompt.
type Config struct {
	// Prefix is drawn in front of the line being edited (for example "$ ").
	//
	// It is text. The color scheme colors it, so an escape sequence written
	// here is not a second color -- it is drawn as a space, one space per rune
	// the terminal would act on, because the prompt measures its own layout
	// from what it draws and a sequence the terminal obeys occupies no cells to
	// measure. A newline is a space for the same reason: the block's height is
	// counted from the input's line breaks, not the prefix's.
	Prefix        string
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
	// multiline entry is still being typed. See WithContinuationPrefix. It is
	// text on the same terms as Prefix.
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
// than the scheme's Input color, as rune offsets into that input. Everything no
// run covers keeps the scheme's color, and so does a run whose Color is the zero
// value, which is how a highlighter says it has nothing to say about that run.
// It is called on every render, which is once per keystroke, so it should be
// cheap over a line's worth of text.
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
	// Text is what accepting this suggestion puts on the line.
	//
	// It is text, whatever its source. A completer built from a file offers
	// whatever the file holds, so a rune the terminal would act on rather than
	// draw -- an escape sequence in a CSV header, say -- is drawn as a space,
	// both in the menu and on the line once the suggestion is accepted. The
	// buffer keeps it, so Run returns the string the completer offered.
	Text string
	// Description is drawn beside Text in the menu, on the same terms.
	Description string
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

// Suggest is what go-prompt calls a Suggestion. It is an alias, so a completer
// written against go-prompt keeps compiling.
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
	if config.HistoryConfig == nil {
		config.HistoryConfig = defaultHistoryConfig()
	} else {
		normalizeHistoryConfig(config.HistoryConfig)
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
//   - Ctrl+U: Delete the line the cursor is on
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
	pastedSinceDraw := 0
	var suggestions []Suggestion
	selectedSuggestion := 0
	suggestionOffset := 0 // Track the offset for scrolling through suggestions

	// How the history is walked, in one place: the index into it, the line being
	// typed (kept when the index leaves it so the way forward has something to
	// come back to), and the completion menu, which stands for a line that is
	// about to be replaced. The arrow keys walk it on a single-line entry, and a
	// key bound to ActionHistoryUp or ActionHistoryDown walks it whatever the
	// entry looks like -- on a multiline one the arrows are moving the cursor
	// between lines, which is what those actions exist for.
	historyBack := func() {
		if historyIndex <= 0 {
			return
		}
		if historyIndex == len(p.history) {
			// Leaving the line being typed, rather than moving between entries:
			// this is the last chance to keep it.
			pendingLine = string(p.buffer)
		}
		historyIndex--
		p.setBuffer(p.history[historyIndex])
		suggestions = nil
	}
	historyForward := func() {
		if historyIndex >= len(p.history) {
			return
		}
		historyIndex++
		if historyIndex == len(p.history) {
			// Back past the newest entry, which is the line that was being
			// typed. An edit made to a history entry along the way is dropped
			// rather than carried here, the way a shell drops it.
			p.setBuffer(pendingLine)
		} else {
			p.setBuffer(p.history[historyIndex])
		}
		suggestions = nil
	}

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
					pastedSinceDraw++
				}
				suggestions = nil
				action = ActionNone
				// Drawn on the same terms as the rest of the paste. This branch
				// falls through to the render at the foot of the loop, so
				// content that arrives as sequences -- copied terminal output
				// has one every few characters -- was costing a redraw each.
				if pastedSinceDraw < pasteDrawInterval {
					continue
				}
				pastedSinceDraw = 0
			}
		case inPaste:
			// Pasted content is data, not keystrokes: it goes into the buffer as
			// written instead of running completion (TAB), ending the prompt
			// (Ctrl+C), or submitting (Enter).
			lastPasted = p.insertPastedRune(r, lastPasted)
			suggestions = nil
			// Drawn on its way in, but not once per character. Every render
			// draws the block whole, so a paste of n characters cost n copies of
			// a block that was growing: a twenty-thousand-character statement
			// wrote fifty megabytes of escape sequences and took seconds, which
			// from the outside is a shell that has hung. The end of the paste
			// redraws it either way -- ActionPasteEnd falls through to the
			// render at the foot of this loop -- so what is left to do here is
			// show that something is arriving.
			pastedSinceDraw++
			if pastedSinceDraw >= pasteDrawInterval {
				pastedSinceDraw = 0
				if err := p.renderWithSuggestionsOffset(nil, 0, 0); err != nil {
					return "", fmt.Errorf("failed to render: %w", err)
				}
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
					p.endEntry()
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
			p.endEntry()
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
				historyBack()
			}

		case ActionMoveDown:
			if len(suggestions) > 0 {
				if selectedSuggestion < len(suggestions)-1 {
					selectedSuggestion++
					suggestionOffset = p.scrollToSelection(suggestions, selectedSuggestion, suggestionOffset)
				}
			} else if p.isMultiLine() {
				// Navigate down within multi-line input
				p.cursor = p.findCursorDown()
			} else {
				historyForward()
			}

		case ActionHistoryUp:
			historyBack()

		case ActionHistoryDown:
			historyForward()

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
			// The line the cursor is on, which on an entry of one line is the
			// whole of it. Emptying the buffer whatever the entry looked like
			// meant the key that says "delete the line" discarded a statement
			// typed across several, and there is no undo. Ctrl+K beside it has
			// asked which line it is on since multiline entries existed, and the
			// key for discarding a whole entry is Ctrl+C, which says so on
			// screen.
			lineStart, lineEnd := p.findLineStart(), p.findLineEnd()
			p.buffer = append(p.buffer[:lineStart], p.buffer[lineEnd:]...)
			p.cursor = lineStart
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
			result, searchErr := p.searchHistory(ctx)
			// A canceled context ends the call rather than the search: the
			// caller asked for the prompt to stop, and going back to the loop
			// would draw a prompt nobody is going to read a key for.
			if errors.Is(searchErr, context.Canceled) || errors.Is(searchErr, context.DeadlineExceeded) {
				return "", searchErr
			}
			if searchErr == nil && result != "" {
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
			pastedSinceDraw = 0

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

	// The watch ends with the session: it holds the interrupt away from its
	// default action, and a watch nobody stopped would keep it there for the
	// rest of the process.
	p.endWatching()

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
		// The session ends below whatever is on screen. A Close that ends a Run
		// in progress -- which is how a session is ended from another goroutine
		// -- leaves the caret wherever the last render put it, and that is the
		// block's last row only when the cursor was on the entry's last line;
		// the line break alone then put the shell's next prompt into the middle
		// of the entry, with the rest of it below and nothing left to redraw
		// over it.
		if p.renderer != nil {
			p.renderer.moveToBlockFoot()
		}
		// Carriage return as well as line feed: the terminal is out of raw mode
		// by now and would translate the one into the other, but a restore that
		// failed leaves it raw, where a line feed alone keeps the column.
		fmt.Fprint(p.output, "\r\n")
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
	// A nil scheme means the default one, which is what New makes of it. The
	// renderer reads colors off the scheme on every render without checking, so
	// writing a nil one through panicked on the next keystroke -- the same value
	// that is accepted at construction.
	if theme == nil {
		theme = ThemeDefault
	}
	p.config.ColorScheme = theme
	p.config.Theme = theme
	// The scheme changes on the renderer that is there rather than by replacing
	// it. What a renderer holds is what it knows about the screen -- how tall
	// the block on it is, which row the caret is on -- and a new one knows none
	// of that, so a theme changed between one keystroke and the next would have
	// left the next redraw erasing from the wrong row.
	if p.renderer != nil {
		p.renderer.setColorScheme(theme)
	}
}

// SetPrefix changes the prompt prefix, which takes effect on the next render.
// It is text on the terms Config.Prefix describes.
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

// endEntry redraws the entry with the caret after its last character, which
// leaves the cursor at the foot of the block. What is written next -- the
// application's output, the ^C of an interrupt, the next prompt -- then starts
// below the entry instead of on top of the rows the cursor was above.
//
// It is a redraw rather than a move down by the rows between the cursor and the
// foot, because a terminal clamps a move down at its last row: a block that
// fills the screen would end the line inside itself. The redraw also brings the
// end of a block taller than the terminal onto the screen, which is where the
// output belongs.
//
// A render that fails changes nothing here. The line the user typed is worth
// more than a tidy screen, and it is returned either way.
// pasteDrawInterval is how many pasted characters go into the buffer between
// redraws. It is a compromise between two costs that are both the terminal's: a
// redraw per character, and a long paste with nothing on screen while it
// arrives. A few hundred is under a tenth of a second of typing at any speed a
// person reaches, and a few percent of the drawing a redraw per character did.
const pasteDrawInterval = 512

func (p *Prompt) endEntry() {
	p.cursor = len(p.buffer)
	_ = p.render() //nolint:errcheck // the line, or the interrupt, is returned whether or not the screen took the redraw
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
