# Prompt Library Examples

This directory contains various examples demonstrating the capabilities of the prompt library.

## Examples Overview

| Example | Description | Key Features |
|---------|-------------|--------------|
| **basic** | Simple prompt usage | Basic input/output, error handling |
| **autocomplete** | Tab completion | Context-aware suggestions, filtered completions |
| **history** | Command history | Arrow key navigation, in-memory storage |
| **multiline** | Multi-line input | Line editing, block text input |
| **shell** | Interactive shell | File system navigation, command execution |

## Building and Running Examples

### Prerequisites

- Go 1.24.0 or higher
- Terminal with UTF-8 support
- ANSI color support (most modern terminals)

### Build Individual Example

```bash
# Navigate to example directory
cd example/basic

# Build the example
go build -o basic

# Run the example
./basic
```

### Build All Examples

From the project root:

```bash
# Build all examples
for dir in example/*/; do
    if [ -f "${dir}main.go" ]; then
        name=$(basename "$dir")
        echo "Building $name..."
        go build -o "example/$name/$name" "./example/$name"
    fi
done
```

### Run Examples Directly

You can also run examples without building:

```bash
# Run basic example
go run example/basic/main.go

# Run autocomplete example
go run example/autocomplete/main.go
```

## Example Details

### 1. Basic Example (`basic/`)

Demonstrates the simplest use case of the prompt library.

**Features:**
- Simple text input
- EOF handling (Ctrl+D)
- Exit commands
- Error handling

**Usage:**
```go
p, err := prompt.New(">>> ")
if err != nil {
    log.Fatal(err)
}
defer p.Close()

result, err := p.Run()
```

### 2. Autocomplete Example (`autocomplete/`)

Shows how to implement intelligent tab completion.

**Features:**
- Command suggestions
- Context-aware completions
- Filtered suggestions based on input
- Multi-level completions

**Key Concepts:**
- Implement a completer function: `func(Document) []Suggestion`
- Return `[]prompt.Suggestion` based on context
- Use manual prefix filtering with `strings.HasPrefix`

**Usage:**
```go
func myCompleter(d prompt.Document) []prompt.Suggestion {
    text := d.TextBeforeCursor()
    // Implement your completion logic here
    return []prompt.Suggestion{
        {Text: "command1", Description: "Description 1"},
        {Text: "command2", Description: "Description 2"},
    }
}

p, err := prompt.New("app> ",
    prompt.WithCompleter(myCompleter),
    prompt.WithColorScheme(prompt.ThemeNightOwl),
)
```

### 3. History Example (`history/`)

Demonstrates command history functionality.

**Features:**
- Up/Down arrow navigation
- History persistence
- Clear history command
- Display history list

**Controls:**
- ↑ (Up Arrow): Previous command
- ↓ (Down Arrow): Next command
- `history`: Show command history
- `clear`: Clear history

**Usage:**
```go
p, err := prompt.New("$ ",
    prompt.WithMemoryHistory(100),
)
```

### 4. Multiline Example (`multiline/`)

Demonstrates multi-line text input.

**Features:**
- Multi-line editing
- Line counting
- Character counting
- Block text processing

**Usage:**
- Enter text across multiple lines
- End input with `.` on a new line
- Or use Ctrl+D to finish

**Configuration:**
```go
p, err := prompt.New("> ",
    prompt.WithMultiline(true),
)
```

### 5. Shell Example (`shell/`)

An interactive shell implementation showcasing prompt capabilities.

**Features:**
- File system navigation
- Command execution
- Path-aware autocomplete
- Command history
- Dynamic prompt updates
- Error handling

**Commands:**
- `ls [path]` - List directory contents
- `cd <path>` - Change directory
- `pwd` - Print working directory
- `cat <file>` - Display file contents
- `mkdir <name>` - Create directory
- `touch <name>` - Create empty file
- `rm <path>` - Remove file or directory
- `clear` - Clear screen
- `help` - Show help
- `exit` - Exit the shell

## Common Patterns

### Creating a Prompt

```go
// Basic prompt
p, err := prompt.New(">>> ")

// Prompt with options
p, err := prompt.New(">>> ",
    prompt.WithCompleter(myCompleter),
    prompt.WithMemoryHistory(100),
    prompt.WithColorScheme(prompt.ThemeDark),
    prompt.WithMultiline(true),
)

if err != nil {
    log.Fatal(err)
}
defer p.Close()
```

### Running the Prompt Loop

```go
for {
    result, err := p.Run()
    if err != nil {
        if errors.Is(err, prompt.ErrEOF) {
            break
        }
        log.Printf("Error: %v\n", err)
        continue
    }
    // Process result
}
```

### Implementing a Completer

```go
func myCompleter(d prompt.Document) []prompt.Suggestion {
    text := d.TextBeforeCursor()
    words := strings.Fields(text)

    suggestions := []prompt.Suggestion{
        {Text: "command1", Description: "Description 1"},
        {Text: "command2", Description: "Description 2"},
    }

    // Manual prefix filtering
    if len(words) > 0 && !strings.HasSuffix(text, " ") {
        prefix := strings.ToLower(words[len(words)-1])
        filtered := []prompt.Suggestion{}
        for _, s := range suggestions {
            if strings.HasPrefix(strings.ToLower(s.Text), prefix) {
                filtered = append(filtered, s)
            }
        }
        return filtered
    }

    return suggestions
}
```

## Available Options

### WithCompleter
Set a completion function for tab completion:
```go
prompt.WithCompleter(func(d prompt.Document) []prompt.Suggestion {
    // Return completion suggestions
})
```

### WithMemoryHistory
Configure in-memory history:
```go
prompt.WithMemoryHistory(100) // Keep last 100 commands
```

### WithHistory
Configure persistent file-based history:
```go
prompt.WithHistory(&prompt.HistoryConfig{
    MaxEntries: 1000,
    File:       "~/.myapp_history",
})
```

### WithColorScheme
Set a color theme:
```go
prompt.WithColorScheme(prompt.ThemeNightOwl)
// Available themes: ThemeDefault, ThemeDark, ThemeLight,
// ThemeSolarizedDark, ThemeAccessible, ThemeVSCode,
// ThemeNightOwl, ThemeDracula, ThemeMonokai
```

### WithMultiline
Enable multiline input mode:
```go
prompt.WithMultiline(true)
```

## Keyboard Shortcuts

The prompt library supports standard terminal shortcuts:

| Shortcut | Action |
|----------|--------|
| Tab | Trigger autocomplete |
| ↑/↓ | Navigate history |
| ←/→ | Move cursor |
| Home/Ctrl+A | Move to line start |
| End/Ctrl+E | Move to line end |
| Ctrl+C | Cancel current input |
| Ctrl+D | Send EOF (exit) |
| Ctrl+L | Clear screen |
| Ctrl+U | Clear line before cursor |
| Ctrl+K | Clear line after cursor |
| Ctrl+W | Delete word before cursor |
| Alt+B | Move back one word |
| Alt+F | Move forward one word |

## Troubleshooting

### Windows Issues

If colors don't display correctly on Windows:
- The library uses `mattn/go-colorable` for Windows ANSI support
- Ensure your terminal supports ANSI colors (Windows Terminal, ConEmu, etc.)

### Terminal Size Issues

If the prompt doesn't detect terminal size correctly:
- Resize your terminal window
- The library uses `golang.org/x/term` for cross-platform terminal handling

### Input Issues

If special keys don't work:
- Ensure your terminal is in raw mode
- Some terminal emulators may not support all key combinations

## Helper Functions

The library provides several helper functions for common use cases:

### Fuzzy Completion
```go
completer := prompt.NewFuzzyCompleter([]string{
    "command1", "command2", "command3",
})
p, err := prompt.New(">>> ", prompt.WithCompleter(completer))
```

### File Completion
```go
completer := prompt.NewFileCompleter()
p, err := prompt.New(">>> ", prompt.WithCompleter(completer))
```

### History Search
```go
searcher := prompt.NewHistorySearcher([]string{
    "previous command 1",
    "previous command 2",
})
results := searcher("prev") // Returns matching history items
```

## Contributing

Feel free to add more examples! When creating a new example:

1. Create a new directory under `example/`
2. Add a descriptive `main.go`
3. Update this README with example details
4. Include comments explaining key concepts
5. Test on multiple platforms (Linux, macOS, Windows)

## License

These examples are part of the prompt library and follow the same license.