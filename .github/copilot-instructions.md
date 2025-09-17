# GitHub Copilot Instructions for prompt

## Project Overview

This is the `prompt` library - a modern, robust replacement for the unmaintained go-prompt library. It provides powerful interactive terminal prompts in Go with cross-platform support, auto-completion, command history, and customizable theming.

## Code Generation Guidelines

### 1. Follow Go Best Practices

```go
// ✅ Good: Proper error handling
func (p *Prompt) Run() (string, error) {
    if p == nil {
        return "", errors.New("prompt is nil")
    }
    // implementation
}

// ❌ Bad: Ignoring errors
func (p *Prompt) Run() string {
    // implementation without error handling
}
```

### 2. Use Functional Options Pattern

```go
// ✅ Good: Functional options for configuration
func WithCompleter(completer func(Document) []Suggestion) Option {
    return func(c *Config) {
        c.Completer = completer
    }
}

// ❌ Bad: Too many parameters
func New(prefix string, completer func(Document) []Suggestion, history []string, maxHistory int) (*Prompt, error)
```

### 3. Proper Resource Management

```go
// ✅ Good: Always provide Close method and defer pattern
func (p *Prompt) Close() error {
    if p.terminal != nil {
        return p.terminal.Close()
    }
    return nil
}

// Usage:
defer p.Close()
```

### 4. Cross-Platform Compatibility

```go
// ✅ Good: Use filepath.Join for paths
configPath := filepath.Join(homeDir, ".myapp_history")

// ❌ Bad: Unix-specific path separators
configPath := homeDir + "/.myapp_history"

// ✅ Good: Use proper line endings
fmt.Fprintf(w, "line1%sline2%s", "\n", "\n")

// ❌ Bad: Hardcoded line endings
fmt.Fprintf(w, "line1\nline2\n")
```

### 5. Interface-Based Design

```go
// ✅ Good: Use interfaces for testability
type terminalInterface interface {
    ReadKey() (Key, error)
    Size() (int, int, error)
    Close() error
}

// ✅ Good: Accept interfaces, return structs
func NewPrompt(terminal terminalInterface) *Prompt {
    return &Prompt{terminal: terminal}
}
```

### 6. Comprehensive Error Handling

```go
// ✅ Good: Specific error types
var (
    ErrEOF         = errors.New("EOF")
    ErrInterrupted = errors.New("interrupted")
)

// ✅ Good: Use errors.Is for checking
if errors.Is(err, prompt.ErrEOF) {
    // handle EOF
}
```

### 7. Thread Safety Awareness

```go
// ✅ Good: Document thread safety
// Prompt is not thread-safe. Do not share instances across goroutines.
type Prompt struct {
    // fields
}

// ✅ Good: Protect shared state
func (p *Prompt) addHistory(entry string) {
    p.historyMutex.Lock()
    defer p.historyMutex.Unlock()
    p.history = append(p.history, entry)
}
```

### 8. Testing Patterns

```go
// ✅ Good: Table-driven tests with clear names
func TestPrompt_Run(t *testing.T) {
    t.Parallel()

    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {
            name:     "simple input returns same string",
            input:    "hello",
            expected: "hello",
            wantErr:  false,
        },
        {
            name:     "EOF input returns ErrEOF",
            input:    "\x04", // Ctrl+D
            expected: "",
            wantErr:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            // test implementation
        })
    }
}
```

### 9. Documentation Standards

```go
// ✅ Good: Comprehensive documentation with examples
// NewFuzzyCompleter creates a completer that performs fuzzy matching
// against the provided candidates.
//
// Example:
//   commands := []string{"git status", "git commit", "docker run"}
//   completer := prompt.NewFuzzyCompleter(commands)
//   p, err := prompt.New("$ ", prompt.WithCompleter(completer))
func NewFuzzyCompleter(candidates []string) func(Document) []Suggestion {
    // implementation
}
```

### 10. Performance Considerations

```go
// ✅ Good: Efficient string building
var buf strings.Builder
for _, item := range items {
    buf.WriteString(item)
}
return buf.String()

// ❌ Bad: Inefficient string concatenation
result := ""
for _, item := range items {
    result += item
}
return result

// ✅ Good: Reuse slices when possible
func (p *Prompt) getSuggestions() []Suggestion {
    p.suggestions = p.suggestions[:0] // reset but keep capacity
    // populate suggestions
    return p.suggestions
}
```

## Common Patterns to Generate

### Configuration Option
```go
func WithOptionName(value Type) Option {
    return func(c *Config) {
        c.FieldName = value
    }
}
```

### Terminal Key Handling
```go
func (p *Prompt) handleKey(key Key) error {
    switch key.Type {
    case KeyTab:
        return p.handleComplete()
    case KeyCtrlC:
        return ErrInterrupted
    case KeyEnter:
        return p.handleSubmit()
    default:
        return p.handleInput(key)
    }
}
```

### History Management
```go
func (p *Prompt) addToHistory(entry string) {
    if entry == "" || (len(p.history) > 0 && p.history[len(p.history)-1] == entry) {
        return // don't add empty or duplicate entries
    }

    p.history = append(p.history, entry)
    if len(p.history) > p.config.MaxHistory {
        p.history = p.history[1:] // remove oldest entry
    }
}
```

### Color Theme Definition
```go
func NewColorScheme(name string) *ColorScheme {
    return &ColorScheme{
        Name:               name,
        Prefix:             "37", // white
        Input:              "37", // white
        Suggestion:         "90", // bright black
        SuggestionSelected: "7",  // reverse
        Description:        "90", // bright black
    }
}
```

## Anti-Patterns to Avoid

1. **Global variables** - Use dependency injection instead
2. **Panic on user input** - Always return errors gracefully
3. **Platform-specific code** without abstraction
4. **Memory leaks** - Always clean up resources
5. **Blocking operations** without context support
6. **Hardcoded strings** that should be configurable
7. **Complex constructors** - Use builder pattern or options
8. **Mutable shared state** without synchronization

## When Suggesting Code

1. **Always include error handling**
2. **Use existing types and interfaces when possible**
3. **Follow the functional options pattern for configuration**
4. **Include relevant documentation comments**
5. **Consider cross-platform compatibility**
6. **Suggest tests alongside implementation**
7. **Use meaningful variable and function names**
8. **Consider performance implications**
9. **Ensure proper resource cleanup**
10. **Follow the project's existing code style**

## Example Completions

When user types "func (p *Prompt) handle", suggest:
```go
func (p *Prompt) handleInput(key Key) error {
    switch key.Type {
    case KeyRune:
        p.insertRune(key.Rune)
        return nil
    case KeyBackspace:
        return p.handleBackspace()
    case KeyDelete:
        return p.handleDelete()
    default:
        return nil
    }
}
```

When user types "func TestPrompt", suggest table-driven test pattern with proper setup and cleanup.

When user types "type Config struct", suggest struct with proper field documentation and tags.