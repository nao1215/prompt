# prompt

prompt is a modern, robust replacement for the go-prompt library (github.com/c-bata/go-prompt), designed to provide powerful interactive terminal prompts in Go. This library addresses the longstanding issues and limitations of the original go-prompt while maintaining compatibility and adding new features.

## Project Context

The original go-prompt library (https://github.com/c-bata/go-prompt) has been unmaintained since March 2021, with 286 open issues and numerous critical bugs:
- **Divide by zero panics** in terminal rendering (issue #277)
- **Windows terminal compatibility issues** (issue #285)
- **File descriptor leaks** in /dev/tty handling (issue #253)
- **Limited TTY support** beyond STDIN (issue #275)
- **Terminal color reset issues** on application exit (issue #265)

Repository Stats (as of September 2025):
- 5,405 stars, 368 forks
- Last commit: March 3, 2021 (4+ years ago)
- Primary language: Go
- Created: August 14, 2017

## Codebase Information
### Development Commands
- `make test`: Run tests and measure coverage (generates cover.out file, viewable in browser with cover.html)
- `make lint`: Code inspection with golangci-lint (.golangci.yml configuration)
- `make clean`: Delete generated files
- `make tools`: Install dependency tools (golangci-lint, octocov)

### Key Features
- Interactive terminal prompts with rich editing capabilities
- Multi-line input support with proper cursor navigation
- Fuzzy auto-completion with intelligent ranking
- Command history with reverse search (Ctrl+R)
- Configurable key bindings and shortcuts
- Cross-platform compatibility (Windows, macOS, Linux)
- Context support for timeouts and cancellation
- Built-in color themes and customizable theming
- Resource management with proper cleanup

## Development Rules
- Test-Driven Development: We adopt the test-driven development promoted by t-wada (Takuto Wada). Always write test code and be mindful of the test pyramid.
- Working code: Ensure that `make test` and `make lint` succeed after completing work.
- Sponsor acquisition: Since development incurs financial costs, we seek sponsors via `https://github.com/sponsors/nao1215`. Include sponsor links in README and documentation.
- Contributor acquisition: Create developer documentation so anyone can participate in development and recruit contributors.
- Comments in English: Write code comments in English to accept international contributors.
- User-friendly documentation comments: Write detailed explanations and example code for public functions so users can understand usage at a glance.

## Coding Guidelines
- No global variables: Do not use global variables. Manage state through function arguments and return values.
- Coding rules: Follow Golang coding rules. [Effective Go](https://go.dev/doc/effective_go) is the basic rule.
- Package comments are mandatory: Describe the package overview in `doc.go` for each package. Clarify the purpose and usage of the package.
- Comments for public functions, variables, and struct fields are mandatory: When visibility is public, always write comments following go doc rules.
- Remove duplicate code: After completing your work, check if you have created duplicate code and remove unnecessary code.
- Error handling: Use `errors.Is` and `errors.As` for error interface equality checks. Never omit error handling.
- Documentation comments: Write documentation comments to help users understand how to use the code. In-code comments should explain why or why not something is done.
- Update README: When adding new features, update the README.
- CHANGELOG.md maintenance: When updating CHANGELOG.md, always include references to the relevant PR numbers and commit hashes with clickable GitHub links. This helps developers trace which specific changes were made in which PR/commit and allows them to browse the actual code changes. Format examples:
  - **Feature description ([abc1234](https://github.com/nao1215/prompt/commit/abc1234))**: Detailed explanation of the change
  - **Feature description (PR #123, [abc1234](https://github.com/nao1215/prompt/commit/abc1234))**: When both PR and commit are relevant
  - Use `git log --oneline` and GitHub PR numbers to identify the specific changes
  - Always format commit hashes as clickable links: `[hash](https://github.com/nao1215/prompt/commit/hash)`
  - This improves traceability and allows developers to browse code changes directly in their browser
  - Users want to see the actual implementation, so always provide GitHub links for commits

## Testing
- [Readable Test Code](https://logmi.jp/main/technology/327449): Avoid excessive optimization (DRY) and aim for a state where it's easy to understand what tests exist.
- Clear input/output: Create tests with `t.Run()` and clarify test case input/output. Test cases clarify test intent by explicitly showing input and expected output.
- Test descriptions: The first argument of `t.Run()` should clearly describe the relationship between input and expected output.
- Test granularity: Aim for 80% or higher coverage with unit tests.
- Parallel test execution: Use `t.Parallel()` to run tests in parallel whenever possible.
- Using `octocov`: Run `octocov` after `make test` to confirm test coverage exceeds 80%.
- Cross-platform support: Tests run on Linux, macOS, and Windows through GitHub Actions. Examples of non-cross-platform code include "concatenating paths without using `filepath.Join`" and "using "\n" for line breaks".
- Test data storage: Store sample files in various formats in the testdata directory

## Project Goals

1. **Modernize Architecture**: Redesign with better separation of concerns and testability
2. **Fix Critical Bugs**: Address all known panics, memory leaks, and platform issues
3. **Enhanced Cross-Platform Support**: Robust Windows, macOS, and Linux compatibility
4. **Improved API Design**: More intuitive and flexible API while maintaining backward compatibility where possible
5. **Comprehensive Testing**: Achieve >90% test coverage with robust cross-platform tests
6. **Better Documentation**: Provide clear examples and comprehensive API documentation

## Architectural Principles
- **Modular Design**: Clear separation between input, output, rendering, and completion
- **Interface-Based**: Use interfaces for testability and extensibility
- **Resource Management**: Proper cleanup of file descriptors and goroutines
- **Memory Safety**: Avoid memory leaks and race conditions
- **Error Recovery**: Graceful handling of terminal resize, signal interruption

## Current Analysis Summary

### Original go-prompt Architecture Issues
1. **Monolithic Design**: Large files with mixed responsibilities
2. **Platform-Specific Code**: Scattered across multiple files without clear abstraction
3. **Resource Leaks**: File descriptors not properly managed
4. **Render Logic Bugs**: Division by zero in terminal coordinate calculations
5. **Limited Extensibility**: Hard to extend for custom use cases

### Key Files in Current Implementation
- `prompt.go`: Main prompt logic and event loop with functional options API
- `terminal.go`: Cross-platform terminal interface abstraction
- `renderer.go`: Terminal rendering and cursor management
- `history.go`: Command history management with persistence
- `color_scheme.go`: Color theme definitions and customization
- `helpers.go`: Utility functions for completion and file operations
- `doc.go`: Package documentation with comprehensive examples

### Priority Issues Addressed
1. **Fixed divide by zero panic** through modernized rendering logic
2. **Implemented proper resource cleanup** with Close() method and defer patterns
3. **Improved Windows terminal support** using mattn/go-colorable
4. **Added configurable TTY support** through terminal interface abstraction
5. **Fixed terminal color reset** on application exit
6. **Enhanced error handling** with specific error types (ErrEOF, ErrInterrupted)

## 簡素化された実装戦略

### 設計方針の変更
**初期設計の問題:**
- パブリックAPIが50+と多すぎる
- 過度な抽象化による実装コストの増大
- 実際のプラットフォーム差異を無視した理想的設計

**新しいアプローチ:**
- **最小限のパブリックAPI**: 5つのコア型のみ公開
- **既存ライブラリ活用**: golang.org/x/term, mattn/go-colorable等を使用
- **段階的実装**: 1ヶ月で実用レベル完成
- **1:1移行サポート**: go-promptからの簡単な置換

### 新しい実装フェーズ

#### フェーズ1: 最小実装（完了）
- 基本的な入力/出力機能
- プラットフォーム抽象化（既存ライブラリ活用）
- エラー回復機能
- 基本的なテストフレームワーク

#### フェーズ2: 補完機能（完了）
- Tab補完サポート
- 候補一覧表示
- go-prompt互換API
- ファジー補完機能

#### フェーズ3: 履歴機能（完了）
- 矢印キーによる履歴ナビゲーション
- 履歴保存機能
- リバース検索（Ctrl+R）
- マイグレーションガイド

### 簡素化されたAPI設計

```go
// 簡素化された API (5つのコア型)
type Prompt struct { ... }
type Suggestion struct { Text, Description string }
type Option func(*Config)
func New(prefix string, options ...Option) (*Prompt, error)
func (p *Prompt) Run() (string, error)

// 使用例
p, err := prompt.New("$ ",
    prompt.WithCompleter(myCompleter),
    prompt.WithMemoryHistory(100),
)
```

### 実用的なクロスプラットフォーム対応
- Windows: mattn/go-colorable でANSI色対応
- Unix: golang.org/x/term でraw mode制御
- エラー回復: divide by zero問題等を実証済み手法で解決

## Terminal Support & Compatibility

### Supported Platforms
- **Linux**: Full support with native terminal capabilities
- **macOS**: Complete compatibility with Terminal.app and iTerm2
- **Windows**: Enhanced support via mattn/go-colorable for ANSI sequences

### Key Features Implementation
- **Raw Mode**: Platform-specific terminal mode handling
- **Signal Handling**: Proper cleanup on Ctrl+C and terminal resize
- **Color Support**: Automatic detection and fallback for limited terminals
- **Unicode Support**: Full UTF-8 character handling including wide characters

## Thread Safety & Concurrency

⚠️ **IMPORTANT**: This library is **NOT thread-safe**:
- **Do NOT** share prompt instances across goroutines
- **Do NOT** call methods concurrently on the same prompt instance
- **Do NOT** call `Close()` while `Run()` is active in another goroutine
- Use separate prompt instances for concurrent operations if needed

## API Design Philosophy

The library follows a functional options pattern for configuration:

```go
type Config struct {
    Prefix        string
    Completer     func(Document) []Suggestion
    HistoryConfig *HistoryConfig
    ColorScheme   *ColorScheme
    KeyMap        *KeyMap
    Multiline     bool
}

// Options pattern allows for clean, extensible API
func WithCompleter(completer func(Document) []Suggestion) Option
func WithMemoryHistory(maxEntries int) Option
func WithHistory(historyConfig *HistoryConfig) Option
func WithColorScheme(scheme *ColorScheme) Option
func WithKeyMap(keyMap *KeyMap) Option
```

## Performance Characteristics

- **Memory Usage**: Minimal allocations during normal operation
- **Rendering**: Efficient diff-based terminal updates
- **History**: Configurable limits with LRU eviction
- **Completion**: Lazy evaluation with fuzzy matching
- **I/O**: Non-blocking input with proper timeout handling

## Sponsor & Contribution Information

- **Sponsors**: https://github.com/sponsors/nao1215
- **Contributing**: See CONTRIBUTING.md for detailed guidelines
- **International Contributors**: All documentation and code comments in English