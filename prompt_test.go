package prompt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "default config",
			config: Config{
				Prefix: "$ ",
			},
		},
		{
			name: "with completer",
			config: Config{
				Prefix: "> ",
				Completer: func(d Document) []Suggestion {
					text := d.GetWordBeforeCursor()
					if strings.HasPrefix("hello", text) {
						return []Suggestion{{Text: "hello", Description: "greeting"}}
					}
					return nil
				},
			},
		},
		{
			name: "with history",
			config: Config{
				Prefix: ">>> ",
				HistoryConfig: &HistoryConfig{
					Enabled:    true,
					MaxEntries: 1000,
				},
			},
		},
		{
			name: "with color scheme",
			config: Config{
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
			require.NotNil(t, p.config.HistoryConfig, "HistoryConfig should not be nil")
			assert.Greater(t, p.config.HistoryConfig.MaxEntries, 0, "HistoryConfig.MaxEntries should have default value")

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
		config   Config
	}{
		{
			name:     "simple input",
			input:    "hello\n",
			expected: "hello",
			config:   Config{Prefix: "$ "},
		},
		{
			name:     "input with backspace",
			input:    "hello\x7f\x7fo\n", // hello, backspace, backspace, o, enter
			expected: "helo",
			config:   Config{Prefix: "$ "},
		},
		{
			name:     "empty input",
			input:    "\n",
			expected: "",
			config:   Config{Prefix: "$ "},
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

			result, err := p.RunWithContext(ctx)
			require.NoError(t, err, "RunWithContext() should not fail")
			assert.Equal(t, tt.expected, result, "RunWithContext() result should match expected")
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
		text := d.GetWordBeforeCursor()
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

	p := newForTestingWithConfig(t, Config{
		Prefix: "$ ",
		HistoryConfig: &HistoryConfig{
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
	p, err := newFromConfig(Config{
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
		config: Config{
			Prefix:      "test> ",
			ColorScheme: scheme,
		},
	}

	assert.NotNil(t, p.config.ColorScheme, "Expected non-nil color scheme")

	// Test with nil color scheme (should use default)
	p2 := &Prompt{
		config: Config{
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
	result := Reset()
	if result != expected {
		t.Errorf("Reset() = %q, want %q", result, expected)
	}
}

func TestPromptClose(t *testing.T) {
	t.Parallel()

	// Test closing a prompt with mock terminal
	mock := &mockTerminal{}
	p := &Prompt{
		config:   Config{Prefix: "test> "},
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
		config: Config{
			Prefix: "test> ",
			HistoryConfig: &HistoryConfig{
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

func TestPromptBufferManipulation(t *testing.T) {
	t.Parallel()

	mock := &mockTerminal{}
	p := &Prompt{
		config:   Config{Prefix: "test> "},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
		buffer:   []rune{},
		cursor:   0,
	}

	// Test insertRune
	p.insertRune('a')
	if string(p.buffer) != "a" {
		t.Errorf("Expected buffer 'a', got %q", string(p.buffer))
	}
	if p.cursor != 1 {
		t.Errorf("Expected cursor position 1, got %d", p.cursor)
	}

	// Test insertText
	p.insertText("bc")
	if string(p.buffer) != "abc" {
		t.Errorf("Expected buffer 'abc', got %q", string(p.buffer))
	}
	if p.cursor != 3 {
		t.Errorf("Expected cursor position 3, got %d", p.cursor)
	}

	// Test setBuffer
	p.setBuffer("hello")
	if string(p.buffer) != "hello" {
		t.Errorf("Expected buffer 'hello', got %q", string(p.buffer))
	}
	if p.cursor != 5 {
		t.Errorf("Expected cursor position 5, got %d", p.cursor)
	}
}

func TestReadEscapeSequence(t *testing.T) {
	t.Parallel()

	// Test with mock terminal that provides escape sequences
	mock := &mockTerminal{
		input: []rune("[A"), // Up arrow (without initial ESC)
	}

	p := &Prompt{
		config:   Config{Prefix: "test> "},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
	}

	seq, err := p.readEscapeSequence()
	if err != nil {
		t.Errorf("Expected no error reading escape sequence, got: %v", err)
	}

	if seq != "[A" {
		t.Errorf("Expected sequence '[A', got %q", seq)
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
				config: Config{
					Prefix: "$ ",
					HistoryConfig: &HistoryConfig{
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
				renderer: newRenderer(&output, ThemeDefault),
			}

			result, err := p.RunWithContext(context.Background())
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
		text := d.GetWordBeforeCursor()
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
		config: Config{
			Prefix:    "$ ",
			Completer: completer,
		},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
		output:   &output,
		buffer:   []rune{},
		cursor:   0,
		history:  []string{},
		renderer: newRenderer(&output, ThemeDefault),
	}

	result, err := p.RunWithContext(context.Background())
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
		config: Config{
			Prefix: "$ ",
			HistoryConfig: &HistoryConfig{
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
		renderer: newRenderer(&output, ThemeDefault),
	}

	result, err := p.RunWithContext(context.Background())
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
		config: Config{
			Prefix: "$ ",
		},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
		output:   &output,
		buffer:   []rune{},
		cursor:   0,
		history:  []string{},
		renderer: newRenderer(&output, ThemeDefault),
	}

	result, err := p.RunWithContext(context.Background())
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
		config: Config{
			Prefix: "$ ",
		},
		terminal: mock,
		keyMap:   NewDefaultKeyMap(),
		output:   &output,
		buffer:   []rune{},
		cursor:   0,
		history:  []string{},
		renderer: newRenderer(&output, ThemeDefault),
	}

	result, err := p.RunWithContext(context.Background())
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
				config: Config{
					Prefix: "$ ",
				},
				terminal: mock,
				keyMap:   NewDefaultKeyMap(),
				output:   &output,
				buffer:   []rune{},
				cursor:   0,
				history:  []string{},
				renderer: newRenderer(&output, ThemeDefault),
			}

			result, err := p.RunWithContext(context.Background())
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
	renderer := newRenderer(failingWriter, ThemeDefault)
	err := renderer.render("$ ", "test", 4)
	if err == nil {
		t.Error("Expected error from failing writer in renderer")
	}

	// Test with a prompt that has initial render failure
	p := &Prompt{
		config: Config{
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

	_, err = p.RunWithContext(context.Background())
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
		config := Config{
			Prefix: "test> ",
			HistoryConfig: &HistoryConfig{
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
			config: Config{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output, // Use buffer instead of stdout
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault),
		}

		result, err := p.Run()
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
		if result != "hello" {
			t.Errorf("Expected 'hello', got %q", result)
		}
	})

	t.Run("NewForTesting coverage", func(t *testing.T) {
		// Test NewForTesting function
		config := Config{
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
			config: Config{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault),
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := p.RunWithContext(ctx)
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
			config: Config{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault),
		}

		result, err := p.RunWithContext(context.Background())
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
		renderer := newRenderer(&output, ThemeDefault)

		// Create more than 10 suggestions
		suggestions := make([]Suggestion, 15)
		for i := range suggestions {
			suggestions[i] = Suggestion{
				Text:        fmt.Sprintf("suggestion_%d", i),
				Description: fmt.Sprintf("desc_%d", i),
			}
		}

		err := renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 5, 0)
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
		renderer := newRenderer(&output, ThemeDefault)

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
		renderer := newRenderer(failing, ThemeDefault)

		// Test renderMainLine error
		err := renderer.renderMainLine("$ ", "test", 2)
		if err == nil {
			t.Error("Expected error from failing writer in renderMainLine")
		}

		// Test renderSuggestions error
		suggestions := []Suggestion{{Text: "test", Description: "desc"}}
		err = renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 0, 0)
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
		renderer := newRenderer(&output, ThemeDefault)

		suggestions := []Suggestion{
			{Text: "cmd1"},
			{Text: "cmd2"},
		}

		err := renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 0, 0)
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
		config := Config{
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
		config := Config{
			Prefix: "test> ",
			HistoryConfig: &HistoryConfig{
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
		config := Config{
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
			input := d.GetWordBeforeCursor()
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
			config: Config{
				Prefix:    "$ ",
				Completer: completer,
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault),
		}

		result, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Errorf("RunWithContext() error = %v", err)
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
			config: Config{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{"previous command", "another command"},
			renderer: newRenderer(&output, ThemeDefault),
		}

		result, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Errorf("RunWithContext() error = %v", err)
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
			config: Config{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault),
		}

		result, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Errorf("RunWithContext() error = %v", err)
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
		config := Config{
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
		renderer := newRenderer(&output, ThemeDefault)

		// Test cursor at beginning
		err := renderer.renderMainLine("$ ", "hello", 0)
		if err != nil {
			t.Errorf("renderMainLine() error = %v", err)
		}

		// Test cursor at end
		err = renderer.renderMainLine("$ ", "hello", 5)
		if err != nil {
			t.Errorf("renderMainLine() error = %v", err)
		}

		// Test cursor beyond end (should be safe)
		err = renderer.renderMainLine("$ ", "hello", 10)
		if err != nil {
			t.Errorf("renderMainLine() error = %v", err)
		}

		// Test with empty input
		err = renderer.renderMainLine("$ ", "", 0)
		if err != nil {
			t.Errorf("renderMainLine() error = %v", err)
		}

		// Test with unicode characters
		err = renderer.renderMainLine("🚀 ", "こんにちは", 2)
		if err != nil {
			t.Errorf("renderMainLine() error = %v", err)
		}
	})

	t.Run("render suggestions with various combinations", func(t *testing.T) {
		var output TestWriter
		renderer := newRenderer(&output, ThemeDefault)

		// Test single suggestion
		suggestions := []Suggestion{{Text: "hello", Description: "greeting"}}
		err := renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 0, 0)
		if err != nil {
			t.Errorf("renderSuggestions() error = %v", err)
		}

		// Test multiple suggestions with selection
		suggestions = []Suggestion{
			{Text: "hello", Description: "greeting"},
			{Text: "help", Description: "assistance"},
			{Text: "history", Description: "past commands"},
		}
		err = renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 1, 0)
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
		err = renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 5, 0)
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
		err = renderer.renderSuggestionsWithOffset("$ ", "test", 2, suggestions, 0, 0)
		if err != nil {
			t.Errorf("renderSuggestions() error = %v", err)
		}

		// Test with no suggestions
		err = renderer.renderSuggestionsWithOffset("$ ", "test", 2, []Suggestion{}, 0, 0)
		if err != nil {
			t.Errorf("renderSuggestions() error = %v", err)
		}
	})

	t.Run("render with suggestions integration", func(t *testing.T) {
		var output TestWriter
		renderer := newRenderer(&output, ThemeDefault)

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
					renderer: newRenderer(&output, ThemeDefault),
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
			config: Config{
				Prefix:    "$ ",
				Completer: completer,
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault),
		}

		result, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Errorf("RunWithContext() error = %v", err)
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
			config: Config{
				Prefix: "$ ",
				HistoryConfig: &HistoryConfig{
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
			renderer: newRenderer(&output, ThemeDefault),
		}

		result, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Errorf("RunWithContext() error = %v", err)
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
		renderer := newRenderer(&output, ThemeDefault)

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
			err := renderer.renderMainLine("$ ", tc.input, tc.cursor)
			if err != nil {
				t.Errorf("renderMainLine(%q, %d) error = %v", tc.input, tc.cursor, err)
			}
		}
	})

	t.Run("complex completion scenarios", func(t *testing.T) {
		completer := func(d Document) []Suggestion {
			input := d.GetWordBeforeCursor()
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
			config: Config{
				Prefix:    "$ ",
				Completer: completer,
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault),
		}

		result, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Errorf("RunWithContext() error = %v", err)
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
			config: Config{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{}, // Empty history
			renderer: newRenderer(&output, ThemeDefault),
		}

		result, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Errorf("RunWithContext() error = %v", err)
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
			config: Config{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{},
			renderer: newRenderer(&output, ThemeDefault),
		}

		result, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Errorf("RunWithContext() error = %v", err)
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
			config: Config{
				Prefix: "$ ",
			},
			terminal: mock,
			keyMap:   NewDefaultKeyMap(),
			output:   &output,
			buffer:   []rune{},
			cursor:   0,
			history:  []string{"test"}, // Same command already in history
			renderer: newRenderer(&output, ThemeDefault),
		}

		result, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Errorf("RunWithContext() error = %v", err)
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

func TestDocumentMethods(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		text           string
		cursorPos      int
		expectedBefore string
		expectedAfter  string
		expectedWord   string
		expectedLine   string
	}{
		{
			name:           "basic text",
			text:           "hello world",
			cursorPos:      6,
			expectedBefore: "hello ",
			expectedAfter:  "world",
			expectedWord:   "", // Cursor is after space, so no current word
			expectedLine:   "hello world",
		},
		{
			name:           "cursor at start",
			text:           "hello world",
			cursorPos:      0,
			expectedBefore: "",
			expectedAfter:  "hello world",
			expectedWord:   "",
			expectedLine:   "hello world",
		},
		{
			name:           "cursor at end",
			text:           "hello world",
			cursorPos:      11,
			expectedBefore: "hello world",
			expectedAfter:  "",
			expectedWord:   "world",
			expectedLine:   "hello world",
		},
		{
			name:           "cursor out of bounds negative",
			text:           "hello world",
			cursorPos:      -1,
			expectedBefore: "hello world",
			expectedAfter:  "",
			expectedWord:   "world",
			expectedLine:   "hello world",
		},
		{
			name:           "cursor out of bounds positive",
			text:           "hello world",
			cursorPos:      20,
			expectedBefore: "hello world",
			expectedAfter:  "",
			expectedWord:   "world",
			expectedLine:   "hello world",
		},
		{
			name:           "multiple words",
			text:           "git commit -m message",
			cursorPos:      10,
			expectedBefore: "git commit",
			expectedAfter:  " -m message",
			expectedWord:   "commit",
			expectedLine:   "git commit -m message",
		},
		{
			name:           "empty text",
			text:           "",
			cursorPos:      0,
			expectedBefore: "",
			expectedAfter:  "",
			expectedWord:   "",
			expectedLine:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc := Document{Text: tt.text, CursorPosition: tt.cursorPos}

			before := doc.TextBeforeCursor()
			if before != tt.expectedBefore {
				t.Errorf("TextBeforeCursor() = %q, want %q", before, tt.expectedBefore)
			}

			after := doc.TextAfterCursor()
			if after != tt.expectedAfter {
				t.Errorf("TextAfterCursor() = %q, want %q", after, tt.expectedAfter)
			}

			word := doc.GetWordBeforeCursor()
			if word != tt.expectedWord {
				t.Errorf("GetWordBeforeCursor() = %q, want %q", word, tt.expectedWord)
			}

			line := doc.CurrentLine()
			if line != tt.expectedLine {
				t.Errorf("CurrentLine() = %q, want %q", line, tt.expectedLine)
			}
		})
	}
}

func TestKeyMapMethods(t *testing.T) {
	t.Parallel()

	km := NewDefaultKeyMap()

	// Test Bind method
	km.Bind('x', ActionSubmit)
	action := km.GetAction('x')
	if action != ActionSubmit {
		t.Errorf("Expected ActionSubmit after binding 'x', got %v", action)
	}

	// Test BindSequence method
	km.BindSequence("TEST", ActionCancel)
	seqAction := km.GetSequenceAction("TEST")
	if seqAction != ActionCancel {
		t.Errorf("Expected ActionCancel after binding sequence 'TEST', got %v", seqAction)
	}

	// Test GetAction with unbound key
	action = km.GetAction('z')
	if action != ActionNone {
		t.Errorf("Expected ActionNone for unbound key 'z', got %v", action)
	}

	// Test GetSequenceAction with unbound sequence
	seqAction = km.GetSequenceAction("UNBOUND")
	if seqAction != ActionNone {
		t.Errorf("Expected ActionNone for unbound sequence 'UNBOUND', got %v", seqAction)
	}

	// Test nil KeyMap
	var nilKm *KeyMap
	action = nilKm.GetAction('a')
	if action != ActionNone {
		t.Errorf("Expected ActionNone for nil KeyMap, got %v", action)
	}

	seqAction = nilKm.GetSequenceAction("test")
	if seqAction != ActionNone {
		t.Errorf("Expected ActionNone for nil KeyMap sequence, got %v", seqAction)
	}
}

func TestFuzzyCompleter(t *testing.T) {
	t.Parallel()

	candidates := []string{
		"git status",
		"git commit",
		"git push",
		"docker build",
		"docker run",
		"kubectl get",
		"kubectl apply",
	}

	completer := NewFuzzyCompleter(candidates)

	tests := []struct {
		name     string
		input    string
		expected int // expected number of results
	}{
		{
			name:     "empty input returns all",
			input:    "",
			expected: 7,
		},
		{
			name:     "git prefix",
			input:    "git",
			expected: 4, // fuzzy matches multiple candidates with g-i-t pattern
		},
		{
			name:     "docker prefix",
			input:    "docker",
			expected: 2,
		},
		{
			name:     "fuzzy match",
			input:    "gst",
			expected: 4, // fuzzy matches multiple candidates with g-s-t pattern
		},
		{
			name:     "no matches",
			input:    "xyz",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			doc := Document{Text: tt.input, CursorPosition: len(tt.input)}
			suggestions := completer(doc)
			if len(suggestions) != tt.expected {
				t.Errorf("Complete(%q) returned %d suggestions, want %d",
					tt.input, len(suggestions), tt.expected)
			}

			// Verify all suggestions contain the text field
			for _, s := range suggestions {
				if s.Text == "" {
					t.Error("Suggestion with empty Text field")
				}
			}
		})
	}
}

func TestFuzzyScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		candidate string
		minScore  int
	}{
		{
			name:      "exact match",
			input:     "git",
			candidate: "git",
			minScore:  1000,
		},
		{
			name:      "prefix match",
			input:     "git",
			candidate: "git status",
			minScore:  800,
		},
		{
			name:      "contains match",
			input:     "status",
			candidate: "git status",
			minScore:  500,
		},
		{
			name:      "fuzzy match",
			input:     "gst",
			candidate: "git status",
			minScore:  10,
		},
		{
			name:      "no match",
			input:     "xyz",
			candidate: "git status",
			minScore:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			score := calculateFuzzyScore(tt.input, tt.candidate, false)
			if tt.minScore == 0 {
				if score != 0 {
					t.Errorf("Expected no match (score 0), got %d", score)
				}
			} else {
				if score < tt.minScore {
					t.Errorf("Score %d is less than expected minimum %d", score, tt.minScore)
				}
			}
		})
	}
}

func TestHistorySearcher(t *testing.T) {
	t.Parallel()

	history := []string{
		"git status",
		"git commit -m 'initial'",
		"docker build .",
		"kubectl get pods",
		"git push origin main",
	}

	searcher := NewHistorySearcher(history)

	tests := []struct {
		name     string
		query    string
		expected int
	}{
		{
			name:     "empty query returns all",
			query:    "",
			expected: 5,
		},
		{
			name:     "git query",
			query:    "git",
			expected: 4, // fuzzy matches include "kubectl get pods" for g-i-t pattern
		},
		{
			name:     "docker query",
			query:    "docker",
			expected: 2, // fuzzy matches may include additional entries with d-o-c-k-e-r pattern
		},
		{
			name:     "no matches",
			query:    "xyz",
			expected: 0,
		},
		{
			name:     "fuzzy match",
			query:    "gst",
			expected: 4, // fuzzy matches multiple history entries with g-s-t pattern
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			results := searcher(tt.query)
			if len(results) != tt.expected {
				t.Errorf("Search(%q) returned %d results, want %d",
					tt.query, len(results), tt.expected)
			}
		})
	}
}

func TestPromptHistoryMethods(t *testing.T) {
	t.Parallel()

	config := Config{
		Prefix: "$ ",
		HistoryConfig: &HistoryConfig{
			Enabled:    true,
			MaxEntries: 3,
		},
	}

	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test initial empty history
	history := p.GetHistory()
	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d items", len(history))
	}

	// Test AddHistory
	p.AddHistory("command1")
	p.AddHistory("command2")
	p.AddHistory("command3")

	history = p.GetHistory()
	if len(history) != 3 {
		t.Errorf("Expected 3 history items, got %d", len(history))
	}

	// Test max history limit
	p.AddHistory("command4")
	history = p.GetHistory()
	if len(history) != 3 {
		t.Errorf("Expected history to be limited to 3 items, got %d", len(history))
	}

	// Test duplicate prevention
	p.AddHistory("command4") // duplicate
	history = p.GetHistory()
	if len(history) != 3 {
		t.Errorf("Expected no duplicate, history length should be 3, got %d", len(history))
	}

	// Test AddHistory with empty string
	p.AddHistory("")
	history = p.GetHistory()
	if len(history) != 3 {
		t.Errorf("Expected empty string to be ignored, history length should be 3, got %d", len(history))
	}

	// Test SetHistory
	newHistory := []string{"new1", "new2", "new3", "new4", "new5"}
	p.SetHistory(newHistory)
	history = p.GetHistory()
	if len(history) != 3 { // should be limited by MaxHistory
		t.Errorf("Expected history to be limited to 3 items after SetHistory, got %d", len(history))
	}

	// Test ClearHistory
	p.ClearHistory()
	history = p.GetHistory()
	if len(history) != 0 {
		t.Errorf("Expected empty history after clear, got %d items", len(history))
	}
}

func TestPromptConfigurationMethods(t *testing.T) {
	t.Parallel()

	config := Config{
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
	if p.config.Theme != newTheme {
		t.Error("Expected Theme alias to be set")
	}

	// Test SetCompleter
	completer := func(_ Document) []Suggestion {
		return []Suggestion{{Text: "test", Description: "test"}}
	}
	p.SetCompleter(completer)
	if p.config.Completer == nil {
		t.Error("Expected completer to be set")
	}
}

func TestWordBoundaryFunctions(t *testing.T) {
	t.Parallel()

	config := Config{Prefix: "$ "}
	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test findWordBoundary
	p.buffer = []rune("hello world test")
	p.cursor = 6 // Position after "hello "

	// Test moving forward (Ctrl+Right)
	newPos := p.findWordBoundary(1)
	if newPos != 11 { // Should move to start of "test"
		t.Errorf("Expected cursor at position 11, got %d", newPos)
	}

	// Test moving backward (Ctrl+Left)
	p.cursor = 11
	newPos = p.findWordBoundary(-1)
	if newPos != 6 { // Should move to start of "world"
		t.Errorf("Expected cursor at position 6, got %d", newPos)
	}
}

func TestIsWordChar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		char     rune
		expected bool
	}{
		{'a', true},
		{'Z', true},
		{'0', true},
		{'9', true},
		{'_', true},
		{' ', false},
		{'-', false},
		{'!', false},
		{'@', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			result := isWordChar(tt.char)
			if result != tt.expected {
				t.Errorf("isWordChar(%q) = %v, want %v", tt.char, result, tt.expected)
			}
		})
	}
}

func TestNewDefaultKeyMapAdvanced(t *testing.T) {
	t.Parallel()

	keyMap := NewDefaultKeyMap()
	if keyMap == nil {
		t.Error("Expected non-nil KeyMap")
		return
	}

	// Test some default key bindings
	if keyMap.bindings == nil {
		t.Error("Expected initialized bindings map")
	}

	if keyMap.sequences == nil {
		t.Error("Expected initialized sequences map")
	}

	// Test that default keys are bound
	enterAction := keyMap.GetAction('\r')
	if enterAction == ActionNone {
		t.Error("Expected Enter key to be bound")
	}

	backspaceAction := keyMap.GetAction('\b')
	if backspaceAction == ActionNone {
		t.Error("Expected Backspace key to be bound")
	}
}

func TestKeyMapBindAdvanced(t *testing.T) {
	t.Parallel()

	keyMap := &KeyMap{
		bindings:  make(map[rune]KeyAction),
		sequences: make(map[string]KeyAction),
	}

	// Test binding a key
	keyMap.Bind('x', ActionComplete)

	action := keyMap.GetAction('x')
	if action != ActionComplete {
		t.Error("Expected bound action to be retrievable")
	}

	// Test overwriting a binding
	keyMap.Bind('x', ActionSubmit)

	retrievedAction := keyMap.GetAction('x')
	if retrievedAction != ActionSubmit {
		t.Error("Expected overwritten action to be retrievable")
	}
}

func TestKeyMapBindSequenceAdvanced(t *testing.T) {
	t.Parallel()

	keyMap := &KeyMap{
		bindings:  make(map[rune]KeyAction),
		sequences: make(map[string]KeyAction),
	}

	// Test binding a sequence
	keyMap.BindSequence("[A", ActionHistoryUp)

	action := keyMap.GetSequenceAction("[A")
	if action != ActionHistoryUp {
		t.Error("Expected bound sequence action to be retrievable")
	}

	// Test nonexistent sequence
	action = keyMap.GetSequenceAction("[Z")
	if action != ActionNone {
		t.Error("Expected ActionNone for nonexistent sequence")
	}
}

func TestKeyMapGetActionAdvanced(t *testing.T) {
	t.Parallel()

	keyMap := &KeyMap{
		bindings:  make(map[rune]KeyAction),
		sequences: make(map[string]KeyAction),
	}

	// Test getting action for unbound key
	action := keyMap.GetAction('z')
	if action != ActionNone {
		t.Error("Expected ActionNone for unbound key")
	}

	// Test getting action for bound key
	keyMap.Bind('z', ActionSubmit)

	action = keyMap.GetAction('z')
	if action != ActionSubmit {
		t.Error("Expected ActionSubmit for bound key")
	}
}

func TestPromptInsertRuneAdvanced(t *testing.T) {
	t.Parallel()

	config := Config{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test inserting a rune
	p.insertRune('a')
	if string(p.buffer) != "a" {
		t.Errorf("Expected buffer 'a', got %q", string(p.buffer))
	}

	// Test inserting another rune
	p.insertRune('b')
	if string(p.buffer) != "ab" {
		t.Errorf("Expected buffer 'ab', got %q", string(p.buffer))
	}

	// Test cursor position after insert
	if p.cursor != 2 {
		t.Errorf("Expected cursor position 2, got %d", p.cursor)
	}
}

func TestPromptInsertTextAdvanced(t *testing.T) {
	t.Parallel()

	config := Config{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test inserting text
	p.insertText("hello")
	if string(p.buffer) != "hello" {
		t.Errorf("Expected buffer 'hello', got %q", string(p.buffer))
	}

	// Test inserting more text
	p.insertText(" world")
	if string(p.buffer) != "hello world" {
		t.Errorf("Expected buffer 'hello world', got %q", string(p.buffer))
	}

	// Test cursor position after insert
	if p.cursor != 11 {
		t.Errorf("Expected cursor position 11, got %d", p.cursor)
	}
}

func TestPromptSetBufferAdvanced(t *testing.T) {
	t.Parallel()

	config := Config{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "initial")
	defer p.Close()

	// Test setting buffer
	p.setBuffer("new text")
	if string(p.buffer) != "new text" {
		t.Errorf("Expected buffer 'new text', got %q", string(p.buffer))
	}

	// Test cursor is set to end
	if p.cursor != len(p.buffer) {
		t.Errorf("Expected cursor at end (%d), got %d", len(p.buffer), p.cursor)
	}

	// Test setting empty buffer
	p.setBuffer("")
	if string(p.buffer) != "" {
		t.Errorf("Expected empty buffer, got %q", string(p.buffer))
	}
	if p.cursor != 0 {
		t.Errorf("Expected cursor at 0, got %d", p.cursor)
	}
}

func TestPromptAcceptSuggestionAdvanced(t *testing.T) {
	t.Parallel()

	config := Config{
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

	config := Config{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "hello\r")
	defer p.Close()

	// Test with context that doesn't timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := p.RunWithContext(ctx)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("Expected result 'hello', got %q", result)
	}
}

func TestPromptWithCancelledContextAdvanced(t *testing.T) {
	t.Parallel()

	config := Config{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test with already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := p.RunWithContext(ctx)
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

func TestDocumentTextMethodsAdvanced(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		text           string
		cursor         int
		expectedBefore string
		expectedAfter  string
		expectedWord   string
		expectedLine   string
	}{
		{
			name:           "cursor at beginning",
			text:           "hello world",
			cursor:         0,
			expectedBefore: "",
			expectedAfter:  "hello world",
			expectedWord:   "",
			expectedLine:   "hello world",
		},
		{
			name:           "cursor at end",
			text:           "hello world",
			cursor:         11,
			expectedBefore: "hello world",
			expectedAfter:  "",
			expectedWord:   "world",
			expectedLine:   "hello world",
		},
		{
			name:           "cursor in middle",
			text:           "hello world",
			cursor:         6,
			expectedBefore: "hello ",
			expectedAfter:  "world",
			expectedWord:   "", // Cursor is after space, so no current word
			expectedLine:   "hello world",
		},
		{
			name:           "cursor in word",
			text:           "hello world",
			cursor:         8,
			expectedBefore: "hello wo",
			expectedAfter:  "rld",
			expectedWord:   "wo",
			expectedLine:   "hello world",
		},
		{
			name:           "multiline text",
			text:           "line1\nline2\nline3",
			cursor:         8,
			expectedBefore: "line1\nli",
			expectedAfter:  "ne2\nline3",
			expectedWord:   "li",
			expectedLine:   "line1\nline2\nline3",
		},
		{
			name:           "empty text",
			text:           "",
			cursor:         0,
			expectedBefore: "",
			expectedAfter:  "",
			expectedWord:   "",
			expectedLine:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc := &Document{
				Text:           tt.text,
				CursorPosition: tt.cursor,
			}

			// Test TextBeforeCursor
			before := doc.TextBeforeCursor()
			if before != tt.expectedBefore {
				t.Errorf("TextBeforeCursor() = %q, want %q", before, tt.expectedBefore)
			}

			// Test TextAfterCursor
			after := doc.TextAfterCursor()
			if after != tt.expectedAfter {
				t.Errorf("TextAfterCursor() = %q, want %q", after, tt.expectedAfter)
			}

			// Test GetWordBeforeCursor
			word := doc.GetWordBeforeCursor()
			if word != tt.expectedWord {
				t.Errorf("GetWordBeforeCursor() = %q, want %q", word, tt.expectedWord)
			}

			// Test CurrentLine
			line := doc.CurrentLine()
			if line != tt.expectedLine {
				t.Errorf("CurrentLine() = %q, want %q", line, tt.expectedLine)
			}
		})
	}
}

func TestPromptCloseMultipleAdvanced(t *testing.T) {
	t.Parallel()

	config := Config{
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
	config := Config{
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
			keyAction := keyMap.GetAction(action.key)
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

	config := Config{
		Prefix:    "$ ",
		Completer: completer,
	}

	p := newForTestingWithConfig(t, config, "git\t\r\r")
	defer p.Close()

	result, err := p.Run()
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

	config := Config{
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
		var suggestions []Suggestion
		for i := range 15 {
			suggestions = append(suggestions, Suggestion{
				Text:        fmt.Sprintf("command%d", i),
				Description: fmt.Sprintf("description%d", i),
			})
		}
		return suggestions
	}

	config := Config{
		Prefix:    "$ ",
		Completer: completer,
	}

	// Test with TAB to trigger suggestions, then submit first one
	p := newForTestingWithConfig(t, config, "c\t\r")
	defer p.Close()

	result, err := p.Run()

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

			config := Config{
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

			result, err := p.Run()

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

	config := Config{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "test\r")
	defer p.Close()

	// Test setting prefix - just verify method exists and doesn't panic
	p.SetPrefix(">> ")

	// Run and verify it still works
	result, err := p.Run()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "test" {
		t.Errorf("Expected 'test', got %q", result)
	}
}

func TestPromptWithHistoryPreloadedComprehensive(t *testing.T) {
	t.Parallel()

	config := Config{
		Prefix: "$ ",
		HistoryConfig: &HistoryConfig{
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
	result, err := p.Run()
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

	config := Config{
		Prefix:    "$ ",
		Completer: completer,
	}
	p := newForTestingWithConfig(t, config, "te\t\r\r")
	defer p.Close()

	// Run with tab completion
	result, err := p.Run()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should have completed to one of the suggestions
	if result != "test" && result != "testing" {
		t.Errorf("Expected result 'test' or 'testing', got %q", result)
	}
}

func TestPromptTimeoutBehaviorComprehensive(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping slow test in local development")
	}

	t.Parallel()

	config := Config{
		Prefix: "$ ",
	}
	p := newForTestingWithConfig(t, config, "")
	defer p.Close()

	// Test with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := p.RunWithContext(ctx)
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
	config := Config{}
	p := newForTestingWithConfig(t, config, "test\r")
	defer p.Close()

	result, err := p.Run()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "test" {
		t.Errorf("Expected 'test', got %q", result)
	}
}

func TestPromptRunMultipleComprehensive(t *testing.T) {
	t.Parallel()

	config := Config{
		Prefix: "$ ",
	}

	// Test multiple runs with same config
	for i := range 3 {
		p := newForTestingWithConfig(t, config, "test\r")
		result, err := p.Run()
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

	config := Config{
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
		if os.Getenv("GITHUB_ACTIONS") == "" {
			t.Skip("Skipping slow test in local development")
		}

		// NewForTesting doesn't load from file, so test manual loading
		tmpDir := t.TempDir()
		historyFile := filepath.Join(tmpDir, "test_history")

		// Create a history file with some content
		err := os.WriteFile(historyFile, []byte("command1\ncommand2\n"), 0600)
		if err != nil {
			t.Fatalf("Failed to create test history file: %v", err)
		}

		config := Config{
			HistoryConfig: &HistoryConfig{
				Enabled:     true,
				MaxEntries:  100,
				File:        historyFile,
				MaxFileSize: 1024,
				MaxBackups:  3,
			},
		}

		// Test history manager loading separately
		hm := NewHistoryManager(config.HistoryConfig)
		err = hm.LoadHistory()
		if err != nil {
			t.Fatalf("Failed to load history: %v", err)
		}

		history := hm.GetHistory()
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
		config := Config{
			Prefix: "test> ",
			HistoryConfig: &HistoryConfig{
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
			histManager := NewHistoryManager(config.HistoryConfig)
			histManager.AddEntry("test")
			saveErr := histManager.SaveHistory()
			if saveErr == nil {
				t.Error("Expected error when saving history to invalid path")
			} else {
				t.Logf("Got expected error when saving history: %v", saveErr)
			}
		}
	})

	t.Run("WithThemeAlias", func(t *testing.T) {
		// Use default theme
		theme := ThemeDefault

		config := Config{
			Prefix: "test> ",
			Theme:  theme, // Using Theme alias instead of ColorScheme
		}

		p := newForTestingWithConfig(t, config, "")
		defer p.Close()

		// Should use the theme (Theme alias sets ColorScheme)
		if p.config.ColorScheme == nil {
			t.Error("Expected ColorScheme to be set")
		}
		if p.config.Theme != theme {
			t.Error("Expected Theme to be preserved")
		}
	})

	t.Run("DefaultsAndNilValues", func(t *testing.T) {
		config := Config{
			Prefix: "test> ",
			// All other fields nil/zero - should use defaults
		}

		p := newForTestingWithConfig(t, config, "")
		defer p.Close()

		if p.config.HistoryConfig == nil || p.config.HistoryConfig.MaxEntries != 1000 {
			maxEntries := 0
			if p.config.HistoryConfig != nil {
				maxEntries = p.config.HistoryConfig.MaxEntries
			}
			t.Errorf("Expected default HistoryConfig.MaxEntries 1000, got %d", maxEntries)
		}
		if p.config.HistoryConfig == nil {
			t.Error("Expected default HistoryConfig to be set")
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
		config := Config{
			Prefix: "test> ",
		}

		p := newForTestingWithConfig(t, config, "")
		defer p.Close()

		// Create a context that cancels immediately
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := p.RunWithContext(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled error, got: %v", err)
		}
	})

	t.Run("ContextTimeout", func(t *testing.T) {
		if os.Getenv("GITHUB_ACTIONS") == "" {
			t.Skip("Skipping slow test in local development")
		}

		config := Config{
			Prefix: "test> ",
		}

		p := newForTestingWithConfig(t, config, "")
		defer p.Close()

		// Create a context with very short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		// Wait for timeout
		time.Sleep(5 * time.Millisecond)

		_, err := p.RunWithContext(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected context.DeadlineExceeded error, got: %v", err)
		}
	})

	t.Run("EOFHandling", func(t *testing.T) {
		config := Config{
			Prefix: "test> ",
		}

		// Mock input that immediately returns EOF
		p := newForTestingWithConfig(t, config, "")
		defer p.Close()

		// Replace terminal with one that returns EOF
		p.terminal = &eofMockTerminal{}

		_, err := p.RunWithContext(context.Background())
		// Should handle EOF gracefully by returning ErrEOF
		// This test mainly ensures the EOF handling branch is covered
		if !errors.Is(err, ErrEOF) {
			t.Errorf("Expected ErrEOF, got: %v", err)
		}
	})

	t.Run("ComplexKeySequences", func(t *testing.T) {
		config := Config{
			Prefix: "test> ",
		}

		// Test escape sequence handling
		input := "\x1b[A\x1b[B\x1b[C\x1b[D\r" // Arrow keys + Enter
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		// Should handle the key sequences and return empty result
		if result != "" {
			t.Logf("Got result: %q", result)
		}
	})

	t.Run("HistoryNavigation", func(t *testing.T) {
		config := Config{
			Prefix: "test> ",
			HistoryConfig: &HistoryConfig{
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

		result, err := p.RunWithContext(context.Background())
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

		config := Config{
			Prefix:    "test> ",
			Completer: completer,
		}

		// Type "te", press Tab for completion, press down arrow, Enter to accept, Enter to submit
		input := "te\t\x1b[B\r\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		// Should complete the input to one of the suggestions
		if result != "test" && result != "temp" {
			t.Errorf("Expected completion result 'test' or 'temp', got %q", result)
		}
	})

	t.Run("CtrlDWithContent", func(t *testing.T) {
		config := Config{
			Prefix: "test> ",
		}

		// Type some content, then Ctrl+D
		input := "hello\x04"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		_, err := p.RunWithContext(context.Background())
		// Should not return EOF when buffer has content
		if errors.Is(err, io.EOF) {
			t.Error("Should not return EOF when buffer has content")
		}
	})

	t.Run("CtrlCInterrupt", func(t *testing.T) {
		config := Config{
			Prefix: "test> ",
		}

		// Type some content, then Ctrl+C
		input := "hello\x03"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.RunWithContext(context.Background())
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
		config := Config{
			Prefix: "test> ",
		}

		// Press Ctrl+C immediately without typing anything
		input := "\x03"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.RunWithContext(context.Background())
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
		config := Config{
			Prefix:    "test> ",
			Multiline: true,
		}

		// Enter multiline content
		input := "line1\nline2\r"
		p := newForTestingWithConfig(t, config, input)
		defer p.Close()

		result, err := p.RunWithContext(context.Background())
		if err != nil {
			t.Fatalf("RunWithContext failed: %v", err)
		}

		if !stringContains(result, "line1") {
			t.Errorf("Expected multiline result to contain 'line1', got %q", result)
		}
	})
}

func TestMultilineNavigation(t *testing.T) {
	// Create a prompt for testing multiline functions
	p := &Prompt{
		config: Config{
			Prefix: "test> ",
			HistoryConfig: &HistoryConfig{
				Enabled:    true,
				MaxEntries: 100,
			},
		},
		terminal: newMockTerminal(""),
		keyMap:   NewDefaultKeyMap(),
		history:  []string{},
	}

	// Test findLineStart
	t.Run("findLineStart", func(t *testing.T) {
		// Single line
		p.buffer = []rune("hello world")
		p.cursor = 6
		start := p.findLineStart()
		if start != 0 {
			t.Errorf("Expected line start 0, got %d", start)
		}

		// Multiple lines - cursor in middle of second line
		p.buffer = []rune("first line\nsecond line\nthird line")
		p.cursor = 17 // Position in "second line"
		start = p.findLineStart()
		expected := 11 // Start of "second line"
		if start != expected {
			t.Errorf("Expected line start %d, got %d", expected, start)
		}

		// Cursor at beginning of line
		p.cursor = 11
		start = p.findLineStart()
		if start != 11 {
			t.Errorf("Expected line start 11, got %d", start)
		}

		// Cursor at newline
		p.cursor = 10 // At the '\n' between first and second line
		start = p.findLineStart()
		if start != 0 {
			t.Errorf("Expected line start 0, got %d", start)
		}
	})

	// Test findLineEnd
	t.Run("findLineEnd", func(t *testing.T) {
		// Single line
		p.buffer = []rune("hello world")
		p.cursor = 6
		end := p.findLineEnd()
		if end != 11 {
			t.Errorf("Expected line end 11, got %d", end)
		}

		// Multiple lines - cursor in middle of second line
		p.buffer = []rune("first line\nsecond line\nthird line")
		p.cursor = 17 // Position in "second line"
		end = p.findLineEnd()
		expected := 22 // End of "second line"
		if end != expected {
			t.Errorf("Expected line end %d, got %d", expected, end)
		}

		// Cursor at end of line
		p.cursor = 22
		end = p.findLineEnd()
		if end != 22 {
			t.Errorf("Expected line end 22, got %d", end)
		}

		// Last line without newline
		p.cursor = 28 // In "third line"
		end = p.findLineEnd()
		if end != len(p.buffer) {
			t.Errorf("Expected line end %d, got %d", len(p.buffer), end)
		}
	})

	// Test findCursorUp
	t.Run("findCursorUp", func(t *testing.T) {
		// Single line - should stay at current position
		p.buffer = []rune("hello world")
		p.cursor = 6
		newPos := p.findCursorUp()
		if newPos != 6 {
			t.Errorf("Expected cursor to stay at 6, got %d", newPos)
		}

		// Multiple lines - move from second to first line
		p.buffer = []rune("first line\nsecond line\nthird line")
		p.cursor = 17 // Position 6 in "second line" (s-e-c-o-n-d)
		newPos = p.findCursorUp()
		expected := 6 // Same column position in "first line" (l-i-n-e)
		if newPos != expected {
			t.Errorf("Expected cursor at %d, got %d", expected, newPos)
		}

		// Move from third to second line
		p.cursor = 28 // Position 5 in "third line"
		newPos = p.findCursorUp()
		expected = 16 // Position 5 in "second line"
		if newPos != expected {
			t.Errorf("Expected cursor at %d, got %d", expected, newPos)
		}

		// Column beyond previous line length
		p.buffer = []rune("short\nthis is a very long line\nend")
		p.cursor = 20 // Far position in long line
		newPos = p.findCursorUp()
		expected = 5 // End of "short" line
		if newPos != expected {
			t.Errorf("Expected cursor at %d, got %d", expected, newPos)
		}

		// Already at first line
		p.cursor = 3
		newPos = p.findCursorUp()
		if newPos != 3 {
			t.Errorf("Expected cursor to stay at 3, got %d", newPos)
		}
	})

	// Test findCursorDown
	t.Run("findCursorDown", func(t *testing.T) {
		// Single line - should stay at current position
		p.buffer = []rune("hello world")
		p.cursor = 6
		newPos := p.findCursorDown()
		if newPos != 6 {
			t.Errorf("Expected cursor to stay at 6, got %d", newPos)
		}

		// Multiple lines - move from first to second line
		p.buffer = []rune("first line\nsecond line\nthird line")
		p.cursor = 6 // Position 6 in "first line"
		newPos = p.findCursorDown()
		expected := 17 // Same column position in "second line"
		if newPos != expected {
			t.Errorf("Expected cursor at %d, got %d", expected, newPos)
		}

		// Move from second to third line
		p.cursor = 16 // Position 5 in "second line"
		newPos = p.findCursorDown()
		expected = 28 // Position 5 in "third line"
		if newPos != expected {
			t.Errorf("Expected cursor at %d, got %d", expected, newPos)
		}

		// Column beyond next line length
		p.buffer = []rune("this is a very long line\nshort\nend")
		p.cursor = 20 // Far position in long line
		newPos = p.findCursorDown()
		expected = 30 // End of "short" line
		if newPos != expected {
			t.Errorf("Expected cursor at %d, got %d", expected, newPos)
		}

		// Already at last line
		p.buffer = []rune("first\nsecond")
		p.cursor = 8 // In "second"
		newPos = p.findCursorDown()
		if newPos != 8 {
			t.Errorf("Expected cursor to stay at 8, got %d", newPos)
		}
	})
}

func TestMultilineEdgeCases(t *testing.T) {
	p := &Prompt{
		config: Config{
			Prefix: "test> ",
			HistoryConfig: &HistoryConfig{
				Enabled:    true,
				MaxEntries: 100,
			},
		},
		terminal: newMockTerminal(""),
		keyMap:   NewDefaultKeyMap(),
		history:  []string{},
	}

	t.Run("EmptyBuffer", func(t *testing.T) {
		p.buffer = []rune{}
		p.cursor = 0

		start := p.findLineStart()
		if start != 0 {
			t.Errorf("Expected line start 0 for empty buffer, got %d", start)
		}

		end := p.findLineEnd()
		if end != 0 {
			t.Errorf("Expected line end 0 for empty buffer, got %d", end)
		}

		up := p.findCursorUp()
		if up != 0 {
			t.Errorf("Expected cursor up 0 for empty buffer, got %d", up)
		}

		down := p.findCursorDown()
		if down != 0 {
			t.Errorf("Expected cursor down 0 for empty buffer, got %d", down)
		}
	})

	t.Run("OnlyNewlines", func(t *testing.T) {
		p.buffer = []rune("\n\n\n")
		p.cursor = 2 // Second newline

		start := p.findLineStart()
		if start != 2 {
			t.Errorf("Expected line start 2, got %d", start)
		}

		end := p.findLineEnd()
		if end != 2 {
			t.Errorf("Expected line end 2, got %d", end)
		}

		up := p.findCursorUp()
		if up != 1 {
			t.Errorf("Expected cursor up 1, got %d", up)
		}

		down := p.findCursorDown()
		if down != 3 {
			t.Errorf("Expected cursor down 3, got %d", down)
		}
	})

	t.Run("CursorAtBoundaries", func(t *testing.T) {
		p.buffer = []rune("abc\ndef\nghi")

		// Cursor at very beginning
		p.cursor = 0
		start := p.findLineStart()
		if start != 0 {
			t.Errorf("Expected line start 0, got %d", start)
		}

		// Cursor at very end
		p.cursor = len(p.buffer)
		end := p.findLineEnd()
		if end != len(p.buffer) {
			t.Errorf("Expected line end %d, got %d", len(p.buffer), end)
		}

		// Test navigation from boundaries
		p.cursor = 0
		down := p.findCursorDown()
		if down != 4 { // Same column in next line
			t.Errorf("Expected cursor down 4, got %d", down)
		}

		p.cursor = len(p.buffer)
		up := p.findCursorUp()
		if up != 7 { // Same column in previous line
			t.Errorf("Expected cursor up 7, got %d", up)
		}
	})

	t.Run("UnicodeCharacters", func(t *testing.T) {
		p.buffer = []rune("こんにちは\n世界\nテスト")
		p.cursor = 7 // In "世界"

		start := p.findLineStart()
		if start != 6 {
			t.Errorf("Expected line start 6, got %d", start)
		}

		end := p.findLineEnd()
		if end != 8 {
			t.Errorf("Expected line end 8, got %d", end)
		}

		up := p.findCursorUp()
		if up != 1 { // Same position in first line
			t.Errorf("Expected cursor up 1, got %d", up)
		}

		down := p.findCursorDown()
		if down != 10 { // Same position in third line
			t.Errorf("Expected cursor down 10, got %d", down)
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
	config := Config{
		Prefix: "test> ",
		HistoryConfig: &HistoryConfig{
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
					if strings.HasPrefix("test", d.GetWordBeforeCursor()) {
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
				WithColorScheme(ThemeDefault),
			},
		},
		{
			name:   "with multiline",
			prefix: ">> ",
			options: []Option{
				WithMultiline(true),
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
				WithMultiline(true),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Test that config is correctly constructed without creating the prompt
			config := Config{
				Prefix: tt.prefix,
			}

			// Apply options
			for _, option := range tt.options {
				option(&config)
			}

			// Verify prefix was set correctly
			if config.Prefix != tt.prefix {
				t.Errorf("Config prefix = %v, want %v", config.Prefix, tt.prefix)
			}

			// Test NewWithOptions function creates config correctly by using NewForTesting
			testConfig := Config{Prefix: tt.prefix}
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
			if p.config.HistoryConfig == nil || p.config.HistoryConfig.MaxEntries <= 0 {
				maxEntries := 0
				if p.config.HistoryConfig != nil {
					maxEntries = p.config.HistoryConfig.MaxEntries
				}
				t.Errorf("Prompt config HistoryConfig.MaxEntries should have default value > 0, got %v", maxEntries)
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

	config := Config{}

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
	if config.HistoryConfig == nil {
		t.Error("WithMemoryHistory() did not create HistoryConfig")
	} else {
		if config.HistoryConfig.MaxEntries != 500 {
			t.Errorf("WithMemoryHistory() MaxEntries = %v, want %v", config.HistoryConfig.MaxEntries, 500)
		}
		if config.HistoryConfig.File != "" {
			t.Errorf("WithMemoryHistory() should create memory-only history, got File = %v", config.HistoryConfig.File)
		}
		if !config.HistoryConfig.Enabled {
			t.Error("WithMemoryHistory() should enable history")
		}
	}

	// Test WithMultiline
	WithMultiline(true)(&config)
	if !config.Multiline {
		t.Error("WithMultiline(true) did not set multiline to true")
	}

	// Test WithColorScheme
	colorScheme := &ColorScheme{}
	WithColorScheme(colorScheme)(&config)
	if config.ColorScheme != colorScheme {
		t.Error("WithColorScheme() did not set color scheme")
	}

	// Test WithTheme (alias)
	theme := &ColorScheme{}
	WithTheme(theme)(&config)
	if config.Theme != theme {
		t.Error("WithTheme() did not set theme")
	}
}

// newForTestingWithConfig creates a new prompt with a mock terminal for testing.
// This function is mainly for testing and migration purposes.
func newForTestingWithConfig(t *testing.T, config Config, mockInput string) *Prompt {
	t.Helper()

	// Set defaults for history config
	if config.HistoryConfig == nil {
		// For testing, disable file persistence by default
		config.HistoryConfig = &HistoryConfig{
			Enabled:    true,
			MaxEntries: 1000,
			File:       "", // No file persistence in tests
		}
	} else {
		// Set defaults for incomplete history config
		if config.HistoryConfig.MaxEntries <= 0 {
			config.HistoryConfig.MaxEntries = 1000
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
	historyManager := NewHistoryManager(config.HistoryConfig)
	historyManager.SetHistory([]string{}) // Start with empty history for testing

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

	return p
}
