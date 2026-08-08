# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.18] - 2026-08-08

### Fixed
- Input that filled a terminal row exactly misplaced the cursor and erased the line above the prompt (PR [#34](https://github.com/nao1215/prompt/pull/34), [#30](https://github.com/nao1215/prompt/issues/30)): a terminal that has just written its last column holds the cursor there until another character arrives, so the row below it does not exist yet. The renderer counted the cursor onto that row anyway, which put it at column 0 on top of the text, and left the next redraw moving up one row too many — erasing whatever the application had printed above the prompt. In a shell whose prefix is a path, some statement length always lands on that boundary, so the row eaten was the bottom of the result the user was reading.
- A prompt prefix wider than the terminal left a stale copy of its first row behind (PR [#34](https://github.com/nao1215/prompt/pull/34), [#31](https://github.com/nao1215/prompt/issues/31)): the row count short-circuited to one whenever the input was empty, or when the first line held nothing but the prefix, and a wrapping prefix occupies more rows than that. The first keystroke of every line therefore cleared only the prefix's last row and redrew the whole prefix from there, orphaning the rows above it — one torn fragment per line entered.
- A wide character wrapped whole at the right margin shifted the cursor one cell left of the text (PR [#34](https://github.com/nao1215/prompt/pull/34), [#32](https://github.com/nao1215/prompt/issues/32)): a terminal never splits a glyph across the margin, so a double-width rune that does not fit moves to the next row and leaves the last cell blank. Row and column came from dividing total cells by the terminal width, which does not know about that blank cell, so the cursor drifted one cell per straddle and the block's height was undercounted. Both now come from walking the line a cell at a time.
- Ctrl+R drew a new search block per keystroke and left all of them on screen (PR [#34](https://github.com/nao1215/prompt/pull/34), [#33](https://github.com/nao1215/prompt/issues/33)): the search interface cleared one line and printed its header and results below the previous block, so typing a three-letter query stacked three copies, and nothing removed them when the search ended — the accepted line appeared at the foot of a column of dead results. The search now remembers the rows it drew, erases them before the next render, and erases the last of them however it returns.

## [0.0.17] - 2026-08-05

### Fixed
- **The prompt erased the last line of whatever was printed before it (PR [#28](https://github.com/nao1215/prompt/pull/28))**: the renderer remembers the block it drew so it can erase it on the next keystroke, and it still remembered it after the line was submitted. By then that block belongs to the finished line, and the application has printed its own output underneath it, so the next prompt moved up into that output and erased from there. One row up per row the entry had occupied: a single-line entry erased nothing extra, while a statement typed across a continuation line ate the last line of the result printed for it — a table lost its bottom border. The renderer now forgets the block when a line ends, so a fresh prompt erases only the line it is on.

## [0.0.16] - 2026-08-05

### Added
- **`WatchInterrupt` for work that runs between prompts (PR [#26](https://github.com/nao1215/prompt/pull/26), [3c3d896](https://github.com/nao1215/prompt/commit/3c3d896))**: `Run` returns as soon as a line is submitted, so while the application executes that line nothing is reading the terminal. In raw mode Ctrl+C is a byte rather than a signal, so it could not reach the running work at all: it waited in the input buffer and was read as part of the next line once the work was over. `WatchInterrupt` watches for it during that gap and returns a context canceled when the key arrives, which is what lets an embedding shell stop a long query. Everything else typed meanwhile is held and delivered to the next `Run` in the order it was typed, so typing ahead keeps working, and the terminal is read in one place so a watcher and the line editor cannot take half of a keystroke each.

## [0.0.15] - 2026-08-05

### Changed
- **Ctrl+C no longer releases the terminal in persistent raw mode (PR [#24](https://github.com/nao1215/prompt/pull/24))**: the interrupt ends the line, not the session, so a REPL that reports `ErrInterrupted` and calls `Run` again used to pay a raw-to-cooked-to-raw switch on every Ctrl+C — the same mode-switch window `WithPersistentRawMode` exists to close, reopened at the moment the user is typing. Raw mode is now released by `Close` or at EOF. An application that treats `ErrInterrupted` as fatal must call `Close`, as it should anyway. The default (non-persistent) mode is unchanged: every `Run` still restores the terminal before returning.

### Fixed
- **Pasted text was obeyed as keystrokes (PR [#24](https://github.com/nao1215/prompt/pull/24))**: a bracketed paste was inserted literally only for Enter. Every other byte still ran its key binding, so a TAB inside pasted text ran completion instead of reaching the buffer — it silently disappeared, and with a matching candidate the pasted word was rewritten (`SELECT ab<TAB>c` became `SELECT abcdefc`). A 0x03 carried in the pasted text ended the prompt with `ErrInterrupted`. Paste is now content: printable runes and TAB are inserted, other control bytes are dropped, and only the paste-end marker leaves paste mode.
- **A pasted CRLF became two line breaks (PR [#24](https://github.com/nao1215/prompt/pull/24))**: text copied on Windows arrives as CR LF and both bytes submit, so each line break in the paste inserted a blank line. A line break now yields exactly one "\n" however the terminal spells it.
- **Escape swallowed the characters typed after it (PR [#24](https://github.com/nao1215/prompt/pull/24))**: the escape reader consumed up to three more runes whatever they were, so a bare Escape — or Alt+key, or any unmapped ESC-prefixed key — ate the start of the next word. Pressing Escape and typing `SELECT 1` ran `ECT 1`. Only CSI (`[`) and SS3 (`O`) now introduce a sequence; anything else is pushed back for the read loop. A CSI sequence is also read to its real final byte instead of being cut after three runes.
- **Escape could not close the completion popup (PR [#24](https://github.com/nao1215/prompt/pull/24))**: with suggestions on screen Enter accepts one instead of submitting, and nothing dismissed them, so the popup could leave a line unrunnable. A bare Escape now closes it.

## [0.0.14] - 2026-08-05

### Fixed
- **The prompt climbed the screen when the cursor was not on the last line (PR [#23](https://github.com/nao1215/prompt/pull/23))**: a redraw erased the block it had drawn by moving up its height, which is where the cursor is only while it sits on the block's last row. A left arrow crossing a line break puts it on an earlier row, so every keystroke after that erased one row too high: the prompt moved up the screen and took a line of scrollback with it each time. The renderer now remembers the row it left the cursor on and erases from there. Cursor positioning is wrap-aware for the same reason — a line long enough to wrap put the cursor's column on an earlier row, and moving back by columns alone can never leave the row it is on, so the cursor stopped at the left margin of the wrong row.
- **A keystroke typed during a redraw could be swallowed, and every redraw cost 100ms (PR [#23](https://github.com/nao1215/prompt/pull/23))**: the terminal was measured with go-tty's `Size`, which falls back to asking the terminal for its size in pixels — it writes `\x1b[14t` and reads the reply back from the input handle — whenever the kernel reports no pixel dimensions, which is the normal case under tmux and most terminals. A terminal that does not answer costs the 100ms read timeout, and anything typed while that read was waiting was consumed with the reply and never reached the prompt. The size now comes from an ioctl on the terminal's own descriptor: nothing is written to the terminal and nothing is read from it. A render also measures once rather than per question, so the clear, the draw, and the cursor cannot disagree about the width halfway through.

## [0.0.13] - 2026-07-29

### Fixed
- **A wide or zero-width character misplaced the cursor and miscounted wrapped rows ([#19](https://github.com/nao1215/prompt/issues/19))**: Prefix and line widths were counted in runes. A rune is not a terminal cell: `"データ> "` is 5 runes and 8 columns, an emoji is 1 rune and 2 columns, and a combining mark is a rune that occupies none. A CJK or emoji prompt prefix therefore left the cursor several columns from the character it was meant to sit on, and a line that wrapped at the terminal edge was counted as fitting, so the redraw cleared too few rows and left stale text behind. Cursor positioning and the wrapped-row count now measure display cells, including the text before the cursor on its own line, which the multi-line branch had ignored entirely.

## [0.0.12] - 2026-07-29

### Added
- **Continuation prefix (`WithContinuationPrefix`, `SetContinuationPrefix`) (PR [#18](https://github.com/nao1215/prompt/pull/18), [e89e17b](https://github.com/nao1215/prompt/commit/e89e17b))**: A multiline prompt can now draw a marker in front of every line after the first. Without one, a buffer that `WithIsComplete` declined left the cursor on a bare line with nothing in front of it, which is indistinguishable from a hung program — the user could not tell that the prompt was waiting for the rest of a statement. This is the state sqlite3 shows as "   ...> ", psql as "db-# ", and mysql as "    -> ". The prefix is drawn in the prompt's own color, counted when positioning the cursor and when measuring how many terminal rows the input occupies, and never enters the returned input. The default is empty, which preserves the previous appearance.

## [0.0.11] - 2026-07-07

### Fixed
- **Interactive sessions broken on macOS by the v0.0.10 raw-mode change ([#13](https://github.com/nao1215/prompt/issues/13))**: v0.0.10 routed raw mode through go-tty's `Raw` on every platform, which sets raw mode on a `/dev/tty` descriptor go-tty opens itself. On macOS that regressed interactive sessions: the terminal ended up in a state where the first read returned immediately and the prompt exited without accepting input. Raw mode is now split by platform — Unix keeps the proven `golang.org/x/term.MakeRaw` on `os.Stdin` (the pty slave, the same terminal go-tty reads through), and only Windows routes raw mode through go-tty, where its read handle (CONIN$) genuinely differs from `os.Stdin`. This keeps the Windows ConPTY fix while restoring the working Unix path.

## [0.0.10] - 2026-07-07

### Fixed
- **Raw mode applied to the wrong handle on Windows ([#13](https://github.com/nao1215/prompt/issues/13))**: The terminal entered raw mode with `golang.org/x/term.MakeRaw` on `os.Stdin`, but read input through go-tty's own handle (its `/dev/tty` on Unix, `CONIN$` on Windows). On a Windows ConPTY those are different handles, so raw mode never governed the handle reads actually came from, and input delivered right after a prompt was re-rendered could be mishandled instead of buffered — a command could be typed but never executed. Raw mode now goes through go-tty's `Raw`, so it and the read path share one handle. `SetRaw`/`Restore` are also idempotent, so a persistent session enters raw mode once and cleanup paths stay balanced.

## [0.0.9] - 2026-07-07

### Added
- **Persistent raw mode (`WithPersistentRawMode`) (PR [#11](https://github.com/nao1215/prompt/pull/11), [3f8681a](https://github.com/nao1215/prompt/commit/3f8681a))**: An embedding REPL can keep the terminal in raw mode across consecutive `Run` calls instead of re-acquiring it on every call. Raw mode is entered once on the first `Run` and restored once — by `Close`, on interrupt (Ctrl+C), or on EOF. Off by default, so the classic single-shot usage keeps cooked-mode output after each `Run`.

### Fixed
- **Interactive input lost between lines ([#10](https://github.com/nao1215/prompt/issues/10), PR [#11](https://github.com/nao1215/prompt/pull/11), [3f8681a](https://github.com/nao1215/prompt/commit/3f8681a))**: A REPL that calls `Run` once per line toggled the terminal between raw and cooked around every command. Input a fast or automated driver (a pipe or pseudo-terminal) sent right after a re-rendered prompt could be dropped in the mode-switch window, making scripted sessions hang intermittently. `WithPersistentRawMode` removes that window; `enterRawMode`/`exitRawMode` are now idempotent so raw mode is acquired and released exactly once per session.

## [0.0.8] - 2026-06-28

### Added
- **Clear-screen action (`ActionClearScreen`)**: A new key action clears the terminal screen and scrollback and redraws the prompt at the top with the current input preserved. The default key map binds it to Ctrl+L, matching the typical shell shortcut.

## [0.0.7] - 2026-06-28

### Added
- **Escaped word boundaries for completion (`WithWordEscape`)**: An embedding app can opt into treating backslash-escaped whitespace as part of the word before the cursor, so a shell-style path like `my\ data.csv` completes and is accepted as one word instead of breaking at the escaped space. The new `Document.GetWordBeforeCursorEscaped` exposes the same boundary rule. Off by default, so existing word boundaries are unchanged.

## [0.0.6] - 2026-06-28

### Added
- **Multiline submit predicate (`WithIsComplete`)**: In multiline mode, an embedding app can supply a predicate that decides whether Enter submits the buffer or inserts a newline to keep editing. When it returns false, the input is treated as incomplete, so apps can buffer multi-line input (for example SQL until a trailing `;`). Backslash continuation and bracketed paste are unaffected; with no predicate or with multiline off, Enter always submits.

### Fixed
- **Bracketed paste multiline handling ([04b4805](https://github.com/nao1215/prompt/commit/04b4805))**: Preserve pasted newlines and trailing backslashes in multiline prompts without changing manual backslash continuation behavior

## [0.0.4] - 2025-01-22

### Fixed
- **Multi-line cursor positioning ([307ee32](https://github.com/nao1215/prompt/commit/307ee32))**: Completely fixed cursor and input character positioning issues in multi-line mode
  - Fixed cursor positioning calculation for continuation lines to start from line beginning (column 0)
  - Eliminated progressive character drift that caused input characters to move rightward over time
  - Simplified position calculations by removing complex prefix-based indentation logic
  - Added explicit carriage return (`\r`) and line clear (`\x1b[K`) for continuation lines to ensure proper line start positioning
  - Resolved visual misalignment between cursor position and actual character input location

### Enhanced
- **Multi-line input reliability**: Continuation lines now consistently start from line beginning without complex position calculations
- **User experience**: Eliminated confusing cursor/input position discrepancies that made multi-line editing difficult
- **Code maintainability**: Simplified renderer logic by removing error-prone position calculations for continuation lines

### Technical Improvements
- **Renderer simplification**: Updated `positionCursor` function to use simple line-start positioning for continuation lines
- **Consistent behavior**: Both cursor positioning and character rendering now follow the same simple rules
- **Cross-platform reliability**: Removed Unicode and terminal-specific positioning edge cases
- **Performance**: Eliminated complex calculations that could cause cumulative positioning errors

## [0.0.3] - 2025-01-21

### Fixed
- **Multi-line history navigation ([b160784](https://github.com/nao1215/prompt/commit/b160784))**: Fixed display position issues when navigating through multi-line command history
  - Improved `clearPreviousLines` function to properly clear multi-line content
  - Enhanced line count tracking for accurate terminal positioning
  - Fixed cursor position management for multi-line input navigation

- **Terminal line wrapping calculation ([b160784](https://github.com/nao1215/prompt/commit/b160784))**: Improved handling of long input lines that wrap across multiple terminal lines
  - Added `calculateRenderedLines` function to accurately count rendered lines
  - Accounts for terminal width when calculating line wrapping
  - Fixed prompt duplication issues when text wraps to next line
  - Properly handles prefix length in line wrapping calculations

### Technical Improvements
- **Renderer enhancements**: Added terminal interface to renderer for dynamic size detection
- **Terminal width awareness**: Renderer now considers terminal width for accurate line wrapping
- **Line counting accuracy**: More precise calculation of actual rendered lines vs logical lines
- **State management**: Improved tracking of rendered line count for better screen clearing

## [0.0.2] - 2025-01-20

### Fixed
- **Completion suggestion scrolling ([994b558](https://github.com/nao1215/prompt/commit/994b558))**: Fixed infinite scrolling bug when navigating through completion suggestions beyond the visible range
  - Implemented proper scroll boundaries to prevent selection from continuing into empty fields
  - Added offset-based rendering system for smooth scrolling through large suggestion lists
  - Maximum 10 suggestions displayed at once with proper up/down navigation
- **Terminal boundary display issues**: Fixed completion suggestions jumping to screen top when displayed at terminal bottom
  - Improved ANSI escape sequence handling for terminal edge cases
  - Enhanced cursor positioning to avoid terminal boundary artifacts
- **Cursor flickering during completion ([994b558](https://github.com/nao1215/prompt/commit/994b558))**: Eliminated excessive cursor movement during suggestion navigation
  - Implemented cursor hiding during suggestion display with `\x1b[?25l`/`\x1b[?25h`
  - Optimized rendering to minimize cursor position updates
  - Added state management to track suggestion display status
- **Suggestion list persistence**: Fixed completion suggestions not clearing after TAB selection
  - Implemented comprehensive screen clearing with `\x1b[0J` escape sequence
  - Added proper state transition handling between suggestion display and normal input
  - Enhanced cleanup of suggestion rendering areas

### Enhanced
- **Scroll test example**: Updated autocomplete example with 23+ commands and 15+ items to demonstrate scrolling functionality
  - Added comprehensive test scenarios for suggestion scrolling
  - Included detailed README with testing instructions
  - Improved user experience validation tools

### Technical Improvements
- **Renderer architecture**: Enhanced separation between cursor management and suggestion rendering
- **State management**: Improved tracking of suggestion display state with `suggestionsActive` flag
- **Screen clearing**: More robust terminal content clearing with multiple fallback strategies
- **Cross-platform compatibility**: Better handling of terminal differences across operating systems

## [0.1.0] - 2025-09-18

### Added
- **Initial implementation of modern prompt library ([45519e9](https://github.com/nao1215/prompt/commit/45519e9))**: Complete rewrite of go-prompt with improved architecture and cross-platform support
- **Functional options API pattern**: Clean, extensible configuration using `WithCompleter`, `WithMemoryHistory`, etc.
- **Cross-platform terminal support**: Enhanced Windows compatibility via mattn/go-colorable, native Unix support
- **Resource management**: Proper cleanup with Close() method and defer patterns to prevent file descriptor leaks
- **Error recovery mechanisms**: Fixed critical divide-by-zero panics and improved error handling
- **Comprehensive testing framework**: Test-driven development with >80% coverage target
- **Multi-language documentation**: Support for Chinese (zh-cn) documentation
- **Sponsor integration**: GitHub Sponsors support for project sustainability

### Fixed
- **Divide by zero panics** in terminal rendering logic that plagued original go-prompt
- **File descriptor leaks** in /dev/tty handling through proper resource management
- **Windows terminal compatibility issues** with ANSI escape sequences
- **Terminal color reset issues** on application exit
- **Memory leaks and race conditions** through improved concurrency design

### Changed
- **Simplified API design**: Reduced from 50+ public APIs to 5 core types for better usability
- **Modernized architecture**: Clear separation of concerns between input, output, rendering, and completion
- **Interface-based design**: Enhanced testability and extensibility through proper abstractions
- **Performance optimizations**: Efficient diff-based terminal updates and minimal memory allocations

### Technical Details
- **Thread Safety**: Library is explicitly NOT thread-safe by design for performance
- **Platform Support**: Linux, macOS, Windows with native terminal capabilities
- **Unicode Support**: Full UTF-8 character handling including wide characters
- **Development Tools**: Makefile with test, lint, clean, and tools targets

---

## Project Context

This project is a modern replacement for the unmaintained go-prompt library (github.com/c-bata/go-prompt), addressing 286 open issues and critical bugs that have existed since March 2021.

### Migration from go-prompt
- **Drop-in replacement**: Designed for easy migration from original go-prompt
- **API compatibility**: Maintains familiar patterns while improving reliability
- **Performance improvements**: Better memory usage and rendering efficiency
- **Enhanced cross-platform support**: Robust Windows, macOS, and Linux compatibility

### Sponsors
Support this project: https://github.com/sponsors/nao1215
