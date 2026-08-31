# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- A tab that reaches the right margin no longer pushes the text after it onto a row the terminal never used (PR [#59](https://github.com/nao1215/prompt/pull/59), [7766180](https://github.com/nao1215/prompt/commit/7766180), [#54](https://github.com/nao1215/prompt/issues/54)). A terminal moves a tab to the next tab stop and, when the row holds no further stop, to the last column -- where the cursor sits without a wrap owed, so the next character prints beside it. The measurement stopped the tab one column further, at the width, which is how a filled row is spelled, so a wrap was claimed that never happened. Everything the wrap arithmetic answers inherited it: the cursor was drawn a row below the text and up to eight columns left of it, and the next redraw moved up past the top of its own block and erased the line the application had printed above the prompt, taking a line of scrollback with it on every keystroke. On an 80-column terminal the tab has to land on column 73 or later, which one pasted TSV row, or one indented statement, is enough to reach.
- A CSI sequence longer than `maxCSILength` no longer has its tail inserted into the line as typed text (PR [#59](https://github.com/nao1215/prompt/pull/59), [7766180](https://github.com/nao1215/prompt/commit/7766180), [#55](https://github.com/nao1215/prompt/issues/55)). The read was bounded by a rune count, and returning at the bound only stopped reading: whatever the terminal was still sending stayed in the input and reached the read loop as keystrokes, so a long terminal reply appeared in the user's line one parameter at a time. The loop now follows the CSI grammar. It reads to the final byte whatever the length and remembers at most `maxCSILength` of it, so a sequence too long to name is still consumed whole; and a byte outside the grammar aborts the sequence and is pushed back as input, so `ESC [` followed by Enter no longer swallows the Enter.
- Ctrl+W, Ctrl+Left, and Ctrl+Right no longer run past the word they are in when the line is not written in ASCII (PR [#59](https://github.com/nao1215/prompt/pull/59), [7766180](https://github.com/nao1215/prompt/commit/7766180), [#56](https://github.com/nao1215/prompt/issues/56)). A word character was a letter in `a-z` or `A-Z`, a digit, or an underscore, which made every other alphabet a separator: word navigation walked over a word written in Japanese as if it were whitespace and carried on into the ASCII word before it, so `select 名前` lost `select` too, and a letter with a diacritic split its own word in two, so Ctrl+W on `naïve ` left `naï`. A letter is now a letter in any script. The ASCII behavior is unchanged. Completion decides what a word is by a separate rule, which this does not touch.
- The history keeps at most `MaxEntries` entries, and reloading a history file replaces what is held rather than adding to it (PR [#59](https://github.com/nao1215/prompt/pull/59), [7766180](https://github.com/nao1215/prompt/commit/7766180), [#57](https://github.com/nao1215/prompt/issues/57), [#58](https://github.com/nao1215/prompt/issues/58)). The limit was applied a layer above the history, by reading it back, cutting it, and writing the shortened copy down again, so the history itself grew for as long as the process ran; it is now applied where the entries are held. A load answers what is in the file, and appending made asking twice -- after another shell wrote to it, after the user edited it -- return every entry twice.

- A completion menu row and a reverse-search result are drawn as the one terminal row they are counted as (PR [#59](https://github.com/nao1215/prompt/pull/59), [c90b66b](https://github.com/nao1215/prompt/commit/c90b66b), [#60](https://github.com/nao1215/prompt/issues/60), [#61](https://github.com/nao1215/prompt/issues/61)). Both heights are measured by walking the text for cells, and a newline occupies no cells while moving the terminal to the next row, so an entry carrying one was drawn taller than it was counted and the extra row survived the erase. A statement entered across several lines is stored as one history entry -- that is what the history file's escaping is for -- so reverse search over a multiline statement drew four rows where it had counted two, and left two on screen each time the search closed. The other C0 controls were worse than uncounted: the history file preserves whatever was typed, so an entry can hold an escape sequence, and drawing it handed the terminal a command. Both lists now flatten what they draw, replacing every rune that would take the cursor off the row with a space, and measure the flattened text. A tab is kept, because a terminal keeps it on the row and the layout walk measures it against tab stops.

- Backslash continuation no longer panics on a line holding a character outside ASCII (PR [#65](https://github.com/nao1215/prompt/pull/65), [32cc371](https://github.com/nao1215/prompt/commit/32cc371), [#63](https://github.com/nao1215/prompt/issues/63)). The buffer is a `[]rune` and the line's start indexes it, but the trailing backslash was located by adding the byte length of the line's text, so every multi-byte rune on the line moved the position three cells further past the end. Pressing Enter on `select 名前 \` ended in a slice bounds error that took the application down and left the terminal in raw mode; where the buffer's capacity happened to reach the computed index the slice was legal instead, and the prompt deleted a rune that was not the backslash. The check runs on every Enter, so neither `WithMultiline` nor any other option was needed to reach it.
- Reverse search reads the whole escape sequence of a key that is spelled as one (PR [#65](https://github.com/nao1215/prompt/pull/65), [491cfca](https://github.com/nao1215/prompt/commit/491cfca), [#64](https://github.com/nao1215/prompt/issues/64)). It read raw runes and treated `ESC` as the cancel key, so an arrow key ended the search and left the rest of its sequence in the input, where the read loop took it as typing: pressing Up put `[A` in front of the user. A bare Escape still cancels. Up and Down now move through the matches, which Tab already does in one direction, and every other sequence is consumed and ignored.

### Removed

- `HistoryManager`, `NewHistoryManager`, `DefaultHistoryConfig`, `NewHistorySearcher`, `KeyBinding`, and `Reset` are no longer exported (PR [#59](https://github.com/nao1215/prompt/pull/59), [a10a22f](https://github.com/nao1215/prompt/commit/a10a22f)). The exported surface goes from 39 top-level names to 33, and seven exported methods go with it. `HistoryManager` had no caller outside the package, and both history bugs above lived in exactly the path an outside caller would have taken, so the history now has one owner and one contract. `KeyBinding` was a struct nothing read. `Reset` was a package-level function named as though it reset the prompt, used only to close a color run while drawing. Migration: reach history through `WithHistory`, `WithMemoryHistory`, `WithFileHistory`, and the prompt's own `GetHistory`, `SetHistory`, `AddHistory`, and `ClearHistory`; build a `HistoryConfig` literal in place of `DefaultHistoryConfig()`; reach reverse search through Ctrl+R.

## [0.0.22] - 2026-08-31

### Added

- `WithAutoIndent` sets what a new line opens with while a multiline entry is being typed. It is called with the input up to where the line breaks, and what it returns is inserted at the start of the new line. Every way a line can break goes through it -- an input the application reports incomplete, a newline key, a trailing backslash -- so a continuation looks the same however it was asked for. Without the option nothing is inserted, which is what a prompt did before it: every continuation line started at the margin, however deep in a bracket the writer was.
- `WithHighlighter` colors the input as it is drawn. It is given the whole input and returns the runs to draw in a color of their own, as rune offsets into that input; everything no run covers keeps the scheme's color. The prompt has no opinion about what the runs mean, so a highlighter for SQL colors keywords and string literals and one for a shell colors the command and its flags. It decides colors and nothing else: the input is drawn exactly as it is, and the prompt measures its layout from that text rather than from what is written to the terminal, so a highlighter cannot move the cursor away from the character under it or wrap a line early. A run reaching outside the input, an inverted one, and two that overlap are drawn over rather than rejected, because getting a color wrong must not cost the user the line they are typing.

## [0.0.21] - 2026-08-31

### Fixed

- `Close` now ends the goroutine that reads the terminal on Unix, so a prompt opened after one was closed reads its input ([#47](https://github.com/nao1215/prompt/issues/47)). `WatchInterrupt` starts a shared reader whose goroutine blocks in a terminal read, and closing the terminal did not end it: go-tty calls `Fd()` on the file it reads, which takes it out of the runtime's poller and back into blocking mode, and a blocking read is what nothing can interrupt. The goroutine was left in `read(2)` on a descriptor number the kernel had taken back, and running a child process is enough to get that number reused — by the next prompt's own `/dev/tty`. The abandoned reader then held the new prompt's terminal, which drew its prefix and swallowed every keystroke, `Ctrl-D` included, so the session could not be ended. On Unix the prompt now reads the terminal itself, through a non-blocking descriptor and a `poll(2)` that also watches a pipe `Close` writes to, so a read waiting for a key ends the moment the terminal is given up; `Close` then waits for the reader to finish. Handing the descriptor to the runtime's poller instead would have been smaller but is not portable: neither `os.OpenFile` with `O_NONBLOCK` nor `os.NewFile` over an already non-blocking descriptor polls a terminal on the BSDs, where both return `EAGAIN` from every read rather than waiting for a key. Windows still reads through go-tty, where raw mode is routed through the same handle, and `Close` does not wait there.

## [0.0.20] - 2026-08-31

### Added

- `Suggestion.Replace` names the span of the input a suggestion overwrites (PR [#45](https://github.com/nao1215/prompt/pull/45), [c04734e](https://github.com/nao1215/prompt/commit/c04734e)), so a completer can match by its own rule. The prompt otherwise decides what a suggestion stands for by taking the word before the cursor and keeping only the suggestions that word is a case-sensitive prefix of. A completer matching case-insensitively had its whole answer discarded — typing `sel` and pressing Tab did nothing, because `SELECT` is not prefixed by `sel` — and one completing a qualified name such as `t.na` could not say that only the part after the dot was being replaced. A suggestion carrying `Replace` is applied literally, and a set containing one skips the built-in filter; leaving it nil keeps the existing behavior. The span is counted in runes and clamped to the buffer.

### Fixed

- `Document.CursorPosition` is now read as the rune offset it is everywhere in `Document` (PR [#45](https://github.com/nao1215/prompt/pull/45), [c04734e](https://github.com/nao1215/prompt/commit/c04734e)): it indexes the prompt's `[]rune` buffer, but `TextBeforeCursor`, `TextAfterCursor`, and `GetWordBeforeCursor` sliced the text by it as a byte offset, so with any multi-byte rune on the line a completer was handed a prefix shorter than what had been typed, cut mid-character.

### Changed

- Dependencies updated (`golang.org/x/term`, `golang.org/x/sys`, `github.com/mattn/go-tty`, `github.com/mattn/go-colorable`, `github.com/mattn/go-runewidth`, `github.com/stretchr/testify`), holding `x/term` at 0.40.0 and `x/sys` at 0.41.0 so the supported Go floor stays at 1.24.0. The GitHub Actions pins move from v5 to v7, and the unit-test matrix runs the floor and the newest Go release.

## [0.0.19] - 2026-08-09

### Fixed
- A completion suggestion wider than the terminal left rows of the menu behind ([#36](https://github.com/nao1215/prompt/issues/36)): the height remembered for the menu was the number of suggestions shown, and a suggestion that wraps occupies more rows than that. The erase moved up too few rows, so closing or accepting the completion left the wrapped rows sitting above the new prompt. The menu now reports the rows it drew, measured the same way the input line is.
- A tab in pasted input left the cursor several columns left of the text and the redraw cleared too few rows ([#37](https://github.com/nao1215/prompt/issues/37)): the renderer measures with `runewidth`, which reports 0 for a tab because it has no notion of tab stops, while a terminal advances a tab up to eight columns, to the next stop. The cursor was drawn that far left of the text, and a line the tab pushed past the terminal width was undercounted. A tab reaches the buffer through a bracketed paste — pasted SQL, a TSV row — since a typed Tab runs completion instead. The wrap arithmetic now advances a tab to the next stop, clamped at the right margin, which is where a terminal leaves it.
- A history entry came back changed after being saved and reloaded (PR [#40](https://github.com/nao1215/prompt/pull/40), [#38](https://github.com/nao1215/prompt/issues/38), [#39](https://github.com/nao1215/prompt/issues/39)): entries were written raw and read back one physical line at a time, so an entry containing a newline came back as several. Every entry was also trimmed on load, so a command submitted with leading or trailing spaces was recalled without them. Entries are now escaped on write (`\`, `\n`, `\r`) and unescaped on read, which keeps a multi-line command one entry and leaves an entry's own whitespace alone. Only the line terminator (including the `\r` of a CRLF file) and empty lines are dropped. An escape the format does not define is read back as it was written, so a line written before this encoding is unchanged unless it contains `\`.

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
