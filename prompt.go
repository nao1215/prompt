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

	"github.com/mattn/go-colorable"
)

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
	historyManager *HistoryManager
	buffer         []rune
	cursor         int
	renderer       *renderer
	terminal       terminalInterface
	keyMap         *KeyMap
}

// KeyBinding represents a keyboard shortcut mapping
type KeyBinding struct {
	Key    rune   // Key character (for simple keys)
	Seq    string // Escape sequence (for special keys like arrows)
	Action KeyAction
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

// Suggestion represents a completion suggestion.
type Suggestion struct {
	Text        string // The text to complete
	Description string // Description of the suggestion
}

// Suggest is an alias for Suggestion for compatibility
type Suggest = Suggestion

// Document represents the current input state for completers
type Document struct {
	Text           string // The entire input text
	CursorPosition int    // Current cursor position in the text
}

// TextBeforeCursor returns the text before the cursor
func (d *Document) TextBeforeCursor() string {
	if d.CursorPosition < 0 || d.CursorPosition > len(d.Text) {
		return d.Text
	}
	return d.Text[:d.CursorPosition]
}

// TextAfterCursor returns the text after the cursor
func (d *Document) TextAfterCursor() string {
	if d.CursorPosition < 0 || d.CursorPosition >= len(d.Text) {
		return ""
	}
	return d.Text[d.CursorPosition:]
}

// GetWordBeforeCursor returns the word before the cursor
func (d *Document) GetWordBeforeCursor() string {
	text := d.TextBeforeCursor()
	if len(text) == 0 {
		return ""
	}

	// If cursor is right after a whitespace character, return empty string
	if text[len(text)-1] == ' ' || text[len(text)-1] == '\t' || text[len(text)-1] == '\n' {
		return ""
	}

	// Find the start of the current word by scanning backwards
	start := len(text) - 1
	for start >= 0 && text[start] != ' ' && text[start] != '\t' && text[start] != '\n' {
		start--
	}
	start++ // Move to the first character of the word

	return text[start:]
}

// CurrentLine returns the current line
func (d *Document) CurrentLine() string {
	return d.Text
}

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
		config.HistoryConfig = DefaultHistoryConfig()
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
	if runtime.GOOS == "windows" {
		// Use colorable for Windows ANSI color support
		output = colorable.NewColorableStdout()
	}

	// Create terminal interface using external libraries
	terminal, err := newRealTerminal()
	if err != nil {
		return nil, fmt.Errorf("failed to create terminal: %w", err)
	}

	// Initialize history manager
	historyManager := NewHistoryManager(config.HistoryConfig)

	// Load history from file if configured
	if err := historyManager.LoadHistory(); err != nil {
		return nil, fmt.Errorf("failed to load history: %w", err)
	}

	// History manager is ready with either loaded history or empty history

	// Initialize prompt
	p := &Prompt{
		config:         config,
		output:         output,
		history:        historyManager.GetHistory(),
		historyManager: historyManager,
		terminal:       terminal,
		keyMap:         config.KeyMap,
	}

	// Initialize renderer
	p.renderer = newRenderer(output, config.ColorScheme)

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
// The prompt can be cancelled via the provided context, allowing for timeouts
// or cancellation from other goroutines. The function supports all configured
// key bindings, multi-line input, completion, and history navigation.
//
// Supported key bindings include:
//   - Enter: Submit input (or add newline in multi-line mode)
//   - Ctrl+C: Cancel and return ErrInterrupted
//   - Ctrl+D: EOF when buffer is empty
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

	restored := false
	defer func() {
		// Only restore if not already restored (prevents double restoration)
		if !restored {
			if err := p.exitRawMode(); err != nil {
				// Log error but don't return it as we're in defer
				fmt.Fprintf(os.Stderr, "Warning: failed to exit raw mode: %v\n", err)
			}
		}
	}()

	// Initialize buffer and display
	p.buffer = []rune{}
	p.cursor = 0
	if err := p.render(); err != nil {
		return "", fmt.Errorf("failed to render prompt: %w", err)
	}

	historyIndex := len(p.history)
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
				return "", ErrEOF
			}
			return "", fmt.Errorf("failed to read input: %w", err)
		}

		var action KeyAction

		// Handle escape sequences
		if r == '\x1b' {
			seq, err := p.readEscapeSequence()
			if err != nil {
				continue
			}
			action = p.keyMap.GetSequenceAction(seq)
		} else {
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
				// Proceed with normal submit only if no suggestions were displayed
				// Check if this is a multi-line context that should add newline instead
				if p.isShiftEnter() {
					p.insertRune('\n')
					suggestions = nil
				} else {
					result := string(p.buffer)
					if result != "" && (len(p.history) == 0 || p.history[len(p.history)-1] != result) {
						p.addToHistory(result)
					}
					fmt.Fprint(p.output, "\r\n")
					// Terminal will be restored by defer, no need to mark as restored here
					return result, nil
				}
			}

		case ActionCancel:
			// Ensure terminal state is properly restored before returning
			if err := p.exitRawMode(); err != nil {
				// Log warning but continue with interrupt handling
				fmt.Fprintf(os.Stderr, "Warning: failed to restore terminal state: %v\n", err)
			}
			restored = true // Mark as restored to prevent double restoration in defer
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

					// Smart matching: filter suggestions based on current input
					currentWord := doc.GetWordBeforeCursor()
					if currentWord != "" {
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
			p.insertRune('\n')
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
	// Restore cursor visibility before closing
	if p.output != nil {
		fmt.Fprint(p.output, "\x1b[?25h") // Show cursor
		fmt.Fprint(p.output, "\n")        // Move to new line
	}

	// Save history before closing
	if p.historyManager != nil {
		if err := p.historyManager.SaveHistory(); err != nil {
			// Log error but continue with cleanup
			fmt.Fprintf(os.Stderr, "Warning: failed to save history: %v\n", err)
		}
	}

	// Close terminal resources to prevent file descriptor leaks
	if p.terminal != nil {
		return p.terminal.Close()
	}
	return nil
}

// Helper methods

func (p *Prompt) insertRune(r rune) {
	p.buffer = append(p.buffer[:p.cursor], append([]rune{r}, p.buffer[p.cursor:]...)...)
	p.cursor++
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

func (p *Prompt) acceptSuggestion(suggestion Suggestion) {
	// Get current document state for context
	doc := Document{
		Text:           string(p.buffer),
		CursorPosition: p.cursor,
	}

	// Determine how to apply the suggestion based on context
	beforeCursor := doc.TextBeforeCursor()
	currentWord := doc.GetWordBeforeCursor()

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
	if p.historyManager != nil && p.historyManager.IsEnabled() {
		return p.historyManager.GetHistory()
	}
	if p.historyManager != nil && !p.historyManager.IsEnabled() {
		return []string{} // Return empty when disabled
	}
	return append([]string{}, p.history...)
}

// AddHistory adds a command to the history
func (p *Prompt) AddHistory(command string) {
	if command == "" {
		return
	}
	if p.historyManager != nil && p.historyManager.IsEnabled() {
		p.historyManager.AddEntry(command)
		p.syncHistoryAfterAdd()
	} else if p.historyManager != nil && !p.historyManager.IsEnabled() {
		// Do nothing when history is explicitly disabled
		return
	} else {
		// Fallback to in-memory only (when no history manager)
		if len(p.history) > 0 && p.history[len(p.history)-1] == command {
			return
		}
		p.history = append(p.history, command)
		maxEntries := 1000 // Default max entries
		if p.config.HistoryConfig != nil && p.config.HistoryConfig.MaxEntries > 0 {
			maxEntries = p.config.HistoryConfig.MaxEntries
		}
		if len(p.history) > maxEntries {
			p.history = p.history[len(p.history)-maxEntries:]
		}
	}
}

// ClearHistory clears the command history
func (p *Prompt) ClearHistory() {
	if p.historyManager != nil && p.historyManager.IsEnabled() {
		p.historyManager.ClearHistory()
	}
	p.history = []string{}
}

// SetHistory replaces the entire history
func (p *Prompt) SetHistory(history []string) {
	if p.historyManager != nil && p.historyManager.IsEnabled() {
		p.historyManager.SetHistory(history)
		p.history = p.historyManager.GetHistory()
	} else {
		p.history = append([]string{}, history...)
	}
	// Trim history if it exceeds max size
	maxEntries := 1000 // Default max entries
	if p.config.HistoryConfig != nil && p.config.HistoryConfig.MaxEntries > 0 {
		maxEntries = p.config.HistoryConfig.MaxEntries
	}
	if len(p.history) > maxEntries {
		p.history = p.history[len(p.history)-maxEntries:]
		if p.historyManager != nil && p.historyManager.IsEnabled() {
			p.historyManager.SetHistory(p.history)
		}
	}
}

// Configuration update methods

// SetTheme changes the color theme of the prompt
func (p *Prompt) SetTheme(theme *ColorScheme) {
	p.config.ColorScheme = theme
	p.config.Theme = theme
	p.renderer = newRenderer(p.output, theme)
}

// SetPrefix changes the prompt prefix
func (p *Prompt) SetPrefix(prefix string) {
	p.config.Prefix = prefix
}

// SetCompleter changes the completion function
func (p *Prompt) SetCompleter(completer func(Document) []Suggestion) {
	p.config.Completer = completer
}

// fuzzyCompleter provides fuzzy matching for completions (internal implementation)
type fuzzyCompleter struct {
	candidates []string
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
	fc := &fuzzyCompleter{
		candidates: candidates,
	}
	return fc.Complete
}

// Complete returns fuzzy-matched suggestions for the given document context.
//
// The method performs fuzzy string matching against all candidates and returns
// a sorted list of suggestions. Results are ranked by match quality:
//   - Exact matches get the highest score (1000)
//   - Prefix matches get high scores (800+)
//   - Substring matches get medium scores (500+)
//   - Character-by-character fuzzy matches get lower scores
//
// If input is empty, all candidates are returned. The suggestions include
// score information in the Description field for debugging purposes.
//
// Example usage:
//
//	completer := prompt.NewFuzzyCompleter([]string{"git status", "git commit"})
//	doc := prompt.Document{Text: "git st", CursorPosition: 6}
//	suggestions := completer.Complete(doc)
//	// Returns: [{"git status", "score: 850"}, ...]
func (f *fuzzyCompleter) Complete(d Document) []Suggestion {
	input := d.TextBeforeCursor()
	if input == "" {
		// Return all candidates if no input
		suggestions := make([]Suggestion, len(f.candidates))
		for i, candidate := range f.candidates {
			suggestions[i] = Suggestion{
				Text:        candidate,
				Description: "",
			}
		}
		return suggestions
	}

	var matches []fuzzyMatch
	inputLower := strings.ToLower(input)

	for _, candidate := range f.candidates {
		if score := calculateFuzzyScore(inputLower, strings.ToLower(candidate), false); score > 0 {
			matches = append(matches, fuzzyMatch{
				text:  candidate,
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
//   - Letters (a-z, A-Z): Always considered part of a word
//   - Digits (0-9): Always considered part of a word
//   - Underscore (_): Considered part of a word (programming convention)
//   - All other characters: Considered word separators (spaces, punctuation, etc.)
//
// This character classification enables intuitive text navigation in programming
// contexts where identifiers commonly contain underscores.
//
// Used by findWordBoundary() for word-based cursor movement operations.
func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// historySearcher provides fuzzy search through command history (internal implementation)
type historySearcher struct {
	history []string
}

// NewHistorySearcher creates a new history searcher for command history.
//
// The history searcher provides fuzzy search capabilities through command
// history, similar to reverse-i-search in bash (Ctrl+R). This is primarily
// used internally by the prompt for history search functionality.
//
// This function returns a search function that can be used to find commands
// in the provided history that match a given query using fuzzy matching.
//
// Example:
//
//	history := []string{
//		"git status",
//		"git commit -m 'fix bug'",
//		"docker run -it ubuntu",
//		"kubectl get pods",
//	}
//
//	search := prompt.NewHistorySearcher(history)
//	matches := search("git")
//	// Returns: ["git commit -m 'fix bug'", "git status"] (sorted by relevance)
func NewHistorySearcher(history []string) func(string) []string {
	hs := &historySearcher{
		history: history,
	}
	return hs.Search
}

// Search returns commands from history that match the query using fuzzy matching.
//
// The search uses the same fuzzy matching algorithm as the fuzzyCompleter,
// ranking results by relevance. Exact matches, prefix matches, and substring
// matches are prioritized over character-by-character fuzzy matches.
//
// If the query is empty, the entire history is returned in original order.
// Results are sorted by match score in descending order (best matches first).
//
// Example:
//
//	searcher := prompt.NewHistorySearcher([]string{
//		"git status",
//		"git commit -m 'initial'",
//		"docker ps",
//	})
//
//	// Search for commands containing "git"
//	matches := searcher.Search("git")
//	// Returns: ["git commit -m 'initial'", "git status"]
//
//	// Search for commands containing "st"
//	matches = searcher.Search("st")
//	// Returns: ["git status"] (fuzzy match)
func (h *historySearcher) Search(query string) []string {
	if query == "" {
		return h.history
	}

	var matches []fuzzyMatch
	queryLower := strings.ToLower(query)

	for _, command := range h.history {
		if score := calculateFuzzyScore(queryLower, strings.ToLower(command), false); score > 0 {
			matches = append(matches, fuzzyMatch{
				text:  command,
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

	// Convert to string slice
	results := make([]string, len(matches))
	for i, match := range matches {
		results[i] = match.text
	}

	return results
}

// searchHistory implements reverse history search (like Ctrl+R in bash)
func (p *Prompt) searchHistory() (string, error) {
	search := NewHistorySearcher(p.history)
	searchBuffer := []rune{}
	searchResults := search("")
	selectedIndex := 0

	for {
		// Render search interface
		p.renderHistorySearch(string(searchBuffer), searchResults, selectedIndex)

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

// renderHistorySearch renders the history search interface
func (p *Prompt) renderHistorySearch(query string, results []string, selected int) {
	// Clear screen
	fmt.Fprint(p.output, "\r\x1b[K")

	// Show search prompt
	fmt.Fprintf(p.output, "reverse-i-search: %s", query)

	// Show selected result if any
	if selected < len(results) && len(results) > 0 {
		fmt.Fprintf(p.output, " -> %s", results[selected])
	}

	fmt.Fprint(p.output, "\r\n")

	// Show top 5 results
	maxResults := 5
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	for i, result := range results {
		if i == selected {
			fmt.Fprintf(p.output, "  > %s\r\n", result)
		} else {
			fmt.Fprintf(p.output, "    %s\r\n", result)
		}
	}
}

// syncHistoryAfterAdd synchronizes in-memory history with history manager after adding an entry.
// This consolidates the common logic shared between AddHistory and addToHistory.
func (p *Prompt) syncHistoryAfterAdd() {
	if p.historyManager != nil && p.historyManager.IsEnabled() {
		p.history = p.historyManager.GetHistory()
		// Trim in-memory history if it exceeds max size
		maxEntries := 1000 // Default max entries
		if p.config.HistoryConfig != nil && p.config.HistoryConfig.MaxEntries > 0 {
			maxEntries = p.config.HistoryConfig.MaxEntries
		}
		if len(p.history) > maxEntries {
			p.history = p.history[len(p.history)-maxEntries:]
			p.historyManager.SetHistory(p.history)
		}
	}
}

func (p *Prompt) addToHistory(text string) {
	if text == "" {
		return
	}
	if p.historyManager != nil && p.historyManager.IsEnabled() {
		p.historyManager.AddEntry(text)
		p.syncHistoryAfterAdd()
	} else if p.historyManager == nil {
		// Fallback to original behavior when no history manager
		if len(p.history) > 0 && p.history[len(p.history)-1] == text {
			return // Avoid duplicate consecutive entries
		}
		p.history = append(p.history, text)
		maxEntries := 1000 // Default max entries
		if p.config.HistoryConfig != nil && p.config.HistoryConfig.MaxEntries > 0 {
			maxEntries = p.config.HistoryConfig.MaxEntries
		}
		if len(p.history) > maxEntries {
			p.history = p.history[len(p.history)-maxEntries:]
		}
	}
	// Do nothing when history manager exists but is disabled
}

// isShiftEnter detects if Shift+Enter was pressed for multi-line input
func (p *Prompt) isShiftEnter() bool {
	// For now, we'll use a simple heuristic: if the buffer already contains
	// newlines, treat Enter as adding a newline. For a complete implementation,
	// we'd need to track modifier keys.
	return p.isMultiLine()
}

// isMultiLine checks if the current buffer contains newline characters
func (p *Prompt) isMultiLine() bool {
	return slices.Contains(p.buffer, '\n')
}

// findLineStart finds the start of the current line
func (p *Prompt) findLineStart() int {
	pos := p.cursor
	for pos > 0 && p.buffer[pos-1] != '\n' {
		pos--
	}
	return pos
}

// findLineEnd finds the end of the current line
func (p *Prompt) findLineEnd() int {
	pos := p.cursor
	for pos < len(p.buffer) && p.buffer[pos] != '\n' {
		pos++
	}
	return pos
}

// findCursorUp moves cursor to the same column on the previous line
func (p *Prompt) findCursorUp() int {
	lineStart := p.findLineStart()
	if lineStart == 0 {
		return p.cursor // Already at first line
	}

	// Find column position in current line
	column := p.cursor - lineStart

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

// findCursorDown moves cursor to the same column on the next line
func (p *Prompt) findCursorDown() int {
	lineStart := p.findLineStart()
	lineEnd := p.findLineEnd()

	if lineEnd >= len(p.buffer) {
		return p.cursor // Already at last line
	}

	// Find column position in current line
	column := p.cursor - lineStart

	// Find start of next line
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

func (p *Prompt) enterRawMode() error {
	return p.terminal.SetRaw()
}

func (p *Prompt) exitRawMode() error {
	return p.terminal.Restore()
}

func (p *Prompt) render() error {
	return p.renderer.render(p.config.Prefix, string(p.buffer), p.cursor)
}

func (p *Prompt) renderWithSuggestionsOffset(suggestions []Suggestion, selected int, offset int) error {
	return p.renderer.renderWithSuggestionsOffset(p.config.Prefix, string(p.buffer), p.cursor, suggestions, selected, offset)
}

func (p *Prompt) readRune() (rune, error) {
	r, _, err := p.terminal.ReadRune()
	return r, err
}

func (p *Prompt) readEscapeSequence() (string, error) {
	seq := make([]rune, 0, 10) // Pre-allocate with capacity
	for range 10 {             // Limit to prevent infinite loop
		r, err := p.readRune()
		if err != nil {
			return "", err
		}
		seq = append(seq, r)

		// Check for complete sequences
		s := string(seq)
		if s == "[A" || s == "[B" || s == "[C" || s == "[D" || s == "[H" || s == "[F" {
			return s, nil
		}
		if strings.HasSuffix(s, "~") && len(s) >= 3 {
			return s, nil
		}
		if len(seq) >= 3 && (seq[len(seq)-1] < '0' || seq[len(seq)-1] > '9') {
			return s, nil
		}
	}
	return string(seq), nil
}
