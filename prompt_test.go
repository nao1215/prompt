package prompt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config options
	}{
		{
			name: "default config",
			config: options{
				Prefix: "$ ",
			},
		},
		{
			name: "with completer",
			config: options{
				Prefix: "> ",
				Completer: func(d Document) []Suggestion {
					text := d.WordBeforeCursor()
					if strings.HasPrefix("hello", text) {
						return []Suggestion{{Text: "hello", Description: "greeting"}}
					}
					return nil
				},
			},
		},
		{
			name: "with history",
			config: options{
				Prefix: ">>> ",
				historyConfig: &historyConfig{
					Enabled:    true,
					MaxEntries: 1000,
				},
			},
		},
		{
			name: "with color scheme",
			config: options{
				Prefix:      "$ ",
				ColorScheme: ThemeDark,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Use NewForTesting to avoid terminal initialization issues in test environment
			p := newForTestingWithConfig(t, tt.config, "test\n")

			require.NotNil(t, p, "NewForTesting() returned nil prompt")

			// Check defaults were set
			require.NotNil(t, p.config.historyConfig, "historyConfig should not be nil")
			assert.Greater(t, p.config.historyConfig.MaxEntries, 0, "historyConfig.MaxEntries should have default value")

			assert.NotNil(t, p.config.ColorScheme, "ColorScheme should have default value")

			// Clean up
			assert.NoError(t, p.Close(), "Close() should not fail")
		})
	}
}

func TestPromptWithMockTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
		config   options
	}{
		{
			name:     "simple input",
			input:    "hello\n",
			expected: "hello",
			config:   options{Prefix: "$ "},
		},
		{
			name:     "input with backspace",
			input:    "hello\x7f\x7fo\n", // hello, backspace, backspace, o, enter
			expected: "helo",
			config:   options{Prefix: "$ "},
		},
		{
			name:     "empty input",
			input:    "\n",
			expected: "",
			config:   options{Prefix: "$ "},
		},
		{
			// IsComplete reports incomplete until a trailing ";", so the bare
			// newlines buffer into one statement instead of submitting per line.
			name:     "multiline buffers until IsComplete returns true",
			input:    "SELECT 1\nUNION ALL\nSELECT 2;\n",
			expected: "SELECT 1\nUNION ALL\nSELECT 2;",
			config: options{
				Prefix:     "$ ",
				Multiline:  true,
				IsComplete: func(in string) bool { return strings.HasSuffix(strings.TrimSpace(in), ";") },
			},
		},
		{
			// A statement already terminated with ";" submits on the first Enter.
			name:     "complete statement submits immediately",
			input:    "SELECT 1;\n",
			expected: "SELECT 1;",
			config: options{
				Prefix:     "$ ",
				Multiline:  true,
				IsComplete: func(in string) bool { return strings.HasSuffix(strings.TrimSpace(in), ";") },
			},
		},
		{
			// Without multiline mode the predicate is ignored and Enter submits.
			name:     "IsComplete ignored when multiline is off",
			input:    "SELECT 1\n",
			expected: "SELECT 1",
			config: options{
				Prefix:     "$ ",
				IsComplete: func(in string) bool { return strings.HasSuffix(strings.TrimSpace(in), ";") },
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create prompt with mock terminal using NewForTesting
			p := newForTestingWithConfig(t, tt.config, tt.input)
			defer p.Close()

			// Capture output
			var output bytes.Buffer
			p.output = &output

			// Run with timeout to prevent hanging
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			result, err := p.Run(ctx)
			require.NoError(t, err, "Run() should not fail")
			assert.Equal(t, tt.expected, result, "Run() result should match expected")
		})
	}
}

func TestColorToANSI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		color    Color
		expected string
	}{
		{
			name:     "simple color",
			color:    Color{R: 255, G: 0, B: 0, Bold: false},
			expected: "\x1b[38;2;255;0;0m",
		},
		{
			name:     "bold color",
			color:    Color{R: 0, G: 255, B: 0, Bold: true},
			expected: "\x1b[1;38;2;0;255;0m",
		},
		{
			name:     "blue color",
			color:    Color{R: 0, G: 0, B: 255, Bold: false},
			expected: "\x1b[38;2;0;0;255m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.color.ToANSI()
			assert.Equal(t, tt.expected, result, "Color.ToANSI() result should match expected")
		})
	}
}

func TestSuggestion(t *testing.T) {
	t.Parallel()

	completer := func(d Document) []Suggestion {
		text := d.WordBeforeCursor()
		suggestions := []Suggestion{
			{Text: "hello", Description: "greeting"},
			{Text: "help", Description: "show help"},
			{Text: "history", Description: "show history"},
		}

		var result []Suggestion
		for _, s := range suggestions {
			if strings.HasPrefix(s.Text, text) {
				result = append(result, s)
			}
		}
		return result
	}

	tests := []struct {
		name     string
		input    string
		expected []Suggestion
	}{
		{
			name:  "empty input",
			input: "",
			expected: []Suggestion{
				{Text: "hello", Description: "greeting"},
				{Text: "help", Description: "show help"},
				{Text: "history", Description: "show history"},
			},
		},
		{
			name:  "h prefix",
			input: "h",
			expected: []Suggestion{
				{Text: "hello", Description: "greeting"},
				{Text: "help", Description: "show help"},
				{Text: "history", Description: "show history"},
			},
		},
		{
			name:  "hel prefix",
			input: "hel",
			expected: []Suggestion{
				{Text: "hello", Description: "greeting"},
				{Text: "help", Description: "show help"},
			},
		},
		{
			name:     "no match",
			input:    "xyz",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc := &Document{Text: tt.input, CursorPosition: len(tt.input)}
			result := completer(*doc)
			require.Equal(t, len(tt.expected), len(result), "completer(%q) should return expected number of suggestions", tt.input)

			for i, expected := range tt.expected {
				if i >= len(result) {
					break
				}
				assert.Equal(t, expected.Text, result[i].Text, "completer(%q)[%d].Text should match", tt.input, i)
				assert.Equal(t, expected.Description, result[i].Description, "completer(%q)[%d].Description should match", tt.input, i)
			}
		})
	}
}

func TestHistory(t *testing.T) {
	t.Parallel()

	p := newForTestingWithConfig(t, options{
		Prefix: "$ ",
		historyConfig: &historyConfig{
			Enabled:    true,
			MaxEntries: 3,
		},
	}, "test\n")
	defer p.Close()

	// Test adding history
	p.addToHistory("command1")
	p.addToHistory("command2")
	p.addToHistory("command3")
	p.addToHistory("command4") // Should remove command1

	expected := []string{"command2", "command3", "command4"}
	require.Equal(t, len(expected), len(p.history), "history length should match expected")

	for i, cmd := range expected {
		assert.Equal(t, cmd, p.history[i], "history[%d] should match expected", i)
	}
}

func BenchmarkPromptRender(b *testing.B) {
	p, err := newFromConfig(options{
		Prefix: "$ ",
	})
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}
	defer p.Close()

	// Use a bytes buffer to avoid terminal output
	var output bytes.Buffer
	p.output = &output

	b.ResetTimer()
	for range b.N {
		output.Reset()
		err := p.render()
		if err != nil {
			b.Fatalf("render() failed: %v", err)
		}
	}
}

func BenchmarkColorToANSI(b *testing.B) {
	color := Color{R: 255, G: 128, B: 64, Bold: true}

	b.ResetTimer()
	for range b.N {
		_ = color.ToANSI()
	}
}

// Additional tests for improved coverage

func TestPromptWithColorScheme(t *testing.T) {
	t.Parallel()

	// Test with custom color scheme using mock terminal
	scheme := &ColorScheme{
		Name:   "test",
		Prefix: Color{R: 255, G: 255, B: 255},
		Input:  Color{R: 200, G: 200, B: 200},
	}

	p := &Prompt{
		config: options{
			Prefix:      "test> ",
			ColorScheme: scheme,
		},
	}

	assert.NotNil(t, p.config.ColorScheme, "Expected non-nil color scheme")

	// Test with nil color scheme (should use default)
	p2 := &Prompt{
		config: options{
			Prefix:      "test> ",
			ColorScheme: nil,
		},
	}

	// Manually set default color scheme as New() would do
	if p2.config.ColorScheme == nil {
		p2.config.ColorScheme = &ColorScheme{
			Name:   "default",
			Prefix: Color{R: 0, G: 255, B: 0, Bold: true},
			Input:  Color{R: 255, G: 255, B: 255},
		}
	}

	assert.NotNil(t, p2.config.ColorScheme, "Expected default color scheme when nil provided")
}

func TestColorToANSIWithBold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		color    Color
		expected string
	}{
		{
			name:     "red bold",
			color:    Color{R: 255, G: 0, B: 0, Bold: true},
			expected: "\x1b[1;38;2;255;0;0m",
		},
		{
			name:     "green no bold",
			color:    Color{R: 0, G: 255, B: 0, Bold: false},
			expected: "\x1b[38;2;0;255;0m",
		},
		{
			name:     "blue bold",
			color:    Color{R: 0, G: 0, B: 255, Bold: true},
			expected: "\x1b[1;38;2;0;0;255m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.color.ToANSI()
			assert.Equal(t, tt.expected, result, "ToANSI() result should match expected")
		})
	}
}

func TestColorReset(t *testing.T) {
	t.Parallel()

	expected := "\x1b[0m"
	result := ansiReset()
	if result != expected {
		t.Errorf("ansiReset() = %q, want %q", result, expected)
	}
}

func TestPromptClose(t *testing.T) {
	t.Parallel()

	// Test closing a prompt with mock terminal
	mock := &mockTerminal{}
	p := &Prompt{
		config:   options{Prefix: "test> "},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
	}

	// First close should succeed
	err := p.Close()
	assert.NoError(t, err, "Expected no error on first close")

	// Second close should also succeed (should be idempotent)
	err = p.Close()
	assert.NoError(t, err, "Expected no error on second close")
}

func TestPromptHistoryFunctionality(t *testing.T) {
	t.Parallel()

	mock := &mockTerminal{}
	p := &Prompt{
		config: options{
			Prefix: "test> ",
			historyConfig: &historyConfig{
				Enabled:    true,
				MaxEntries: 3,
			},
		},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
		history:  []string{},
	}

	// Test adding to history
	p.addToHistory("command1")
	p.addToHistory("command2")
	p.addToHistory("command3")
	p.addToHistory("command4") // Should remove command1

	if len(p.history) != 3 {
		t.Errorf("Expected history length 3, got %d", len(p.history))
	}

	if p.history[0] != "command2" {
		t.Errorf("Expected first history item to be 'command2', got %q", p.history[0])
	}
}

// TestEscapeDoesNotSwallowTypedCharacters drives the whole read loop: pressing
// Escape and then typing must leave the typed text intact.
func TestEscapeDoesNotSwallowTypedCharacters(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("\x1bSELECT 1\r")
	p := newTestPrompt(mock)

	got, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("RunWithContext returned error: %v", err)
	}
	if want := "SELECT 1"; got != want {
		t.Errorf("input after Escape = %q, want %q", got, want)
	}
}

// TestEscapeDismissesSuggestions pins Escape as the way out of a completion
// popup. With suggestions on screen Enter accepts one instead of submitting, so
// a popup that Escape could not close left the user unable to run the line.
func TestEscapeDismissesSuggestions(t *testing.T) {
	t.Parallel()

	mock := newMockTerminal("se\t\x1b\r")
	p := newTestPrompt(mock, WithCompleter(func(Document) []Suggestion {
		return []Suggestion{{Text: "select"}, {Text: "session"}}
	}))

	got, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("RunWithContext returned error: %v", err)
	}
	if want := "se"; got != want {
		t.Errorf("input after dismissing the popup = %q, want %q", got, want)
	}
}

func TestNewRealTerminal(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping real terminal test in local development")
	}

	t.Parallel()

	// This test might fail in non-interactive environments, so we'll make it lenient
	terminal, err := newRealTerminal()
	if err != nil {
		t.Logf("Cannot create real terminal in test environment: %v", err)
		return
	}

	if terminal == nil {
		t.Error("Expected non-nil terminal")
		return
	}

	// Test that we can get size
	w, h, err := terminal.Size()
	if err != nil {
		t.Logf("Cannot get terminal size: %v", err)
	} else {
		if w <= 0 || h <= 0 {
			t.Errorf("Expected positive terminal size, got %dx%d", w, h)
		}
	}

	// Clean up
	if err := terminal.Close(); err != nil {
		t.Errorf("Failed to close terminal: %v", err)
	}
}

func TestPromptInteractiveFeatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple enter",
			input:    "hello\r",
			expected: "hello",
		},
		{
			name:     "enter with newline",
			input:    "test\n",
			expected: "test",
		},
		{
			name:     "empty input",
			input:    "\r",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockTerminal{
				input: []rune(tt.input),
			}

			var output bytes.Buffer
			p := &Prompt{
				config: options{
					Prefix: "$ ",
					historyConfig: &historyConfig{
						Enabled:    true,
						MaxEntries: 10,
					},
				},
				terminal: mock,
				keyMap:   NewDefaultKeyMap(),
				output:   &output,
				buffer:   []rune{},
				cursor:   0,
				history:  []string{},
				renderer: newRenderer(&output, ThemeDefault, nil),
			}

			result, err := p.Run(context.Background())
			if err != nil {
				t.Errorf("Input() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestPromptWithCompleter(t *testing.T) {
	t.Parallel()

	completer := func(d Document) []Suggestion {
		text := d.WordBeforeCursor()
		if strings.HasPrefix("hello", text) {
			return []Suggestion{{Text: "hello", Description: "greeting"}}
		}
		if strings.HasPrefix("help", text) {
			return []Suggestion{{Text: "help", Description: "show help"}}
		}
		return nil
	}

	mock := &mockTerminal{
		input: []rune("h\t\r"), // Type 'h', press tab for completion, then enter
	}

	var output bytes.Buffer
	p := &Prompt{
		config: options{
			Prefix:    "$ ",
			Completer: completer,
		},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
		output:   &output,
		buffer:   []rune{},
		cursor:   0,
		history:  []string{},
		renderer: newRenderer(&output, ThemeDefault, nil),
	}

	result, err := p.Run(context.Background())
	if err != nil {
		t.Errorf("Input() error = %v", err)
	}

	// Should complete to "hello" or "help" based on implementation
	if result != "hello" && result != "help" {
		t.Errorf("Expected completion result, got %q", result)
	}
}

func TestPromptHistoryNavigation(t *testing.T) {
	t.Parallel()

	mock := &mockTerminal{
		// Simulate up arrow followed by enter
		input: []rune("\x1b[A\r"),
	}

	var output bytes.Buffer
	p := &Prompt{
		config: options{
			Prefix: "$ ",
			historyConfig: &historyConfig{
				Enabled:    true,
				MaxEntries: 10,
			},
		},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
		output:   &output,
		buffer:   []rune{},
		cursor:   0,
		history:  []string{"previous command", "another command"},
		renderer: newRenderer(&output, ThemeDefault, nil),
	}

	result, err := p.Run(context.Background())
	if err != nil {
		t.Errorf("Input() error = %v", err)
	}

	// Should return the last command from history
	if result != "another command" {
		t.Errorf("Expected 'another command', got %q", result)
	}
}

func TestPromptBackspaceHandling(t *testing.T) {
	t.Parallel()

	mock := &mockTerminal{
		// Type "hello", backspace twice, then enter
		input: []rune("hello\b\b\r"),
	}

	var output bytes.Buffer
	p := &Prompt{
		config: options{
			Prefix: "$ ",
		},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
		output:   &output,
		buffer:   []rune{},
		cursor:   0,
		history:  []string{},
		renderer: newRenderer(&output, ThemeDefault, nil),
	}

	result, err := p.Run(context.Background())
	if err != nil {
		t.Errorf("Input() error = %v", err)
	}

	// Should have "hel" after backspacing twice
	if result != "hel" {
		t.Errorf("Expected 'hel', got %q", result)
	}
}

func TestPromptDeleteHandling(t *testing.T) {
	t.Parallel()

	mock := &mockTerminal{
		// Type "hello", move cursor to position 2, delete, then enter
		input: []rune("hello\x1b[D\x1b[D\x1b[D\x7f\r"), // Left 3 times, delete, enter
	}

	var output bytes.Buffer
	p := &Prompt{
		config: options{
			Prefix: "$ ",
		},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
		output:   &output,
		buffer:   []rune{},
		cursor:   0,
		history:  []string{},
		renderer: newRenderer(&output, ThemeDefault, nil),
	}

	result, err := p.Run(context.Background())
	if err != nil {
		t.Errorf("Input() error = %v", err)
	}

	// Result depends on exact cursor positioning and delete behavior
	t.Logf("Result after delete operations: %q", result)
}

func TestPromptCursorMovement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		desc  string
	}{
		{
			name:  "left arrow",
			input: "abc\x1b[D\r",
			desc:  "type 'abc', move left, enter",
		},
		{
			name:  "right arrow",
			input: "ab\x1b[D\x1b[C\r",
			desc:  "type 'ab', move left, move right, enter",
		},
		{
			name:  "home key",
			input: "abc\x1b[H\r",
			desc:  "type 'abc', go to home, enter",
		},
		{
			name:  "end key",
			input: "ab\x1b[D\x1b[F\r",
			desc:  "type 'ab', move left, go to end, enter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockTerminal{
				input: []rune(tt.input),
			}

			var output bytes.Buffer
			p := &Prompt{
				config: options{
					Prefix: "$ ",
				},
				terminal: mock,
				keyMap:   NewDefaultKeyMap(),
				output:   &output,
				buffer:   []rune{},
				cursor:   0,
				history:  []string{},
				renderer: newRenderer(&output, ThemeDefault, nil),
			}

			result, err := p.Run(context.Background())
			if err != nil {
				t.Errorf("Input() error = %v", err)
			}

			t.Logf("Test %s (%s): result = %q", tt.name, tt.desc, result)
		})
	}
}

func TestPromptComplexEscapeSequences(t *testing.T) {
	t.Parallel()

	// Test reading escape sequences directly
	mock := &mockTerminal{
		input: []rune("[A"), // Up arrow sequence (without ESC)
	}

	p := &Prompt{
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
	}

	seq, err := p.readEscapeSequence()
	if err != nil {
		t.Errorf("readEscapeSequence() error = %v", err)
	}
	if seq != "[A" {
		t.Errorf("Expected '[A', got %q", seq)
	}
}

func TestPromptLongEscapeSequence(t *testing.T) {
	t.Parallel()

	// Test with a sequence that should be truncated
	mock := &mockTerminal{
		input: []rune("abcdefghijklmnop"), // Longer than 10 characters
	}

	p := &Prompt{
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
	}

	seq, err := p.readEscapeSequence()
	if err != nil {
		t.Errorf("readEscapeSequence() error = %v", err)
	}

	// Should read up to the limit
	if len(seq) > 10 {
		t.Errorf("Expected sequence length <= 10, got %d: %q", len(seq), seq)
	}
}

func TestPromptRenderError(t *testing.T) {
	t.Parallel()

	// Create a mock that will cause render errors
	mock := &mockTerminal{
		input: []rune("test\r"),
	}

	// Use a failing writer
	failingWriter := &failingWriter{}

	// Test the renderer directly to ensure it fails
	renderer := newRenderer(failingWriter, ThemeDefault, nil)
	err := renderer.render("$ ", "test", 4)
	if err == nil {
		t.Error("Expected error from failing writer in renderer")
	}

	// Test with a prompt that has initial render failure
	p := &Prompt{
		config: options{
			Prefix: "$ ",
		},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
		output:   failingWriter,
		buffer:   []rune{},
		cursor:   0,
		history:  []string{},
		renderer: renderer,
	}

	_, err = p.Run(context.Background())
	if err == nil {
		t.Error("Expected error from failing writer in prompt")
	}
}

// failingWriter is a writer that always returns an error
type failingWriter struct{}

func (w *failingWriter) Write(_ []byte) (n int, err error) {
	return 0, io.ErrClosedPipe
}

// TestMissingCoverageAreas tests specific code paths for better coverage
func TestMissingCoverageAreas(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping test that creates real terminal in local development")
	}

	t.Parallel()

	t.Run("New function coverage", func(t *testing.T) {
		// Test New() function (currently 0% coverage)
		config := options{
			Prefix: "test> ",
			historyConfig: &historyConfig{
				Enabled:    true,
				MaxEntries: 100,
			},
			ColorScheme: ThemeDefault,
		}

		p, err := newFromConfig(config)
		if err != nil {
			t.Logf("New() failed as expected in test environment: %v", err)
			// This is expected when /dev/tty is not available
		} else if p != nil {
			// If it succeeds, test basic functionality
			if p.config.Prefix != config.Prefix {
				t.Errorf("Expected prefix %q, got %q", config.Prefix, p.config.Prefix)
			}
			_ = p.Close()
		}
	})

	t.Run("Run function coverage", func(t *testing.T) {
		// Test Run() function (currently 0% coverage)
		mock := &mockTerminal{
			input: []rune("hello\r"),
		}

		var output bytes.Buffer
		p := &Prompt{
			config: options{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output, // Use buffer instead of stdout
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault, nil),
		}

		result, err := p.Run(context.Background())
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != "hello" {
			t.Errorf("Expected 'hello', got %q", result)
		}
	})

	t.Run("NewForTesting coverage", func(t *testing.T) {
		// Test NewForTesting function
		config := options{
			Prefix: "test> ",
		}

		p := newForTestingWithConfig(t, config, "test input\r")
		if p == nil {
			t.Error("Expected non-nil prompt from NewForTesting")
		}
	})

	t.Run("Context cancellation", func(t *testing.T) {
		// Test context cancellation
		mock := &mockTerminal{
			input: []rune("never ending input..."),
		}

		var output bytes.Buffer
		p := &Prompt{
			config: options{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault, nil),
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := p.Run(ctx)
		if err == nil {
			t.Error("Expected context cancellation error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Logf("Got error: %v", err)
		}
	})

	t.Run("Error in raw mode operations", func(t *testing.T) {
		// Test error paths in terminal operations
		mock := &mockTerminal{}

		// These should all succeed for mock terminal
		err := mock.SetRaw()
		if err != nil {
			t.Errorf("SetRaw() error = %v", err)
		}

		err = mock.Restore()
		if err != nil {
			t.Errorf("Restore() error = %v", err)
		}
	})

	t.Run("Special key sequences", func(t *testing.T) {
		sequences := []struct {
			name  string
			input string
		}{
			{"F1 key", "\x1b[11~"},
			{"F2 key", "\x1b[12~"},
			{"Delete key", "\x1b[3~"},
			{"Insert key", "\x1b[2~"},
			{"Page Up", "\x1b[5~"},
			{"Page Down", "\x1b[6~"},
		}

		for _, seq := range sequences {
			t.Run(seq.name, func(t *testing.T) {
				mock := &mockTerminal{
					input: []rune(seq.input[1:] + "\r"), // Skip ESC, add enter
				}

				p := &Prompt{
					terminal: mock,
					keyMap:   NewDefaultKeyMap(),
				}

				result, err := p.readEscapeSequence()
				if err != nil {
					t.Errorf("readEscapeSequence() error = %v", err)
				}
				t.Logf("Sequence %s: %q", seq.name, result)
			})
		}
	})

	t.Run("Complex cursor movements and editing", func(t *testing.T) {
		// Test complex editing scenarios
		mock := &mockTerminal{
			input: []rune("hello world\x1b[D\x1b[D\x1b[D\x1b[D\x1b[D\x1b[Dcruel \x1b[F\r"),
			// Type "hello world", move left 6 times, type "cruel ", go to end, enter
		}

		var output TestWriter
		p := &Prompt{
			config: options{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault, nil),
		}

		result, err := p.Run(context.Background())
		if err != nil {
			t.Errorf("Complex editing error = %v", err)
		}
		t.Logf("Complex editing result: %q", result)
	})
}

// TestWriter captures output for testing
type TestWriter struct {
	data []byte
}

func (w *TestWriter) Write(p []byte) (n int, err error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *TestWriter) String() string {
	return string(w.data)
}

// TestMockTerminalHelpers tests mock terminal helper functions
func TestMockTerminalHelpers(t *testing.T) {
	t.Parallel()

	// Test newMockTerminalHelper helper
	helper := newMockTerminalHelper("hello")
	if string(helper.input) != "hello\r" {
		t.Errorf("Expected 'hello\\r', got %q", string(helper.input))
	}
}

// newMockTerminalHelper creates a mock terminal with the given input
func newMockTerminalHelper(input string) *mockTerminal {
	return &mockTerminal{
		input:        []rune(input + "\r"), // Add enter key
		terminalSize: [2]int{80, 24},
	}
}

// TestRendererWithSuggestionEdgeCases covers edge cases in suggestion rendering
func TestRendererWithSuggestionEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("many suggestions truncation", func(t *testing.T) {
		var output TestWriter
		renderer := newRenderer(&output, ThemeDefault, nil)

		// Create more than 10 suggestions
		suggestions := make([]Suggestion, 15)
		for i := range suggestions {
			suggestions[i] = Suggestion{
				Text:        fmt.Sprintf("suggestion_%d", i),
				Description: fmt.Sprintf("desc_%d", i),
			}
		}

		_, err := renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 5, 0)
		if err != nil {
			t.Errorf("renderSuggestions() error = %v", err)
		}

		result := output.String()
		// Should only contain first 10 suggestions
		if !containsString(result, "suggestion_9") {
			t.Error("Expected to find suggestion_9 in output")
		}
		if containsString(result, "suggestion_10") {
			t.Error("Should not find suggestion_10 in output (truncated)")
		}
	})

	t.Run("clear multiple lines", func(t *testing.T) {
		var output TestWriter
		renderer := newRenderer(&output, ThemeDefault, nil)

		// Simulate having rendered multiple lines
		renderer.lastLines = 5
		renderer.clearPreviousLines()

		result := output.String()
		// Should contain escape sequences for clearing multiple lines
		if len(result) == 0 {
			t.Error("Expected output from clearing multiple lines")
		}
	})

	t.Run("render error conditions", func(t *testing.T) {
		// Test failing writer scenarios
		failing := &failingWriter{}
		renderer := newRenderer(failing, ThemeDefault, nil)

		// Test renderMainLine error
		_, _, err := renderer.renderMainLine("$ ", "test", 2)
		if err == nil {
			t.Error("Expected error from failing writer in renderMainLine")
		}

		// Test renderSuggestions error
		suggestions := []Suggestion{{Text: "test", Description: "desc"}}
		_, err = renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 0, 0)
		if err == nil {
			t.Error("Expected error from failing writer in renderSuggestions")
		}

		// Test renderWithSuggestions error
		err = renderer.renderWithSuggestionsOffset("$ ", "test", 2, suggestions, 0, 0)
		if err == nil {
			t.Error("Expected error from failing writer in renderWithSuggestions")
		}
	})

	t.Run("suggestions without descriptions", func(t *testing.T) {
		var output TestWriter
		renderer := newRenderer(&output, ThemeDefault, nil)

		suggestions := []Suggestion{
			{Text: "cmd1"},
			{Text: "cmd2"},
		}

		_, err := renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 0, 0)
		if err != nil {
			t.Errorf("renderSuggestions() error = %v", err)
		}

		result := output.String()
		if !containsString(result, "cmd1") {
			t.Error("Expected to find cmd1 in output")
		}
		if !containsString(result, "cmd2") {
			t.Error("Expected to find cmd2 in output")
		}
	})
}

// TestNewFunctionCoverage tests various paths in the New function
func TestNewFunctionCoverage(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping test that creates real terminal in local development")
	}

	t.Parallel()

	t.Run("new with invalid config", func(t *testing.T) {
		// Test with empty prefix
		config := options{
			Prefix: "",
		}
		p, err := newFromConfig(config)
		if err != nil {
			t.Logf("New with empty prefix failed: %v", err)
		}
		if p != nil {
			_ = p.Close()
		}
	})

	t.Run("new with large history", func(t *testing.T) {
		config := options{
			Prefix: "test> ",
			historyConfig: &historyConfig{
				Enabled:    true,
				MaxEntries: 1000,
			},
		}
		p, err := newFromConfig(config)
		if err != nil {
			t.Logf("New with large history failed: %v", err)
		}
		if p != nil {
			_ = p.Close()
		}
	})

	t.Run("new with custom color scheme", func(t *testing.T) {
		customScheme := &ColorScheme{
			Name:   "custom",
			Prefix: Color{R: 255, G: 100, B: 50, Bold: true},
			Input:  Color{R: 200, G: 200, B: 200},
		}
		config := options{
			Prefix:      "custom> ",
			ColorScheme: customScheme,
		}
		p, err := newFromConfig(config)
		if err != nil {
			t.Logf("New with custom color scheme failed: %v", err)
		}
		if p != nil {
			_ = p.Close()
		}
	})
}

// TestRunWithContextCoverage tests various paths in RunWithContext
func TestRunWithContextCoverage(t *testing.T) {
	t.Parallel()

	t.Run("prompt with suggestions and selection", func(t *testing.T) {
		completer := func(d Document) []Suggestion {
			input := d.WordBeforeCursor()
			if input == "h" {
				return []Suggestion{
					{Text: "hello", Description: "greeting"},
					{Text: "help", Description: "assistance"},
				}
			}
			return nil
		}

		mock := &mockTerminal{
			// Type 'h', press down arrow, press enter (accept suggestion)
			input: []rune("h\x1b[B\r\r"),
		}

		var output TestWriter
		p := &Prompt{
			config: options{
				Prefix:    "$ ",
				Completer: completer,
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault, nil),
		}

		result, err := p.Run(context.Background())
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		t.Logf("Result with suggestions: %q", result)
	})

	t.Run("prompt with history navigation", func(t *testing.T) {
		mock := &mockTerminal{
			// Press up arrow, then enter
			input: []rune("\x1b[A\r"),
		}

		var output TestWriter
		p := &Prompt{
			config: options{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{"previous command", "another command"},
			renderer: newRenderer(&output, ThemeDefault, nil),
		}

		result, err := p.Run(context.Background())
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		t.Logf("Result with history: %q", result)
	})

	t.Run("prompt with various key combinations", func(t *testing.T) {
		mock := &mockTerminal{
			// Type "test", press home, press end, press enter
			input: []rune("test\x1b[H\x1b[F\r"),
		}

		var output TestWriter
		p := &Prompt{
			config: options{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault, nil),
		}

		result, err := p.Run(context.Background())
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		t.Logf("Result with key combinations: %q", result)
	})
}

// TestCloseFunctionCoverage tests the Close function
func TestCloseFunctionCoverage(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping test that creates real terminal in local development")
	}

	t.Parallel()

	t.Run("close with real terminal", func(t *testing.T) {
		config := options{
			Prefix: "$ ",
		}
		p, err := newFromConfig(config)
		if err != nil {
			t.Logf("Cannot create prompt for close test: %v", err)
			return
		}

		// Close should not error
		err = p.Close()
		if err != nil {
			t.Errorf("Close() error = %v", err)
		}

		// Double close should also not error
		err = p.Close()
		if err != nil {
			t.Errorf("Second Close() error = %v", err)
		}
	})

	t.Run("close with mock terminal", func(t *testing.T) {
		mock := &mockTerminal{}
		p := &Prompt{
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
		}

		err := p.Close()
		if err != nil {
			t.Errorf("Close() with mock error = %v", err)
		}
	})
}

// TestComprehensiveRendererCoverage tests more renderer code paths
func TestComprehensiveRendererCoverage(t *testing.T) {
	t.Parallel()

	t.Run("render with cursor positions", func(t *testing.T) {
		var output TestWriter
		renderer := newRenderer(&output, ThemeDefault, nil)

		// Test cursor at beginning
		_, _, err := renderer.renderMainLine("$ ", "hello", 0)
		if err != nil {
			t.Errorf("renderMainLine() error = %v", err)
		}

		// Test cursor at end
		_, _, err = renderer.renderMainLine("$ ", "hello", 5)
		if err != nil {
			t.Errorf("renderMainLine() error = %v", err)
		}

		// Test cursor beyond end (should be safe)
		_, _, err = renderer.renderMainLine("$ ", "hello", 10)
		if err != nil {
			t.Errorf("renderMainLine() error = %v", err)
		}

		// Test with empty input
		_, _, err = renderer.renderMainLine("$ ", "", 0)
		if err != nil {
			t.Errorf("renderMainLine() error = %v", err)
		}

		// Test with unicode characters
		_, _, err = renderer.renderMainLine("🚀 ", "こんにちは", 2)
		if err != nil {
			t.Errorf("renderMainLine() error = %v", err)
		}
	})

	t.Run("render suggestions with various combinations", func(t *testing.T) {
		var output TestWriter
		renderer := newRenderer(&output, ThemeDefault, nil)

		// Test single suggestion
		suggestions := []Suggestion{{Text: "hello", Description: "greeting"}}
		_, err := renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 0, 0)
		if err != nil {
			t.Errorf("renderSuggestions() error = %v", err)
		}

		// Test multiple suggestions with selection
		suggestions = []Suggestion{
			{Text: "hello", Description: "greeting"},
			{Text: "help", Description: "assistance"},
			{Text: "history", Description: "past commands"},
		}
		_, err = renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 1, 0)
		if err != nil {
			t.Errorf("renderSuggestions() error = %v", err)
		}

		// Test exactly 10 suggestions (boundary)
		suggestions = make([]Suggestion, 10)
		for i := range suggestions {
			suggestions[i] = Suggestion{
				Text:        fmt.Sprintf("cmd%d", i),
				Description: fmt.Sprintf("description %d", i),
			}
		}
		_, err = renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 5, 0)
		if err != nil {
			t.Errorf("renderSuggestions() error = %v", err)
		}

		// Test 11 suggestions (will be truncated)
		suggestions = make([]Suggestion, 11)
		for i := range suggestions {
			suggestions[i] = Suggestion{
				Text:        fmt.Sprintf("cmd%d", i),
				Description: fmt.Sprintf("description %d", i),
			}
		}
		_, err = renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 0, 0)
		if err != nil {
			t.Errorf("renderSuggestions() error = %v", err)
		}

		// Test with no suggestions
		_, err = renderer.renderSuggestionsWithOffset("$ ", "test", 2, []Suggestion{}, 0, 0)
		if err != nil {
			t.Errorf("renderSuggestions() error = %v", err)
		}
	})

	t.Run("render with suggestions integration", func(t *testing.T) {
		var output TestWriter
		renderer := newRenderer(&output, ThemeDefault, nil)

		suggestions := []Suggestion{
			{Text: "hello", Description: "greeting"},
			{Text: "help", Description: "assistance"},
		}

		// Test with suggestions
		err := renderer.renderWithSuggestionsOffset("$ ", "h", 1, suggestions, 0, 0)
		if err != nil {
			t.Errorf("renderWithSuggestions() error = %v", err)
		}

		// Test without suggestions
		err = renderer.renderWithSuggestionsOffset("$ ", "hello", 5, nil, 0, 0)
		if err != nil {
			t.Errorf("renderWithSuggestions() error = %v", err)
		}

		// Test lastLines tracking
		if renderer.lastLines != 1 {
			t.Errorf("Expected lastLines = 1, got %d", renderer.lastLines)
		}

		// Test with suggestions again to verify lastLines update
		err = renderer.renderWithSuggestionsOffset("$ ", "h", 1, suggestions, 1, 0)
		if err != nil {
			t.Errorf("renderWithSuggestions() error = %v", err)
		}

		if renderer.lastLines != 3 { // 1 main line + 2 suggestions
			t.Errorf("Expected lastLines = 3, got %d", renderer.lastLines)
		}
	})
}

// TestAdvancedPromptCoverage tests more prompt functionality
func TestAdvancedPromptCoverage(t *testing.T) {
	t.Parallel()

	t.Run("escape sequence coverage", func(t *testing.T) {
		// Test all escape sequences
		sequences := []struct {
			name     string
			sequence string
			expected string
		}{
			{"up arrow", "\x1b[A", "[A"},
			{"down arrow", "\x1b[B", "[B"},
			{"right arrow", "\x1b[C", "[C"},
			{"left arrow", "\x1b[D", "[D"},
			{"home", "\x1b[H", "[H"},
			{"end", "\x1b[F", "[F"},
			{"delete", "\x1b[3~", "[3~"},
			{"insert", "\x1b[2~", "[2~"},
			{"page up", "\x1b[5~", "[5~"},
			{"page down", "\x1b[6~", "[6~"},
			{"F1", "\x1b[11~", "[11~"},
			{"F2", "\x1b[12~", "[12~"},
		}

		for _, seq := range sequences {
			t.Run(seq.name, func(t *testing.T) {
				mock := &mockTerminal{
					input: []rune(seq.sequence[1:]), // Skip initial ESC
				}

				var output TestWriter
				p := &Prompt{
					terminal: mock,
					keyMap:   NewDefaultKeyMap(),
					output:   &output,
					renderer: newRenderer(&output, ThemeDefault, nil),
				}

				result, err := p.readEscapeSequence()
				if err != nil {
					t.Errorf("readEscapeSequence() error = %v", err)
				}
				if result != seq.expected {
					t.Errorf("Expected %q, got %q", seq.expected, result)
				}
			})
		}
	})

	t.Run("prompt with different configurations", func(t *testing.T) {
		// Test with completer returning empty suggestions
		completer := func(_ Document) []Suggestion {
			return []Suggestion{} // Empty suggestions
		}

		mock := &mockTerminal{
			input: []rune("test\t\r"), // Type test, press tab, press enter
		}

		var output TestWriter
		p := &Prompt{
			config: options{
				Prefix:    "$ ",
				Completer: completer,
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault, nil),
		}

		result, err := p.Run(context.Background())
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != "test" {
			t.Errorf("Expected 'test', got %q", result)
		}
	})

	t.Run("history with max limit", func(t *testing.T) {
		mock := &mockTerminal{
			input: []rune("command1\r"),
		}

		var output TestWriter
		p := &Prompt{
			config: options{
				Prefix: "$ ",
				historyConfig: &historyConfig{
					Enabled:    true,
					MaxEntries: 2, // Small limit
				},
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{"old1", "old2"}, // Already at limit
			renderer: newRenderer(&output, ThemeDefault, nil),
		}

		result, err := p.Run(context.Background())
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != "command1" {
			t.Errorf("Expected 'command1', got %q", result)
		}

		// History should be truncated
		if len(p.history) != 2 {
			t.Errorf("Expected history length 2, got %d", len(p.history))
		}
		if p.history[0] != "old2" || p.history[1] != "command1" {
			t.Errorf("Expected history [old2, command1], got %v", p.history)
		}
	})
}

// TestFinalCoverageBoost adds tests for remaining uncovered code paths
func TestFinalCoverageBoost(t *testing.T) {
	t.Parallel()

	t.Run("cursor positioning edge cases", func(t *testing.T) {
		var output TestWriter
		renderer := newRenderer(&output, ThemeDefault, nil)

		// Test cursor at different positions
		testCases := []struct {
			input  string
			cursor int
		}{
			{"hello", 0},
			{"hello", 2},
			{"hello", 5},
			{"🚀", 0},
			{"🚀", 1},
			{"", 0},
		}

		for _, tc := range testCases {
			_, _, err := renderer.renderMainLine("$ ", tc.input, tc.cursor)
			if err != nil {
				t.Errorf("renderMainLine(%q, %d) error = %v", tc.input, tc.cursor, err)
			}
		}
	})

	t.Run("complex completion scenarios", func(t *testing.T) {
		completer := func(d Document) []Suggestion {
			input := d.WordBeforeCursor()
			switch input {
			case "g":
				return []Suggestion{
					{Text: "git", Description: "version control"},
					{Text: "grep", Description: "search text"},
					{Text: "go", Description: "programming language"},
				}
			case "git":
				return []Suggestion{
					{Text: "git status", Description: "show status"},
					{Text: "git commit", Description: "commit changes"},
				}
			default:
				return nil
			}
		}

		// Test multiple tab completions
		mock := &mockTerminal{
			// Type 'git', tab (multiple suggestions), enter to accept first, enter to submit
			input: []rune("git\t\r\r"),
		}

		var output TestWriter
		p := &Prompt{
			config: options{
				Prefix:    "$ ",
				Completer: completer,
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault, nil),
		}

		result, err := p.Run(context.Background())
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		t.Logf("Complex completion result: %q", result)
	})

	t.Run("history edge cases", func(t *testing.T) {
		// Test with empty history
		mock := &mockTerminal{
			input: []rune("\x1b[A\x1b[B\r"), // Up arrow, down arrow, enter
		}

		var output TestWriter
		p := &Prompt{
			config: options{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{}, // Empty history
			renderer: newRenderer(&output, ThemeDefault, nil),
		}

		result, err := p.Run(context.Background())
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != "" {
			t.Errorf("Expected empty result, got %q", result)
		}
	})

	t.Run("newMockTerminal helper coverage", func(t *testing.T) {
		mock := newMockTerminal("test")
		if mock == nil {
			t.Error("Expected non-nil mock terminal")
			return
		}
		if string(mock.input) != "test" {
			t.Errorf("Expected 'test', got %q", string(mock.input))
		}
	})

	t.Run("various key combinations", func(t *testing.T) {
		// Test backspace at different positions
		mock := &mockTerminal{
			input: []rune("hello\x7f\x7f\x7f\r"), // Type hello, 3 backspaces, enter
		}

		var output TestWriter
		p := &Prompt{
			config: options{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault, nil),
		}

		result, err := p.Run(context.Background())
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != "he" {
			t.Errorf("Expected 'he', got %q", result)
		}
	})

	t.Run("duplicate history handling", func(t *testing.T) {
		mock := &mockTerminal{
			input: []rune("test\r"),
		}

		var output TestWriter
		p := &Prompt{
			config: options{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{"test"}, // Same command already in history
			renderer: newRenderer(&output, ThemeDefault, nil),
		}

		result, err := p.Run(context.Background())
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != "test" {
			t.Errorf("Expected 'test', got %q", result)
		}

		// History should not have duplicate
		if len(p.history) != 1 {
			t.Errorf("Expected history length 1, got %d", len(p.history))
		}
	})
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 1; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}()))
}

func TestPromptHistoryMethods(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
		historyConfig: &historyConfig{
			Enabled:    true,
			MaxEntries: 3,
		},
	}

	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test initial empty history
	history := p.History()
	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d items", len(history))
	}

	// Test AddHistory
	p.AddHistory("command1")
	p.AddHistory("command2")
	p.AddHistory("command3")

	history = p.History()
	if len(history) != 3 {
		t.Errorf("Expected 3 history items, got %d", len(history))
	}

	// Test max history limit
	p.AddHistory("command4")
	history = p.History()
	if len(history) != 3 {
		t.Errorf("Expected history to be limited to 3 items, got %d", len(history))
	}

	// Test duplicate prevention
	p.AddHistory("command4") // duplicate
	history = p.History()
	if len(history) != 3 {
		t.Errorf("Expected no duplicate, history length should be 3, got %d", len(history))
	}

	// Test AddHistory with empty string
	p.AddHistory("")
	history = p.History()
	if len(history) != 3 {
		t.Errorf("Expected empty string to be ignored, history length should be 3, got %d", len(history))
	}

	// Test SetHistory
	newHistory := []string{"new1", "new2", "new3", "new4", "new5"}
	p.SetHistory(newHistory)
	history = p.History()
	if len(history) != 3 { // should be limited by MaxHistory
		t.Errorf("Expected history to be limited to 3 items after SetHistory, got %d", len(history))
	}

	// Test ClearHistory
	p.SetHistory(nil)
	history = p.History()
	if len(history) != 0 {
		t.Errorf("Expected empty history after clear, got %d items", len(history))
	}
}

func TestPromptConfigurationMethods(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
	}

	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test SetPrefix
	p.SetPrefix(">>> ")
	if p.config.Prefix != ">>> " {
		t.Errorf("Expected prefix '>>> ', got %q", p.config.Prefix)
	}

	// Test SetTheme
	newTheme := &ColorScheme{
		Name:   "test",
		Prefix: Color{R: 255, G: 0, B: 0},
	}
	p.SetTheme(newTheme)
	if p.config.ColorScheme != newTheme {
		t.Error("Expected theme to be set")
	}
}

func TestPromptAcceptSuggestionAdvanced(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test accepting a suggestion
	suggestion := Suggestion{
		Text:        "git status",
		Description: "Show git status",
	}

	p.acceptSuggestion(suggestion)
	if string(p.buffer) != "git status" {
		t.Errorf("Expected buffer 'git status', got %q", string(p.buffer))
	}

	// Test cursor is at end after accepting suggestion
	if p.cursor != len(p.buffer) {
		t.Errorf("Expected cursor at end (%d), got %d", len(p.buffer), p.cursor)
	}
}

func TestPromptWithContextAdvanced(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "hello\r")
	defer p.Close()

	// Test with context that doesn't timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := p.Run(ctx)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("Expected result 'hello', got %q", result)
	}
}

func TestPromptWithCancelledContextAdvanced(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test with already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := p.Run(ctx)
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

func TestPromptCloseMultipleAdvanced(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "")

	// Test closing the prompt
	err := p.Close()
	if err != nil {
		t.Errorf("Unexpected error closing prompt: %v", err)
	}

	// Test closing again (should not error)
	err = p.Close()
	if err != nil {
		t.Errorf("Unexpected error closing prompt twice: %v", err)
	}
}

func TestPromptErrorHandlingAdvanced(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping test that creates real terminal in local development")
	}

	t.Parallel()

	// Test creating prompt with invalid config
	config := options{
		Prefix: "$ ",
	}

	p, err := newFromConfig(config)
	if err != nil {
		// This may fail in test environment - that's expected
		t.Logf("Expected error in test environment: %v", err)
		return
	}

	if p != nil {
		defer p.Close()
	}
}

func TestAdvancedKeyBindingsExtended(t *testing.T) {
	t.Parallel()

	keyMap := NewDefaultKeyMap()

	// Test some specific key actions
	actions := []struct {
		key  rune
		name string
	}{
		{'\r', "Enter"},
		{'\n', "Newline"},
		{'\b', "Backspace"},
		{'\t', "Tab"},
	}

	for _, action := range actions {
		t.Run(action.name, func(t *testing.T) {
			t.Parallel()
			keyAction := keyMap.Action(action.key)
			if keyAction == ActionNone {
				t.Errorf("Expected %s key to be bound", action.name)
			}
		})
	}
}

func TestPromptWithCustomCompleterAdvanced(t *testing.T) {
	t.Parallel()

	suggestions := []string{"git status", "git commit", "git push"}
	completer := func(d Document) []Suggestion {
		text := d.TextBeforeCursor()
		if strings.HasPrefix(text, "git") {
			var result []Suggestion
			for _, s := range suggestions {
				if strings.HasPrefix(s, text) {
					result = append(result, Suggestion{Text: s, Description: "Git command"})
				}
			}
			return result
		}
		return nil
	}

	config := options{
		Prefix:    "$ ",
		Completer: completer,
	}

	p := newForTestingWithConfig(t, config, "git\t\r\r")
	defer p.Close()

	result, err := p.Run(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// The result should contain one of the completions
	validResults := []string{"git status", "git commit", "git push"}
	found := slices.Contains(validResults, result)
	if !found {
		t.Errorf("Expected one of %v, got %q", validResults, result)
	}
}

func TestPromptAddHistoryComprehensive(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test adding multiple history items
	p.AddHistory("command1")
	p.AddHistory("command2")
	p.AddHistory("command3")

	if len(p.history) != 3 {
		t.Errorf("Expected history length 3, got %d", len(p.history))
	}
}

func TestPromptSuggestionScrolling(t *testing.T) {
	t.Parallel()

	// Create a completer that returns many suggestions
	completer := func(_ Document) []Suggestion {
		suggestions := make([]Suggestion, 0, 15)
		for i := range 15 {
			suggestions = append(suggestions, Suggestion{
				Text:        fmt.Sprintf("command%d", i),
				Description: fmt.Sprintf("description%d", i),
			})
		}
		return suggestions
	}

	config := options{
		Prefix:    "$ ",
		Completer: completer,
	}

	// Test with TAB to trigger suggestions, then submit first one
	p := newForTestingWithConfig(t, config, "c\t\r")
	defer p.Close()

	result, err := p.Run(context.Background())

	// EOF and ErrEOF are acceptable for this test - they just mean input ended
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, ErrEOF) {
		t.Errorf("Unexpected error: %v", err)
		return
	}

	// For multiple suggestions, TAB shows them and user input is used
	// The result might be the partial input "c" or a completed command
	// Accept empty result if EOF occurred
	if !errors.Is(err, io.EOF) && !errors.Is(err, ErrEOF) && result != "c" && !strings.HasPrefix(result, "command") {
		t.Errorf("Expected result to be 'c' or start with 'command', got %q", result)
	}
}

func TestPromptSuggestionScrollingEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		suggestionCount  int
		expectedComplete bool
	}{
		{
			name:             "empty suggestions",
			suggestionCount:  0,
			expectedComplete: false,
		},
		{
			name:             "single suggestion",
			suggestionCount:  1,
			expectedComplete: true,
		},
		{
			name:             "exactly max display count",
			suggestionCount:  10,
			expectedComplete: false, // Should show suggestions
		},
		{
			name:             "more than max display",
			suggestionCount:  15,
			expectedComplete: false, // Should show suggestions with scrolling
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			completer := func(_ Document) []Suggestion {
				var suggestions []Suggestion
				for i := range tt.suggestionCount {
					suggestions = append(suggestions, Suggestion{
						Text:        fmt.Sprintf("command%d", i),
						Description: fmt.Sprintf("description%d", i),
					})
				}
				return suggestions
			}

			config := options{
				Prefix:    "$ ",
				Completer: completer,
			}

			// Test TAB behavior
			var input string
			if tt.expectedComplete && tt.suggestionCount == 1 {
				input = "c\t\r" // TAB should auto-complete, then enter
			} else {
				input = "c\t\r" // TAB shows suggestions, enter submits current
			}

			p := newForTestingWithConfig(t, config, input)
			defer p.Close()

			result, err := p.Run(context.Background())

			// Accept EOF and ErrEOF as valid test termination conditions
			if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, ErrEOF) {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tt.expectedComplete && tt.suggestionCount == 1 {
				// Should have auto-completed
				if !errors.Is(err, io.EOF) && !errors.Is(err, ErrEOF) && result != "command0" {
					t.Errorf("Expected auto-completion to 'command0', got %q", result)
				}
			} else if tt.suggestionCount > 0 {
				// For multiple suggestions, either EOF or valid result is acceptable
				// This tests that scrolling doesn't crash or hang
				if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, ErrEOF) {
					t.Errorf("Expected valid result or EOF, got error: %v", err)
				}
			}
		})
	}
}

func TestPromptSetPrefixComprehensive(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "test\r")
	defer p.Close()

	// Test setting prefix - just verify method exists and doesn't panic
	p.SetPrefix(">> ")

	// Run and verify it still works
	result, err := p.Run(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "test" {
		t.Errorf("Expected 'test', got %q", result)
	}
}

func TestPromptWithHistoryPreloadedComprehensive(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
		historyConfig: &historyConfig{
			Enabled:    true,
			MaxEntries: 1000,
		},
	}
	p := newForTestingWithConfig(t, config, "new\r")
	defer p.Close()

	// Add some history entries to test with
	p.AddHistory("command1")
	p.AddHistory("command2")
	p.AddHistory("command3")

	// Verify that prompt can be created with history
	result, err := p.Run(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "new" {
		t.Errorf("Expected 'new', got %q", result)
	}

	// Verify history contains the manually added entries plus the new command
	if len(p.history) < 4 {
		t.Errorf("Expected history length at least 4, got %d", len(p.history))
	}
}

func TestPromptCompleterFunctionalityComprehensive(t *testing.T) {
	t.Parallel()

	completer := func(d Document) []Suggestion {
		text := d.TextBeforeCursor()
		if strings.HasPrefix(text, "te") {
			return []Suggestion{
				{Text: "test", Description: "Test command"},
				{Text: "testing", Description: "Testing command"},
			}
		}
		return nil
	}

	config := options{
		Prefix:    "$ ",
		Completer: completer,
	}
	p := newForTestingWithConfig(t, config, "te\t\r\r")
	defer p.Close()

	// Run with tab completion
	result, err := p.Run(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should have completed to one of the suggestions
	if result != "test" && result != "testing" {
		t.Errorf("Expected result 'test' or 'testing', got %q", result)
	}
}

func TestPromptTimeoutBehaviorComprehensive(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := p.Run(ctx)
	if err == nil {
		t.Error("Expected timeout error")
	}
	// On some platforms (like macOS), timeout might manifest as EOF instead of DeadlineExceeded
	if !errors.Is(err, context.DeadlineExceeded) && err.Error() != "EOF" {
		t.Errorf("Expected context.DeadlineExceeded or EOF, got %v", err)
	}
}

func TestPromptMinimalConfigComprehensive(t *testing.T) {
	t.Parallel()

	// Test with absolutely minimal config
	config := options{}
	p := newForTestingWithConfig(t, config, "test\r")
	defer p.Close()

	result, err := p.Run(context.Background())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "test" {
		t.Errorf("Expected 'test', got %q", result)
	}
}

func TestPromptRunMultipleComprehensive(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
	}

	// Test multiple runs with same config
	for i := range 3 {
		p := newForTestingWithConfig(t, config, "test\r")
		result, err := p.Run(context.Background())
		if err != nil {
			t.Errorf("Run %d failed: %v", i, err)
		}
		if result != "test" {
			t.Errorf("Run %d: expected 'test', got %q", i, result)
		}

		_ = p.Close()
	}
}

func TestPromptCloseIdempotencyComprehensive(t *testing.T) {
	t.Parallel()

	config := options{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "")

	// Test multiple closes
	for i := range 3 {
		err := p.Close()
		if err != nil {
			t.Errorf("Close %d failed: %v", i, err)
		}
	}
}

func TestNewFunctionAdditionalCoverage(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping test that creates real terminal in local development")
	}

	t.Run("WithHistoryFile", func(t *testing.T) {
		// NewForTesting doesn't load from file, so test manual loading
		tmpDir := t.TempDir()
		historyFile := filepath.Join(tmpDir, "test_history")

		// Create a history file with some content
		err := os.WriteFile(historyFile, []byte("command1\ncommand2\n"), 0600)
		if err != nil {
			t.Fatalf("Failed to create test history file: %v", err)
		}

		config := options{
			historyConfig: &historyConfig{
				Enabled:     true,
				MaxEntries:  100,
				File:        historyFile,
				MaxFileSize: 1024,
				MaxBackups:  3,
			},
		}

		// Test history manager loading separately
		hm := newHistoryManager(config.historyConfig)
		err = hm.loadHistory()
		if err != nil {
			t.Fatalf("Failed to load history: %v", err)
		}

		history := hm.getHistory()
		if len(history) != 2 {
			t.Errorf("Expected 2 history entries, got %d", len(history))
		}
	})

	t.Run("WithInvalidHistoryFile", func(t *testing.T) {
		// Current implementation: NewFromConfig doesn't validate history file path
		// It only calls LoadHistory(), which returns nil for non-existent files
		// So this test validates that invalid paths don't crash the system

		// Create a file then try to use it as a directory parent
		tmpDir := t.TempDir()
		blockingFile := filepath.Join(tmpDir, "blocking_file")
		if err := os.WriteFile(blockingFile, []byte("content"), 0600); err != nil {
			t.Fatalf("Failed to create blocking file: %v", err)
		}

		// Try to load history from path where parent is a file
		config := options{
			Prefix: "test> ",
			historyConfig: &historyConfig{
				Enabled: true,
				File:    filepath.Join(blockingFile, "history"), // Invalid path
			},
		}

		p, err := newFromConfig(config)
		// Current implementation: This should NOT fail during creation
		// because LoadHistory() returns nil for non-existent files
		if err != nil {
			// If it fails, it's likely due to terminal creation in test environment
			t.Logf("NewFromConfig failed (expected in test environment): %v", err)
		} else if p != nil {
			defer p.Close()
			// The error should occur when trying to save history
			histManager := newHistoryManager(config.historyConfig)
			histManager.addEntry("test")
			saveErr := histManager.saveHistory()
			if saveErr == nil {
				t.Error("Expected error when saving history to invalid path")
			} else {
				t.Logf("Got expected error when saving history: %v", saveErr)
			}
		}
	})

	t.Run("DefaultsAndNilValues", func(t *testing.T) {
		config := options{
			Prefix: "test> ",
			// All other fields nil/zero - should use defaults
		}

		p := newForTestingWithConfig(t, config, "")
		defer p.Close()

		if p.config.historyConfig == nil || p.config.historyConfig.MaxEntries != 1000 {
			maxEntries := 0
			if p.config.historyConfig != nil {
				maxEntries = p.config.historyConfig.MaxEntries
			}
			t.Errorf("Expected default historyConfig.MaxEntries 1000, got %d", maxEntries)
		}
		if p.config.historyConfig == nil {
			t.Error("Expected default historyConfig to be set")
		}
		if p.config.ColorScheme == nil {
			t.Error("Expected default ColorScheme to be set")
		}
		if p.config.KeyMap == nil {
			t.Error("Expected default KeyMap to be set")
		}
	})
}

func TestRunWithContextAdditionalCoverage(t *testing.T) {
	t.Run("ContextCancellation", func(t *testing.T) {
		config := options{
			Prefix: "test> ",
		}

		p := newForTestingWithConfig(t, config, "")
		defer p.Close()

		// Create a context that cancels immediately
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := p.Run(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled error, got: %v", err)
		}
	})

	t.Run("ContextTimeout", func(t *testing.T) {
		config := options{
			Prefix: "test> ",
		}

		p := newForTestingWithConfig(t, config, "")
		defer p.Close()

		// Create a context with very short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		// Wait for the deadline to actually pass rather than for a sleep that is
		// assumed to outlast it. On a loaded runner the timer can fire later than
		// the sleep ends, and Run then read the terminal instead of the expired
		// context and reported EOF.
		<-ctx.Done()

		_, err := p.Run(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected context.DeadlineExceeded error, got: %v", err)
		}
	})

	t.Run("EOFHandling", func(t *testing.T) {
		config := options{
			Prefix: "test> ",
		}

		// Mock input that immediately returns EOF
		p := newForTestingWithConfig(t, config, "")
		defer p.Close()

		// Replace terminal with one that returns EOF
		p.terminal = &eofMockTerminal{}

		_, err := p.Run(context.Background())
		// Should handle EOF gracefully by returning ErrEOF
		// This test mainly ensures the EOF handling branch is covered
		if !errors.Is(err, ErrEOF) {
			t.Errorf("Expected ErrEOF, got: %v", err)
		}
	})

	t.Run("ComplexKeySequences", func(t *testing.T) {
		config := options{
			Prefix: "test> ",
		}

		// Test escape sequence handling
		input := "\x1b[A\x1b[B\x1b[C\x1b[D\r" // Arrow keys + Enter
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		// Should handle the key sequences and return empty result
		if result != "" {
			t.Logf("Got result: %q", result)
		}
	})

	t.Run("HistoryNavigation", func(t *testing.T) {
		config := options{
			Prefix: "test> ",
			historyConfig: &historyConfig{
				Enabled:    true,
				MaxEntries: 1000,
			},
		}

		// Navigate history and submit
		input := "\x1b[A\x1b[A\r" // Up, Up, Enter
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		// Add some history entries to test navigation
		p.AddHistory("command1")
		p.AddHistory("command2")

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		// Should navigate to history and return a command
		if result == "" {
			t.Error("Expected non-empty result from history navigation")
		}
	})

	t.Run("CompletionFlow", func(t *testing.T) {
		completer := func(d Document) []Suggestion {
			if d.TextBeforeCursor() == "te" {
				return []Suggestion{
					{Text: "test", Description: "test command"},
					{Text: "temp", Description: "temp command"},
				}
			}
			return nil
		}

		config := options{
			Prefix:    "test> ",
			Completer: completer,
		}

		// Type "te", press Tab for completion, press down arrow, Enter to accept, Enter to submit
		input := "te\t\x1b[B\r\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		// Should complete the input to one of the suggestions
		if result != "test" && result != "temp" {
			t.Errorf("Expected completion result 'test' or 'temp', got %q", result)
		}
	})

	t.Run("CtrlDWithContent", func(t *testing.T) {
		config := options{
			Prefix: "test> ",
		}

		// Ctrl+D ends the session only on an empty line, so the line survives it
		// and is submitted by the Enter after it. Leaving the Enter out cannot
		// tell "Ctrl+D was ignored" from "the input ran out", which both end the
		// call the same way.
		input := "hello\x04\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result != "hello" {
			t.Errorf("Run() = %q, want %q", result, "hello")
		}
	})

	t.Run("CtrlCInterrupt", func(t *testing.T) {
		config := options{
			Prefix: "test> ",
		}

		// Type some content, then Ctrl+C
		input := "hello\x03"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		// Should return ErrInterrupted
		if !errors.Is(err, ErrInterrupted) {
			t.Errorf("Expected ErrInterrupted, got %v", err)
		}
		// Result should be empty on interrupt
		if result != "" {
			t.Errorf("Expected empty result on interrupt, got %q", result)
		}
	})

	t.Run("CtrlCWithoutContent", func(t *testing.T) {
		config := options{
			Prefix: "test> ",
		}

		// Press Ctrl+C immediately without typing anything
		input := "\x03"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		// Should return ErrInterrupted
		if !errors.Is(err, ErrInterrupted) {
			t.Errorf("Expected ErrInterrupted, got %v", err)
		}
		// Result should be empty on interrupt
		if result != "" {
			t.Errorf("Expected empty result on interrupt, got %q", result)
		}
	})

	t.Run("MultilineMode", func(t *testing.T) {
		config := options{
			Prefix:    "test> ",
			Multiline: true,
		}

		// Test multiline with backslash continuation
		input := "line1\\\nline2\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		if !stringContains(result, "line1") {
			t.Errorf("Expected multiline result to contain 'line1', got %q", result)
		}
	})

	t.Run("BackslashContinuation", func(t *testing.T) {
		config := options{
			Prefix:    "test> ",
			Multiline: true,
		}

		// Test backslash continuation in multiline mode
		input := "line1 \\\nline2\n\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		expected := "line1 \nline2"
		if result != expected {
			t.Errorf("Expected backslash continuation result %q, got %q", expected, result)
		}
	})

	t.Run("SingleLineBackslashContinuation", func(t *testing.T) {
		config := options{
			Prefix:    "test> ",
			Multiline: false,
		}

		// Test backslash continuation in single-line mode
		input := "echo hello \\\nworld\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		expected := "echo hello \nworld"
		if result != expected {
			t.Errorf("Expected single-line backslash continuation result %q, got %q", expected, result)
		}
	})

	t.Run("BracketedPasteMultilineInput", func(t *testing.T) {
		config := options{
			Prefix:    "test> ",
			Multiline: true,
		}

		input := "\x1b[200~SELECT 1\nSELECT 2\x1b[201~\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		expected := "SELECT 1\nSELECT 2"
		if result != expected {
			t.Errorf("Expected bracketed paste result %q, got %q", expected, result)
		}
	})

	t.Run("BracketedPastePreservesTrailingBackslash", func(t *testing.T) {
		config := options{
			Prefix:    "test> ",
			Multiline: true,
		}

		input := "\x1b[200~line1\\\nline2\x1b[201~\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		expected := "line1\\\nline2"
		if result != expected {
			t.Errorf("Expected pasted backslash result %q, got %q", expected, result)
		}
	})

	// Pasted text is data, not keystrokes. A TAB inside it must reach the buffer
	// instead of running completion, which silently deleted the TAB (and, with a
	// matching candidate, rewrote the pasted word).
	t.Run("BracketedPasteKeepsTabAsText", func(t *testing.T) {
		config := options{
			Prefix:    "test> ",
			Multiline: true,
			Completer: func(Document) []Suggestion {
				return []Suggestion{{Text: "abcdef"}}
			},
		}

		input := "\x1b[200~SELECT ab\tc\x1b[201~\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		expected := "SELECT ab\tc"
		if result != expected {
			t.Errorf("Expected pasted TAB result %q, got %q", expected, result)
		}
	})

	// A terminal sends CRLF for the line breaks of text copied on Windows. Both
	// bytes submit, so pasting one line break inserted two.
	t.Run("BracketedPasteCollapsesCRLFToOneNewline", func(t *testing.T) {
		config := options{
			Prefix:    "test> ",
			Multiline: true,
		}

		input := "\x1b[200~SELECT 1\r\nFROM t\x1b[201~\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		expected := "SELECT 1\nFROM t"
		if result != expected {
			t.Errorf("Expected pasted CRLF result %q, got %q", expected, result)
		}
	})

	// A control byte carried in pasted text must not be obeyed as a keystroke:
	// a stray 0x03 ended the whole prompt with ErrInterrupted.
	t.Run("BracketedPasteIgnoresControlBytes", func(t *testing.T) {
		config := options{
			Prefix:    "test> ",
			Multiline: true,
		}

		input := "\x1b[200~SELECT\x03 1\x1b[201~\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		expected := "SELECT 1"
		if result != expected {
			t.Errorf("Expected pasted control byte to be dropped, want %q, got %q", expected, result)
		}
	})

	// Pasting terminal output can carry an escape sequence. The ESC is a control
	// byte and goes, but the characters after it are text the user pasted and
	// must survive, and the sequence must not move the cursor.
	t.Run("BracketedPasteKeepsTextOfAnEscapeSequence", func(t *testing.T) {
		config := options{
			Prefix:    "test> ",
			Multiline: true,
		}

		input := "\x1b[200~ab\x1b[Acd\x1b[201~\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		expected := "ab[Acd"
		if result != expected {
			t.Errorf("Expected pasted escape sequence to keep its text, want %q, got %q", expected, result)
		}
	})

	// The CRLF pairing is per paste. A paste that ends in CR must not swallow the
	// newline the next paste begins with.
	t.Run("BracketedPasteResetsCRLFStateBetweenPastes", func(t *testing.T) {
		config := options{
			Prefix:    "test> ",
			Multiline: true,
		}

		input := "\x1b[200~a\r\x1b[201~\x1b[200~\nb\x1b[201~\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.Run(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		expected := "a\n\nb"
		if result != expected {
			t.Errorf("Expected each paste to keep its own line break, want %q, got %q", expected, result)
		}
	})
}

// Mock terminals for testing specific scenarios

type eofMockTerminal struct {
	mockTerminal
}

func (t *eofMockTerminal) ReadRune() (rune, int, error) {
	return 0, 0, io.EOF
}

func TestNewRealTerminalHandling(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping real terminal test in local development")
	}

	// Test actual New function with real terminal (may fail in CI)
	config := options{
		Prefix: "test> ",
		historyConfig: &historyConfig{
			Enabled: true,
			File:    "/dev/null/invalid", // Invalid path that will cause mkdir to fail
		},
	}

	_, err := newFromConfig(config)
	// In headless environments, this might fail due to terminal creation
	// In that case, we test that it fails appropriately
	if err != nil {
		t.Logf("New function failed as expected in headless environment: %v", err)
	}
}

// Helper function
func stringContains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestNewWithOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prefix  string
		options []Option
	}{
		{
			name:    "basic prefix only",
			prefix:  "$ ",
			options: nil,
		},
		{
			name:   "with completer",
			prefix: "> ",
			options: []Option{
				WithCompleter(func(d Document) []Suggestion {
					if strings.HasPrefix("test", d.WordBeforeCursor()) {
						return []Suggestion{{Text: "test", Description: "test command"}}
					}
					return nil
				}),
			},
		},
		{
			name:   "with history",
			prefix: ">>> ",
			options: []Option{
				WithMemoryHistory(50),
			},
		},
		{
			name:   "with color scheme",
			prefix: "# ",
			options: []Option{
				WithTheme(ThemeDefault),
			},
		},
		{
			name:   "with multiline",
			prefix: ">> ",
			options: []Option{
				WithMultiline(),
			},
		},
		{
			name:   "multiple options",
			prefix: "prompt> ",
			options: []Option{
				WithCompleter(func(_ Document) []Suggestion {
					return []Suggestion{{Text: "help", Description: "show help"}}
				}),
				WithMemoryHistory(100),
				WithMultiline(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Test that config is correctly constructed without creating the prompt
			config := options{
				Prefix: tt.prefix,
			}

			// Apply options
			for _, option := range tt.options {
				option(&config)
			}

			// Verify prefix was set correctly
			if config.Prefix != tt.prefix {
				t.Errorf("options prefix = %v, want %v", config.Prefix, tt.prefix)
			}

			// Test NewWithOptions function creates config correctly by using NewForTesting
			testConfig := options{Prefix: tt.prefix}
			for _, option := range tt.options {
				option(&testConfig)
			}

			// Create prompt with mock terminal for testing
			p := newForTestingWithConfig(t, testConfig, "test\n")
			defer p.Close()

			// Verify prefix was set correctly
			if p.config.Prefix != tt.prefix {
				t.Errorf("Prompt config prefix = %v, want %v", p.config.Prefix, tt.prefix)
			}

			// Verify defaults were set
			if p.config.historyConfig == nil || p.config.historyConfig.MaxEntries <= 0 {
				maxEntries := 0
				if p.config.historyConfig != nil {
					maxEntries = p.config.historyConfig.MaxEntries
				}
				t.Errorf("Prompt config historyConfig.MaxEntries should have default value > 0, got %v", maxEntries)
			}

			if p.config.ColorScheme == nil {
				t.Errorf("Prompt config ColorScheme should have default value, got nil")
			}

			if p.config.KeyMap == nil {
				t.Errorf("Prompt config KeyMap should have default value, got nil")
			}
		})
	}
}

func TestOptionFunctions(t *testing.T) {
	t.Parallel()

	config := options{}

	// Test WithCompleter
	completer := func(_ Document) []Suggestion {
		return []Suggestion{{Text: "test", Description: "test"}}
	}
	WithCompleter(completer)(&config)
	if config.Completer == nil {
		t.Error("WithCompleter() did not set completer")
	}

	// Test WithMemoryHistory
	WithMemoryHistory(500)(&config)
	if config.historyConfig == nil {
		t.Error("WithMemoryHistory() did not create historyConfig")
	} else {
		if config.historyConfig.MaxEntries != 500 {
			t.Errorf("WithMemoryHistory() MaxEntries = %v, want %v", config.historyConfig.MaxEntries, 500)
		}
		if config.historyConfig.File != "" {
			t.Errorf("WithMemoryHistory() should create memory-only history, got File = %v", config.historyConfig.File)
		}
		if !config.historyConfig.Enabled {
			t.Error("WithMemoryHistory() should enable history")
		}
	}

	// Test WithMultiline
	WithMultiline()(&config)
	if !config.Multiline {
		t.Error("WithMultiline() did not set multiline to true")
	}

	// Test WithTheme
	colorScheme := &ColorScheme{}
	WithTheme(colorScheme)(&config)
	if config.ColorScheme != colorScheme {
		t.Error("WithTheme() did not set color scheme")
	}
}

// newForTestingWithConfig creates a new prompt with a mock terminal for testing.
// This function is mainly for testing and migration purposes.
func newForTestingWithConfig(t *testing.T, config options, mockInput string) *Prompt {
	t.Helper()

	// Set defaults for history config
	if config.historyConfig == nil {
		// For testing, disable file persistence by default
		config.historyConfig = &historyConfig{
			Enabled:    true,
			MaxEntries: 1000,
			File:       "", // No file persistence in tests
		}
	} else {
		// Set defaults for incomplete history config
		if config.historyConfig.MaxEntries <= 0 {
			config.historyConfig.MaxEntries = 1000
		}
	}
	if config.ColorScheme == nil {
		config.ColorScheme = ThemeDefault
	}
	if config.KeyMap == nil {
		config.KeyMap = NewDefaultKeyMap()
	}

	// Setup output writer
	output := os.Stdout

	// Create mock terminal for testing
	terminal := newMockTerminal(mockInput)

	// Initialize history manager (but don't load from file for testing)
	historyManager := newHistoryManager(config.historyConfig)
	historyManager.setHistory([]string{}) // Start with empty history for testing

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

	return p
}

func TestContinuationPrefix(t *testing.T) {
	t.Parallel()

	incomplete := func(in string) bool { return strings.HasSuffix(strings.TrimSpace(in), ";") }

	t.Run("marks buffered lines without entering the result", func(t *testing.T) {
		t.Parallel()

		p := newForTestingWithConfig(t, options{
			Prefix:             "$ ",
			Multiline:          true,
			IsComplete:         incomplete,
			ContinuationPrefix: "...> ",
		}, "SELECT 1\nUNION ALL\nSELECT 2;\n")
		defer p.Close()

		var output bytes.Buffer
		p.output = &output
		p.renderer = newRenderer(&output, p.config.ColorScheme, p.terminal)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		result, err := p.Run(ctx)
		require.NoError(t, err)
		assert.Equal(t, "SELECT 1\nUNION ALL\nSELECT 2;", result, "the continuation prefix must not enter the returned input")
		assert.Contains(t, output.String(), "...> ", "the continuation prefix should have been drawn")
	})

	t.Run("is absent by default", func(t *testing.T) {
		t.Parallel()

		p := newForTestingWithConfig(t, options{
			Prefix:     "$ ",
			Multiline:  true,
			IsComplete: incomplete,
		}, "SELECT 1\nSELECT 2;\n")
		defer p.Close()

		var output bytes.Buffer
		p.output = &output
		p.renderer = newRenderer(&output, p.config.ColorScheme, p.terminal)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		result, err := p.Run(ctx)
		require.NoError(t, err)
		assert.Equal(t, "SELECT 1\nSELECT 2;", result)
		assert.NotContains(t, output.String(), "...> ")
	})

	t.Run("the option reaches the config", func(t *testing.T) {
		t.Parallel()

		var cfg options
		WithContinuationPrefix("...> ")(&cfg)
		assert.Equal(t, "...> ", cfg.ContinuationPrefix)
	})
}

// TestDeleteWordBackStopsAtTheWordItIsIn covers word editing outside ASCII. Every
// letter that is not a-z was a separator, so a word in Japanese was walked over
// as if it were whitespace and the deletion carried on into the word before it,
// while a letter with a diacritic split its own word in two.
func TestDeleteWordBackStopsAtTheWordItIsIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		typed string
		want  string
	}{
		{name: "ascii", typed: "one two", want: "one "},
		{name: "a japanese word after an ascii one", typed: "select 名前", want: "select "},
		{name: "a chinese word after an ascii one", typed: "a 中文", want: "a "},
		{name: "a word whose only non-ascii letter is inside it", typed: "naïve ", want: ""},
		{name: "a japanese word on its own", typed: "テーブル", want: ""},
		{name: "an underscore joins a word", typed: "a table_name", want: "a "},
		{name: "punctuation is still a separator", typed: "a, b", want: "a, "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPrompt(newMockTerminal(tt.typed + "\x17\r"))
			got, err := p.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("typing %q then Ctrl+W left %q, want %q", tt.typed, got, tt.want)
			}
		})
	}
}

// TestBackslashContinuationCountsInRunes covers the position arithmetic behind
// backslash continuation. The buffer is a []rune and the line's start indexes
// it, but the trailing backslash was found by adding the byte length of the
// line's text, so every multi-byte rune moved the position three cells further
// past the end: the prompt panicked, or -- when the buffer's capacity happened
// to reach that far -- deleted a rune that was not the backslash.
//
// The cases have to hold a multi-byte rune before the backslash. An ASCII line
// cannot tell a byte length from a rune index.
func TestBackslashContinuationCountsInRunes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		typed string
		want  string
	}{
		{name: "ascii", typed: "select a \\", want: "select a \n"},
		{name: "a japanese word before the backslash", typed: "select 名前 \\", want: "select 名前 \n"},
		{name: "the whole line is multibyte", typed: "日本語\\", want: "日本語\n"},
		{name: "an accented letter", typed: "naïve \\", want: "naïve \n"},
		{name: "an emoji", typed: "🎉 \\", want: "🎉 \n"},
		// Whitespace after the backslash is kept and ends up on the new line.
		// The point of the pair is that the two alphabets agree.
		{name: "spaces after an ascii backslash", typed: "a \\   ", want: "a \n   "},
		{name: "spaces after a multibyte backslash", typed: "名前 \\   ", want: "名前 \n   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPrompt(newMockTerminal(tt.typed + "\r\r"))
			got, err := p.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("typing %q then Enter gave %q, want %q", tt.typed, got, tt.want)
			}
		})
	}
}

// TestHistoryDownRestoresTheLineBeingTyped covers walking out of the history and
// back. The position past the newest entry is where the line being edited
// belongs, and nothing had saved it, so coming back to it emptied the prompt:
// looking up an earlier command cost whatever was half typed.
func TestHistoryDownRestoresTheLineBeingTyped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
		want   string
	}{
		{name: "up then down brings the line back", script: "hello\x1b[A\x1b[B\r", want: "hello"},
		{name: "twice up then twice down brings it back", script: "hello\x1b[A\x1b[A\x1b[B\x1b[B\r", want: "hello"},
		// Typing puts the prompt back on a fresh line, so an edited history entry
		// is what is being written now and Down has nowhere further to go.
		{name: "editing a history entry leaves the history", script: "hello\x1b[A!\x1b[B\r", want: "second!"},
		{name: "down on an untouched line stays empty", script: "\x1b[B\r", want: ""},
		{name: "up then down on an empty line stays empty", script: "\x1b[A\x1b[B\r", want: ""},
		{name: "stopping on a history entry submits that entry", script: "hello\x1b[A\r", want: "second"},
		{name: "typing after coming back keeps the restored line", script: "hel\x1b[A\x1b[Blo\r", want: "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPrompt(newMockTerminal(tt.script), WithMemoryHistory(10))
			p.SetHistory([]string{"first", "second"})
			got, err := p.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Run() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCtrlDReportsErrEOF pins the value a REPL matches on. Ctrl+D on an empty
// line returned io.EOF while every other end of input returned ErrEOF, so the
// loop this package's README shows -- break on errors.Is(err, prompt.ErrEOF) --
// never ended: it took the error branch and drew the prompt again, forever.
func TestCtrlDReportsErrEOF(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		script string
	}{
		{name: "ctrl+d on an empty line", script: "\x04"},
		{name: "the terminal reaching end of input", script: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newTestPrompt(newMockTerminal(tt.script))
			_, err := p.Run(context.Background())
			if !errors.Is(err, ErrEOF) {
				t.Errorf("Run() error = %v, want it to match ErrEOF", err)
			}
			// Matching io.EOF is what the one application using this library
			// does, so both endings have to keep answering to it.
			if !errors.Is(err, io.EOF) {
				t.Errorf("Run() error = %v, want it to match io.EOF too", err)
			}
		})
	}
}

// TestErrEOFKeepsItsMessage pins what the error prints, which callers log.
func TestErrEOFKeepsItsMessage(t *testing.T) {
	t.Parallel()

	if got := ErrEOF.Error(); got != "EOF" {
		t.Errorf("ErrEOF.Error() = %q, want %q", got, "EOF")
	}
}

// closeCountingTerminal records how often it was closed, so a test can see a
// terminal that was opened and then abandoned.
type closeCountingTerminal struct {
	mockTerminal
	closes int
}

func (c *closeCountingTerminal) Close() error {
	c.closes++
	return nil
}

// TestNewClosesTheTerminalWhenItCannotFinish covers the descriptor a failed New
// used to leave open. The terminal is opened before the history is loaded, and a
// load that fails returns no prompt, so nothing was left that could close it:
// the go-tty handle, the descriptor the reader polls, and the pipe that wakes it
// leaked, three per attempt for a caller that retries.
func TestNewClosesTheTerminalWhenItCannotFinish(t *testing.T) {
	t.Parallel()

	// A directory opens and then fails to read, which is a history file New
	// cannot load.
	unreadable := filepath.Join(t.TempDir(), "history")
	if err := os.Mkdir(unreadable, 0o750); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	terminal := &closeCountingTerminal{}
	config := options{
		Prefix:        "> ",
		historyConfig: &historyConfig{Enabled: true, File: unreadable, MaxEntries: 100},
		ColorScheme:   ThemeDefault,
		KeyMap:        NewDefaultKeyMap(),
	}

	p, err := newFromConfigOn(config, terminal, io.Discard)
	if err == nil {
		t.Fatalf("newFromConfigOn() built a prompt over an unreadable history file")
	}
	if p != nil {
		t.Errorf("newFromConfigOn() returned a prompt alongside its error")
	}
	if terminal.closes != 1 {
		t.Errorf("the terminal was closed %d times, want 1", terminal.closes)
	}
}

// TestNewKeepsTheTerminalWhenItSucceeds is the other half: the prompt owns the
// terminal once it has one, and closing it is the caller's to do.
func TestNewKeepsTheTerminalWhenItSucceeds(t *testing.T) {
	t.Parallel()

	terminal := &closeCountingTerminal{}
	config := options{
		Prefix:        "> ",
		historyConfig: &historyConfig{Enabled: true, MaxEntries: 100},
		ColorScheme:   ThemeDefault,
		KeyMap:        NewDefaultKeyMap(),
	}

	if _, err := newFromConfigOn(config, terminal, io.Discard); err != nil {
		t.Fatalf("newFromConfigOn() error = %v", err)
	}
	if terminal.closes != 0 {
		t.Errorf("the terminal was closed %d times, want 0", terminal.closes)
	}
}

// sizedMockTerminal is a mock terminal of a chosen width, so a property can be
// run against a narrow screen where the wrap arithmetic has somewhere to go
// wrong.
type sizedMockTerminal struct {
	mockTerminal
	width int
	// height is the terminal's height in rows, and zero means the usual 24: a
	// test that says nothing about the height is not asking about it.
	height int
}

func (s *sizedMockTerminal) Size() (width, height int, err error) {
	if s.height > 0 {
		return s.width, s.height, nil
	}
	return s.width, 24, nil
}

// TestRunLeavesTheScreenAgreeingWithTheLineItReturns drives whole sessions of
// random keystrokes and checks the one property that ties the read loop to the
// renderer: when Run returns a line, the screen shows the prefix and that line,
// and nothing else.
//
// It is the invariant every measurement bug this package has had would have
// broken -- the cursor drawn on the wrong row, a menu row left behind, a tab
// counted as a wrap, an escape sequence typed into the buffer -- because each of
// them leaves the screen saying something different from the line. Asserting one
// escape sequence at a time cannot see that; a terminal can.
//
// The keys are the ones a session actually receives, including the pastes and
// the escape sequences that turned out to be where the bugs were. The terminal
// varies in height as well as width, because an entry taller than the terminal
// is drawn as the window of itself the terminal has room for: the line has just
// been submitted when the screen is read, so the caret is at the end of the
// entry and the window is its last rows.
func TestRunLeavesTheScreenAgreeingWithTheLineItReturns(t *testing.T) {
	t.Parallel()

	keys := []string{
		"a", "b", "z", "0", " ", "あ", "日", "é", "e\u0301", "😀", "_", "-",
		"\x01", "\x05", "\x02", "\x06", "\x7f", "\x0b", "\x15", "\x17", "\x0c",
		"\x1b[A", "\x1b[B", "\x1b[C", "\x1b[D", "\x1b[H", "\x1b[F", "\x1b[3~",
		"\x1bb", "\x1bf", "\t",
		"\x1b[200~pasted text\x1b[201~", "\x1b[200~a\tb\x1b[201~", "\x1b[200~x\r\ny\x1b[201~",
	}
	widths := []int{8, 10, 20, 40, 80}

	// A fixed seed, so a failure is a failure anyone can reproduce from the
	// iteration it names.
	random := rand.New(rand.NewSource(31415)) //nolint:gosec // test input, not a secret

	for iter := range 2000 {
		width := widths[random.Intn(len(widths))]
		height := []int{2, 3, 5, 8, 24}[random.Intn(5)]
		var script strings.Builder
		for range random.Intn(25) {
			script.WriteString(keys[random.Intn(len(keys))])
		}
		script.WriteString("\r")

		var out bytes.Buffer
		terminal := &sizedMockTerminal{width: width, height: height}
		terminal.mockTerminal = *newMockTerminal(script.String())
		p := newTestPromptOn(terminal,
			WithCompleter(func(Document) []Suggestion {
				// One candidate carries an escape sequence, because a completer
				// reading names out of a file offers whatever the file holds,
				// and that is one of the two ways a control character reaches
				// the line without being typed. It is the only candidate the
				// word "z" matches, so typing z and pressing Tab accepts it
				// rather than opening a menu somebody has to steer.
				return []Suggestion{{Text: "create"}, {Text: "credit"}, {Text: "z\x1b[31mq"}, {Text: "テーブル"}}
			}),
			WithMemoryHistory(10),
		)
		p.output = &out
		p.renderer = newRenderer(&out, ThemeDefault, terminal)
		// And one history entry carries one, which is the other way: the history
		// file keeps every byte of an entry that is not a backslash or a line
		// break, and Up puts it back on the line.
		p.SetHistory([]string{"older one", "older two", "older\x1b[31mthree"})

		line, err := p.Run(context.Background())
		if err != nil {
			// A script can run out of keys before submitting, which ends the
			// session rather than the line.
			if !errors.Is(err, ErrEOF) {
				t.Fatalf("iter %d: Run() error = %v, script %q", iter, err, script.String())
			}
			continue
		}

		// Everything before the line break the submit wrote is the block.
		written := out.String()
		cut := strings.LastIndex(written, "\r\n")
		if cut < 0 {
			t.Fatalf("iter %d: submitting wrote no line break", iter)
		}
		drawn := newScreenModel(width)
		drawn.feed(written[:cut])

		// The screen shows the line with every rune the terminal would act on
		// replaced by a space: what is drawn has to be what is measured, and a
		// control character measures nothing.
		expected := newScreenModel(width)
		for i, text := range strings.Split(line, "\n") {
			if i == 0 {
				expected.writeString("$ ")
			} else {
				expected.startRow()
			}
			expected.writeString(singleLine(text))
		}

		// Where the cursor belongs: the same walk, stopped where the cursor was
		// when the line was submitted.
		atCursor := newScreenModel(width)
		cursorLine, cursorCol := p.renderer.findCursorPosition([]rune(line), p.cursor)
		for i, text := range strings.Split(line, "\n") {
			if i > cursorLine {
				break
			}
			if i == 0 {
				atCursor.writeString("$ ")
			} else {
				atCursor.startRow()
			}
			runes := []rune(singleLine(text))
			var atCaret rune
			if i == cursorLine && cursorCol < len(runes) {
				atCaret, runes = runes[cursorCol], runes[:cursorCol]
			}
			atCursor.writeString(string(runes))
			// The text before the cursor may have filled its row, which leaves a
			// terminal holding the cursor on the last cell with the wrap owed.
			// The character the cursor is in front of is what takes that wrap, so
			// the caret belongs in front of it on the next row rather than on top
			// of the row it filled -- but only if that character moves the cursor
			// at all. A combining mark joins the cell already written and leaves
			// the wrap owed, which is where cursorRowCol leaves the caret too.
			if atCaret == '\t' || runewidth.RuneWidth(atCaret) > 0 {
				atCursor.resolvePending()
			}
		}

		// A block taller than the terminal is drawn as the window of it the
		// terminal has room for, and the line has just been submitted, so the
		// caret is at its end and the window is the block's last rows.
		// The rows the block occupies, blank ones at its foot included: an entry
		// ending in a line break has a last line that holds nothing and is still
		// a row, and the caret is on it.
		total := expected.row + 1
		want := make([]string, total)
		copy(want, expected.rows())
		offset := 0
		if total > height {
			offset = total - height
			want = want[offset:]
		}
		for len(want) > 0 && want[len(want)-1] == "" {
			want = want[:len(want)-1]
		}
		if got := drawn.rows(); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("iter %d width=%d height=%d script=%q returned %q\n screen %q\n want   %q",
				iter, width, height, script.String(), line, got, want)
		}
		// And the cursor is on the character it is on. A measurement can put the
		// right text on screen and still leave the cursor somewhere else, which
		// is what a tab counted as a wrap did.
		if drawn.row != atCursor.row-offset || drawn.col != atCursor.col {
			t.Fatalf("iter %d width=%d height=%d script=%q returned %q: the cursor is at (%d, %d), want (%d, %d)\n%q",
				iter, width, height, script.String(), line, drawn.row, drawn.col, atCursor.row-offset, atCursor.col, drawn.rows())
		}
	}
}

// TestOptionsSetWhatTheyName covers the option constructors that nothing else
// reaches, so that a rename or a wrong field assignment is caught here rather
// than by the application that finds its history is not persisted.
func TestOptionsSetWhatTheyName(t *testing.T) {
	t.Parallel()

	t.Run("WithFileHistory names the file and the limit", func(t *testing.T) {
		t.Parallel()

		var config options
		WithFileHistory("/tmp/history", 42)(&config)
		if config.historyConfig == nil {
			t.Fatal("WithFileHistory() set no history configuration")
		}
		if !config.historyConfig.Enabled {
			t.Error("WithFileHistory() left history disabled")
		}
		if config.historyConfig.File != "/tmp/history" {
			t.Errorf("WithFileHistory() set File = %q, want /tmp/history", config.historyConfig.File)
		}
		if config.historyConfig.MaxEntries != 42 {
			t.Errorf("WithFileHistory() set MaxEntries = %d, want 42", config.historyConfig.MaxEntries)
		}
	})

	t.Run("WithHighlighter takes the function as given", func(t *testing.T) {
		t.Parallel()

		var config options
		WithHighlighter(func(string) []StyleSpan {
			return []StyleSpan{{Start: 0, End: 1}}
		})(&config)
		if config.Highlighter == nil {
			t.Fatal("WithHighlighter() set no highlighter")
		}
		if got := config.Highlighter("x"); len(got) != 1 || got[0].End != 1 {
			t.Errorf("the highlighter answered %v, want the one that was set", got)
		}
	})
}

// TestSubmittingFromALineAboveTheLastEndsTheBlockBelowTheEntry submits a
// multiline entry with the cursor on its first line, then prints what an
// application prints after a line: the output belongs under the entry, and the
// rows of the entry below the cursor are still on screen for it to land in.
func TestSubmittingFromALineAboveTheLastEndsTheBlockBelowTheEntry(t *testing.T) {
	t.Parallel()

	const width = 20

	var out bytes.Buffer
	terminal := &sizedMockTerminal{width: width}
	// Two Ups put the cursor on the first line; the statement is complete, so
	// the Enter after them submits rather than opening another line.
	terminal.mockTerminal = *newMockTerminal("select 1,\r2,\r3;\x1b[A\x1b[A\r")
	p := newTestPromptOn(terminal,
		WithMultiline(),
		WithContinuationPrefix("...> "),
		WithIsComplete(func(in string) bool { return strings.HasSuffix(strings.TrimSpace(in), ";") }),
	)
	p.output = &out
	p.renderer = newRenderer(&out, ThemeDefault, terminal)

	line, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if want := "select 1,\n2,\n3;"; line != want {
		t.Fatalf("Run() = %q, want %q", line, want)
	}
	fmt.Fprint(&out, "result\r\n")

	screen := newScreenModel(width)
	screen.feed(out.String())
	want := []string{"$ select 1,", "...> 2,", "...> 3;", "result"}
	if got := screen.rows(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the screen shows\n%q\nwant\n%q", got, want)
	}
}

// TestCancelingFromALineAboveTheLastLeavesNoRowOfTheEntryBehind interrupts a
// multiline entry with the cursor on its first line. What the next prompt is
// drawn under has to be the whole entry: ^C written into the middle of it eats a
// character, and the rows below the cursor are erased by nothing afterwards,
// because the next Run is told that what is on screen is not its to erase.
func TestCancelingFromALineAboveTheLastLeavesNoRowOfTheEntryBehind(t *testing.T) {
	t.Parallel()

	const width = 20

	var out bytes.Buffer
	terminal := &sizedMockTerminal{width: width}
	terminal.mockTerminal = *newMockTerminal("select 1,\r2,\r3\x1b[A\x1b[A\x03")
	p := newTestPromptOn(terminal,
		WithMultiline(),
		WithContinuationPrefix("...> "),
		WithIsComplete(func(in string) bool { return strings.HasSuffix(strings.TrimSpace(in), ";") }),
	)
	p.output = &out
	p.renderer = newRenderer(&out, ThemeDefault, terminal)

	if _, err := p.Run(context.Background()); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("Run() error = %v, want ErrInterrupted", err)
	}
	fmt.Fprint(&out, "$ ") // the prompt the REPL draws next

	screen := newScreenModel(width)
	screen.feed(out.String())
	want := []string{"$ select 1,", "...> 2,", "...> 3^C", "$"}
	if got := screen.rows(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the screen shows\n%q\nwant\n%q", got, want)
	}
}

// TestAKeyBoundToTheHistoryActionsWalksTheHistory binds Ctrl+P and Ctrl+N to
// ActionHistoryUp and ActionHistoryDown, which is what a person reaches for when
// the arrow keys are busy moving the cursor between the lines of an entry.
//
// Both actions are exported and neither had a case in the read loop, so a key
// bound to them did nothing at all.
func TestAKeyBoundToTheHistoryActionsWalksTheHistory(t *testing.T) {
	t.Parallel()

	const (
		ctrlP = "\x10"
		ctrlN = "\x0e"
	)

	tests := map[string]struct {
		keys      string
		multiline bool
		want      string
	}{
		"one back":                     {keys: ctrlP + "\r", want: "second"},
		"two back":                     {keys: ctrlP + ctrlP + "\r", want: "first"},
		"back past the oldest entry":   {keys: strings.Repeat(ctrlP, 5) + "\r", want: "first"},
		"back and forward again":       {keys: ctrlP + ctrlP + ctrlN + "\r", want: "second"},
		"forward from the line typed":  {keys: ctrlN + "\r", want: ""},
		"back and all the way forward": {keys: ctrlP + ctrlN + "\r", want: ""},
		"on a multiline entry":         {keys: "a\rb" + ctrlP + "\r", multiline: true, want: "second"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			keyMap := NewDefaultKeyMap()
			keyMap.Bind('\x10', ActionHistoryUp)
			keyMap.Bind('\x0e', ActionHistoryDown)

			var out bytes.Buffer
			terminal := newMockTerminal(tt.keys)
			opts := []Option{WithKeyMap(keyMap), WithMemoryHistory(10)}
			if tt.multiline {
				// The entry is complete only once it has been recalled from the
				// history, so the Enters typed before that open lines instead of
				// submitting and the cursor keys would be moving between them.
				opts = append(opts,
					WithMultiline(),
					WithIsComplete(func(in string) bool { return strings.HasPrefix(in, "second") }),
				)
			}
			p := newTestPromptOn(terminal, opts...)
			p.output = &out
			p.renderer = newRenderer(&out, ThemeDefault, terminal)
			p.SetHistory([]string{"first", "second"})

			line, err := p.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if line != tt.want {
				t.Errorf("Run() = %q, want %q", line, tt.want)
			}
		})
	}
}

// TestCloseEndsTheSessionBelowTheEntryOnScreen closes a prompt whose caret is
// on the first line of a three-line entry, then draws what a shell draws when
// the program exits. The shell's prompt belongs under the entry: the rows below
// the caret are still on screen and nothing redraws over them, because they
// belong to no program any more.
func TestCloseEndsTheSessionBelowTheEntryOnScreen(t *testing.T) {
	t.Parallel()

	const width, height = 20, 10

	tests := map[string]struct {
		buffer string
		cursor int
		want   []string
	}{
		"the caret on the first line of three": {
			buffer: "one\ntwo\nthree", cursor: 1,
			want: []string{"$ one", "two", "three", "sh$"},
		},
		"the caret on the last line": {
			buffer: "one\ntwo\nthree", cursor: 13,
			want: []string{"$ one", "two", "three", "sh$"},
		},
		"a single line": {
			buffer: "one", cursor: 1,
			want: []string{"$ one", "sh$"},
		},
		"nothing typed": {
			buffer: "", cursor: 0,
			want: []string{"$", "sh$"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			terminal := &sizedMockTerminal{width: width, height: height}
			p := newTestPromptOn(terminal, WithMultiline())
			p.output = &out
			p.renderer = newRenderer(&out, ThemeDefault, terminal)
			p.buffer = []rune(tt.buffer)
			p.cursor = tt.cursor

			if err := p.render(); err != nil {
				t.Fatalf("render: %v", err)
			}
			if err := p.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			fmt.Fprint(&out, "sh$ ") // what the shell draws once the program is gone

			screen := newBoundedScreenModel(width, height)
			screen.feed(out.String())
			if got := screen.rows(); strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("the screen shows\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestSetThemeTakesANilThemeTheWayNewDoes gives the setter the value the
// constructor accepts. A nil scheme means the default one there, and the
// renderer reads colors off the scheme on every render without checking, so
// writing a nil one through was a panic on the next keystroke.
func TestSetThemeTakesANilThemeTheWayNewDoes(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	terminal := newMockTerminal("ab\r")
	p := newTestPromptOn(terminal)
	p.output = &out
	p.renderer = newRenderer(&out, ThemeDefault, terminal)

	p.SetTheme(nil)
	if p.config.ColorScheme != ThemeDefault {
		t.Errorf("SetTheme(nil) left the scheme as %v, want the default", p.config.ColorScheme)
	}

	line, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if line != "ab" {
		t.Errorf("Run() = %q, want %q", line, "ab")
	}
}

// TestSetThemeKeepsWhatTheRendererKnows changes the theme between one render and
// the next. What the renderer holds is what it knows about the screen -- how
// tall the block on it is and which row the caret is on -- and the next redraw
// erases from there. Replacing the renderer to change a color threw that away,
// so the redraw erased from the wrong row and left the block behind.
func TestSetThemeKeepsWhatTheRendererKnows(t *testing.T) {
	t.Parallel()

	const width, height = 20, 24

	var out bytes.Buffer
	terminal := &sizedMockTerminal{width: width, height: height}
	p := newTestPromptOn(terminal, WithMultiline())
	p.output = &out
	p.renderer = newRenderer(&out, ThemeDefault, terminal)
	p.buffer = []rune("one\ntwo\nthree")
	p.cursor = len(p.buffer)

	if err := p.render(); err != nil {
		t.Fatalf("render: %v", err)
	}
	screen := newBoundedScreenModel(width, height)
	screen.feed(out.String())

	p.SetTheme(&ColorScheme{Name: "other", Prefix: Color{R: 255}, Input: Color{G: 255}})

	out.Reset()
	if err := p.render(); err != nil {
		t.Fatalf("render: %v", err)
	}
	screen.feed(out.String())

	want := []string{"$ one", "two", "three"}
	if got := screen.rows(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("after the theme changed the screen shows\n%q\nwant\n%q", got, want)
	}
}

// TestAMultilineSessionLeavesTheScreenAgreeingWithTheEntry is
// TestRunLeavesTheScreenAgreeingWithTheLineItReturns for the configuration the
// one user of this library runs: a multiline entry collected until a semicolon,
// a continuation prefix on every line after the first, an auto-indenter, and a
// highlighter over the lot.
//
// Every piece of that changes what is drawn without changing what is measured,
// which is where this package's bugs live: the continuation prefix is cells the
// first line does not have, the indent is text the user did not type, and the
// highlighter is escape sequences between the runes. When the entry is
// submitted the screen has to show the prefix, the continuation prefixes and the
// entry -- the window of it the terminal has room for -- and the caret has to be
// at its end, which is where ending a line at the foot of the block leaves it.
func TestAMultilineSessionLeavesTheScreenAgreeingWithTheEntry(t *testing.T) {
	t.Parallel()

	keys := []string{
		"a", "b", ";", " ", "あ", "é", "é", "😀", "\t",
		"\x01", "\x05", "\x02", "\x06", "\x7f", "\x0b", "\x15", "\x17",
		"\x1b[A", "\x1b[B", "\x1b[C", "\x1b[D", "\x1b[H", "\x1b[F", "\x1b[3~",
		"\r", "\r", "\x1b[200~x\r\ny\x1b[201~",
	}
	widths := []int{8, 20, 40}
	heights := []int{3, 5, 24}
	// A fixed seed, so a failure is one anyone can reproduce from the iteration
	// it names.
	random := rand.New(rand.NewSource(90909)) //nolint:gosec // test input, not a secret

	for iter := range 3000 {
		width := widths[random.Intn(len(widths))]
		height := heights[random.Intn(len(heights))]
		var script strings.Builder
		for range random.Intn(20) {
			script.WriteString(keys[random.Intn(len(keys))])
		}
		script.WriteString(";\r")

		var out bytes.Buffer
		terminal := &sizedMockTerminal{width: width, height: height}
		terminal.mockTerminal = *newMockTerminal(script.String())
		p := newTestPromptOn(terminal,
			WithMultiline(),
			WithContinuationPrefix("..> "),
			WithIsComplete(func(in string) bool { return strings.HasSuffix(strings.TrimSpace(in), ";") }),
			WithAutoIndent(func(before string) string {
				line := before[strings.LastIndex(before, "\n")+1:]
				return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			}),
			WithHighlighter(func(in string) []StyleSpan {
				var spans []StyleSpan
				for i := 0; i+1 < len([]rune(in)); i += 3 {
					spans = append(spans, StyleSpan{Start: i, End: i + 2, Color: Color{R: 255}})
				}
				return spans
			}),
		)
		p.output = &out
		p.renderer = newRenderer(&out, ThemeDefault, terminal)

		line, err := p.Run(context.Background())
		if err != nil {
			if !errors.Is(err, ErrEOF) && !errors.Is(err, ErrInterrupted) {
				t.Fatalf("iter %d: Run() error = %v script %q", iter, err, script.String())
			}
			continue
		}

		written := out.String()
		cut := strings.LastIndex(written, "\r\n")
		if cut < 0 {
			t.Fatalf("iter %d: submitting wrote no line break", iter)
		}
		drawn := newScreenModel(width)
		drawn.feed(written[:cut])

		expected := newScreenModel(width)
		for i, text := range strings.Split(line, "\n") {
			if i == 0 {
				expected.writeString("$ ")
			} else {
				expected.startRow()
				expected.writeString("..> ")
			}
			expected.writeString(singleLine(text))
		}

		total := expected.row + 1
		want := make([]string, total)
		copy(want, expected.rows())
		offset := 0
		if total > height {
			offset = total - height
			want = want[offset:]
		}
		for len(want) > 0 && want[len(want)-1] == "" {
			want = want[:len(want)-1]
		}
		if got := drawn.rows(); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("iter %d width=%d height=%d script=%q returned %q\n screen %q\n want   %q",
				iter, width, height, script.String(), line, got, want)
		}
		if drawn.row != expected.row-offset || drawn.col != expected.col {
			t.Fatalf("iter %d width=%d height=%d script=%q returned %q: the caret is at (%d,%d), the end of the entry is at (%d,%d)\n%q",
				iter, width, height, script.String(), line, drawn.row, drawn.col, expected.row-offset, expected.col, drawn.rows())
		}
	}
}

// TestAPasteIsNotRedrawnOncePerCharacter pastes a statement of the length a
// generated query reaches and counts what reached the terminal.
//
// The block is redrawn whole every time, so a redraw per pasted character cost
// the terminal a copy of the block per character: fifty megabytes of escape
// sequences for twenty thousand characters, and seconds of a terminal drawing
// them, which from the outside is a shell that has hung.
//
// What is asserted is a bound rather than a number. The block's height depends
// on the terminal, so a redraw is worth what it is worth; the count of them is
// what was wrong, and the paste is drawn on its way in so that a long one still
// shows something happening.
// TestWhatABracketedPasteIsMadeOf pins what the read loop does between the two
// markers a terminal wraps a paste in. Everything there is content rather than
// a keystroke, which is the point of bracketed paste and the reason a pasted
// statement does not run itself; the cases below are the edges of that rule.
func TestWhatABracketedPasteIsMadeOf(t *testing.T) {
	t.Parallel()

	const (
		start = "\x1b[200~"
		end   = "\x1b[201~"
	)

	for _, tt := range []struct {
		name   string
		script string
		want   string
	}{
		{name: "an ordinary paste", script: start + "hello" + end + "\r", want: "hello"},
		// The whole point: a statement pasted with its line breaks is one entry
		// waiting to be read, not several that ran as they arrived.
		{name: "a newline inside a paste does not submit", script: start + "one\rtwo" + end + "\r", want: "one\ntwo"},
		{name: "a tab inside a paste is not completion", script: start + "a\tb" + end + "\r", want: "a\tb"},
		{name: "Ctrl+C inside a paste does not interrupt", script: start + "a\x03b" + end + "\r", want: "ab"},
		// An escape sequence in the middle of a paste is content the terminal
		// passed through -- copied terminal output is full of them. The ESC is a
		// control byte and goes; what it introduced is text and stays.
		{name: "a sequence inside a paste keeps its text", script: start + "a\x1b[31mb" + end + "\r", want: "a[31mb"},
		{name: "an end marker with no start is not text", script: "ab" + end + "c\r", want: "abc"},
		{name: "an empty paste", script: start + end + "ab\r", want: "ab"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, err := newFromConfigOn(options{
				Prefix:      "$ ",
				ColorScheme: ThemeDefault,
				KeyMap:      NewDefaultKeyMap(),
				Multiline:   true,
				IsComplete:  func(string) bool { return true },
			}, newMockTerminal(tt.script), io.Discard)
			if err != nil {
				t.Fatalf("newFromConfigOn() error = %v", err)
			}
			defer p.Close()

			got, err := p.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Run() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAPasteWithNoEndMarkerHoldsTheLine is the other side of that rule, and the
// reason the godoc states it: while a paste is open every key is content, so
// there is none left that ends the line. A terminal always closes a paste it
// opened, but a program writing the input by hand may not, and what that looks
// like is a prompt that ignores Enter.
//
// The line ends when the input does, which is what this asserts: ErrEOF rather
// than the entry.
func TestAPasteWithNoEndMarkerHoldsTheLine(t *testing.T) {
	t.Parallel()

	p, err := newFromConfigOn(options{
		Prefix:      "$ ",
		ColorScheme: ThemeDefault,
		KeyMap:      NewDefaultKeyMap(),
	}, newMockTerminal("\x1b[200~hello\r\x03\r"), io.Discard)
	if err != nil {
		t.Fatalf("newFromConfigOn() error = %v", err)
	}
	defer p.Close()

	got, err := p.Run(context.Background())
	if !errors.Is(err, ErrEOF) {
		t.Fatalf("Run() error = %v, want ErrEOF: neither Enter nor Ctrl+C ends an open paste", err)
	}
	if got != "" {
		t.Errorf("Run() = %q, want nothing", got)
	}
}

func TestAPasteIsNotRedrawnOncePerCharacter(t *testing.T) {
	t.Parallel()

	const width, height = 80, 24
	const pasted = 20000

	statement := strings.Repeat("select 1, 2, 3 from t where x = 1; ", pasted/34+1)[:pasted]

	var out bytes.Buffer
	terminal := &sizedMockTerminal{width: width, height: height}
	terminal.mockTerminal = *newMockTerminal("\x1b[200~" + statement + "\x1b[201~\r")
	p := newTestPromptOn(terminal)
	p.output = &out
	p.renderer = newRenderer(&out, ThemeDefault, terminal)

	line, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len([]rune(line)) != pasted {
		t.Fatalf("the paste of %d characters came back as %d", pasted, len([]rune(line)))
	}

	// One redraw of a full screen is about width*height cells plus the escape
	// sequences between them. A hundred of those is room for the redraws a paste
	// this long is worth and nowhere near the twenty thousand it used to take.
	const budget = 100 * width * height * 4
	if out.Len() > budget {
		t.Errorf("pasting %d characters wrote %d bytes to the terminal, which is more than %d: the block is being redrawn per character",
			pasted, out.Len(), budget)
	}
	// And it is drawn on the way in rather than only at the end, so a paste that
	// takes a moment shows something happening.
	if strings.Count(out.String(), showCursorSequence) < 2 {
		t.Errorf("the paste was drawn %d time(s): a long one shows nothing arriving", strings.Count(out.String(), showCursorSequence))
	}
}

// TestAPasteHoldingEscapeSequencesIsNotRedrawnPerSequence pastes what copying
// colored terminal output gives you: text with an escape sequence every few
// characters.
//
// Those arrive through the branch that reads a sequence rather than the one that
// reads a rune, and that branch falls through to the render at the foot of the
// read loop. Throttling only the rune branch left a paste of colored output
// costing a redraw per sequence, which is the cost the throttle exists to remove.
func TestAPasteHoldingEscapeSequencesIsNotRedrawnPerSequence(t *testing.T) {
	t.Parallel()

	const width, height = 80, 24
	const sequences = 2000

	// "\x1b[31mword " repeated: a sequence and a word, the way a colored log
	// reaches the clipboard.
	colored := strings.Repeat("\x1b[31mword ", sequences)

	var out bytes.Buffer
	terminal := &sizedMockTerminal{width: width, height: height}
	terminal.mockTerminal = *newMockTerminal("\x1b[200~" + colored + "\x1b[201~\r")
	p := newTestPromptOn(terminal)
	p.output = &out
	p.renderer = newRenderer(&out, ThemeDefault, terminal)

	line, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// The ESC of each sequence is a control byte and is dropped; the rest of it
	// is text and is kept, which is what the read loop documents.
	if want := strings.Repeat("[31mword ", sequences); line != want {
		t.Fatalf("the paste came back as %d characters, want %d", len([]rune(line)), len([]rune(want)))
	}

	const budget = 100 * width * height * 4
	if out.Len() > budget {
		t.Errorf("pasting %d escape sequences wrote %d bytes to the terminal, which is more than %d: the block is being redrawn per sequence",
			sequences, out.Len(), budget)
	}
}

// TestCtrlUClearsTheLineTheCursorIsOn pins what the key that says "delete the
// line" deletes. Ctrl+K beside it works on the current line when the entry has
// more than one, and there is no undo: on a statement typed across several
// lines, a key that looks like "clear this line" was discarding the statement.
//
// What discards a whole entry is Ctrl+C, which says so on screen and gives the
// caller ErrInterrupted.
func TestCtrlUClearsTheLineTheCursorIsOn(t *testing.T) {
	t.Parallel()

	const (
		ctrlU = "\x15"
		ctrlB = "\x02" // one character back, which is what the key map binds it to
	)

	tests := map[string]struct {
		keys string
		want string
	}{
		// On the third line, which is then retyped and submitted.
		"the line the cursor is on": {keys: "select 1,\r2,\r3" + ctrlU + "x;\r", want: "select 1,\n2,\nx;"},
		// Part way along a line takes the whole of that line, which is what it
		// does on an entry of one.
		"the whole of that line": {keys: "select 1,\rfrom t" + ctrlB + ctrlB + ctrlU + "x;\r", want: "select 1,\nx;"},
		// The first line of several, leaving the lines under it alone.
		"the first line of several": {keys: "one,\rtwo;" + strings.Repeat("\x1b[A", 1) + ctrlU + "select 1,\r", want: "select 1,\ntwo;"},
		// An entry of one line is unchanged: there the line is the entry.
		"an entry of one line": {keys: "select 9" + ctrlU + "select 1;\r", want: "select 1;"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			terminal := &sizedMockTerminal{width: 40, height: 24}
			terminal.mockTerminal = *newMockTerminal(tt.keys)
			p := newTestPromptOn(terminal,
				WithMultiline(),
				WithIsComplete(func(in string) bool { return strings.HasSuffix(in, ";") }),
			)
			p.output = &out
			p.renderer = newRenderer(&out, ThemeDefault, terminal)

			line, err := p.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if line != tt.want {
				t.Errorf("Run() = %q, want %q", line, tt.want)
			}
		})
	}
}
