# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- `NewFuzzyCompleter` completes a candidate holding a space, one reached by an input in another case, and one matched as a subsequence (PR [#110](https://github.com/nao1215/prompt/pull/110), [fb78f15](https://github.com/nao1215/prompt/commit/fb78f15), [#108](https://github.com/nao1215/prompt/issues/108)). It matches the input before the cursor, ignoring case, and accepts a subsequence; the read loop then kept only the candidates starting with the word before the cursor, case-sensitively, which does none of those three things. Pressing Tab after typing `git st` left the prompt asking whether `git status` starts with `st`, so every candidate was discarded, no menu opened and the key changed nothing -- the candidate list in the function's own documentation could not be completed past its first word. The suggestions now name the span they stand for, which is the mechanism the prompt already has for a completer that owns its matching, so the filter is skipped and the candidate replaces the input before the cursor. The filter itself is unchanged: it is what makes a plain list of candidates work, and the auto-accept when exactly one candidate matches depends on it.
- Reverse search and `NewFuzzyCompleter` no longer list entries that do not match (PR [#110](https://github.com/nao1215/prompt/pull/110), [fb78f15](https://github.com/nao1215/prompt/commit/fb78f15), [#109](https://github.com/nao1215/prompt/issues/109)). The subsequence pass added ten per character it found and returned whatever it had when the candidate ran out, which is how far the query got rather than whether it arrived, and every caller reads a score above zero as a match: Ctrl+R for `sql` offered an entry holding no `q`, and any entry sharing the query's first character was listed, so the list barely narrowed as the user typed. A match is now the whole query or nothing. The exact, prefix and substring tiers keep the scores they had, so nothing reorders among true matches.
- Reverse search fits the terminal it is drawn on (PR [#110](https://github.com/nao1215/prompt/pull/110), [0513a77](https://github.com/nao1215/prompt/commit/0513a77), [#107](https://github.com/nao1215/prompt/issues/107)). Ctrl+R drew a header and up to five matches without asking the terminal how many rows it had. The block is redrawn on every keystroke and erased by a cursor move back up its own height from the row below it, so it has to leave that row on screen: the room for it is a row less than the terminal's height. A block that took more never got its first row back. Every redraw started a row lower than the last and pushed that many rows of the session off the top -- three rows per keystroke for a twelve-row block on an 80x10 terminal -- and the header, which names the entry Enter would take, was the first thing gone, so the user was steering a search they could not see. Five statements of the length a pasted query reaches are twelve rows on that terminal; nothing bounds an entry, so a terminal of the usual size reaches it too, and any history at all reaches it in a split pane. The block is now measured against the height as well as the width. The header is always drawn, however little room there is, because a search that shows nothing is a search the user cannot steer, and it is cut to the rows available; what the height costs is matches, down to none.
- A completion menu stays inside the terminal whichever candidate is selected (PR [#110](https://github.com/nao1215/prompt/pull/110), [2193e57](https://github.com/nao1215/prompt/commit/2193e57), [#106](https://github.com/nao1215/prompt/issues/106)). The selected candidate was marked with a black right-pointing triangle while every entry was measured with the two spaces drawn in front of an unselected one. Unicode calls that triangle East Asian Ambiguous, so go-runewidth reports it as two cells under a CJK locale and one elsewhere: the selected indicator took three cells where it had been counted as two, so a candidate that filled its row wrapped when the selection reached it, and the menu grew a row past what the window had room for. The terminal scrolled, and the line being completed is the first thing to go -- which is what [#104](https://github.com/nao1215/prompt/issues/104) had just fixed for every other row. No measurement could have been right, because a terminal makes its own choice about an ambiguous glyph as well, and one that draws the triangle in a single cell puts the arithmetic out in the other direction. Both indicators are now ASCII and both are two cells, so the counted row and the drawn row are the same row by construction; the selected row is still told apart by the scheme's `Selected` color.

- A completion menu no longer scrolls the line being completed off the screen (PR [#105](https://github.com/nao1215/prompt/pull/105), [60de54e](https://github.com/nao1215/prompt/commit/60de54e), [#104](https://github.com/nao1215/prompt/issues/104)). The menu's size was a count of entries, ten, and the renderer never read the terminal's height, so the menu's height in rows was whatever those ten happened to occupy: a candidate wider than the terminal was drawn, and counted, as the rows it wrapped onto. Ten candidates of about 215 cells are 30 rows on an ordinary 80x24 terminal, and ten short ones do not fit in a ten-row split pane. The terminal scrolled to make room and the first rows to go were the prompt and the line being completed, so Tab left the user reading candidates with no sight of what they were completing, and scrolled the previous command's output away as well. The menu now lists at most ten candidates and only as many as fit in the rows left under the input block, and none at all when nothing fits. The scroll position is worked out from the same measurement the menu is drawn with, because a window smaller than the read loop assumed would leave the highlight off screen while Enter still accepted the candidate it was on.
- `WatchInterrupt` sees Ctrl+C in a session that did not ask for persistent raw mode, instead of the key killing the application (PR [#103](https://github.com/nao1215/prompt/pull/103), [6c1c7aa](https://github.com/nao1215/prompt/commit/6c1c7aa), [#101](https://github.com/nao1215/prompt/issues/101)). It looked only for the byte 0x03, which a terminal delivers only in raw mode, and outside a persistent session raw mode is given back when `Run` returns -- so the gap the watch covers is a cooked terminal, where the driver turns the key into SIGINT for the foreground process group. The context was never canceled, and the signal's default action ended the process in the middle of the work the watch exists to cancel, with nothing deferred run and the session's history unsaved. The README's own example is this arrangement: a prompt built without that option, doing work between prompts. The watch now registers for the signal as well as reading the byte, so either one cancels the context; registering also takes the signal's default action away for as long as the watch is active. A persistent session is unchanged, because with `ISIG` off no signal is generated and the byte path cancels as before. An interrupt sent any other way, such as `kill -INT`, cancels the work too: nothing tells it apart from the key.
- A `Run` on a closed prompt returns `ErrEOF` at once, instead of entering raw mode on a terminal the session has already given up and leaving it that way (PR [#103](https://github.com/nao1215/prompt/pull/103), [6c1c7aa](https://github.com/nao1215/prompt/commit/6c1c7aa), [#102](https://github.com/nao1215/prompt/issues/102)). It entered raw mode before discovering the terminal was gone, and because raw mode is set on a descriptor `Close` never touches, the attempt succeeded: in a session using `WithPersistentRawMode` the per-call restore is skipped by design and the `Close` that would have restored the terminal had already run, so the application exited leaving the user's shell with no echo, no line editing, a dead Ctrl+C, and bracketed paste still on. `Close` now records that the session is over, `Run` answers before touching anything, and a `Run` ended by a `Close` from another goroutine reports `ErrEOF` too rather than an unexported terminal error a caller could not match -- so a REPL loop written the way the README shows it leaves its loop instead of drawing the prompt again on a terminal that has been given up.
- Moving the cursor with the completion menu open no longer mangles the line (PR [#100](https://github.com/nao1215/prompt/pull/100), [53e147a](https://github.com/nao1215/prompt/commit/53e147a), [#98](https://github.com/nao1215/prompt/issues/98)). A suggestion stands for the word before the cursor at the moment Tab was pressed, and applying it works that word out again from wherever the cursor is now. Moving left made the two disagree: `cre`, Tab, Left, Tab inserted part of the suggestion into the middle of the word already there and gave `createe`, and `cre`, Tab, Home, Tab gave `createcre`. Left, Home, End, and the two word moves now end the completion, so the next Tab asks the completer again for the word the cursor is on. Right still accepts the highlighted suggestion, and Up and Down still walk the menu.
- Ctrl+U, Ctrl+K and Ctrl+R close the completion menu, as every other key that changes the line already did (PR [#100](https://github.com/nao1215/prompt/pull/100), [53e147a](https://github.com/nao1215/prompt/commit/53e147a), [#99](https://github.com/nao1215/prompt/issues/99)). They replaced what was on the line and left the menu drawn over it, so the prompt listed completions under an empty prefix and the next accept put the discarded text back -- which is the opposite of what Ctrl+U is for.

## [0.0.27] - 2026-09-01

### Removed

- `ColorScheme.Background`, `ColorScheme.Cursor`, `SuggestionColors.Match`, and `SuggestionColors.Background` are gone ([#93](https://github.com/nao1215/prompt/issues/93)). Nothing ever read them: the prompt writes a foreground color and nothing else, so setting one changed nothing on screen. The one application using this library set `Cursor` and `Suggestion.Match` on every one of its twelve themes, so those colors were chosen twelve times and never drawn. A field that changes nothing is worse than one that is missing.

  Coloring the part of a suggestion that matched would need the completer to say which part matched, which is what `Suggestion.Replace` is for and is not something the renderer is told today; a background would mean painting cells no text covers, which is a different renderer from this one; and a cursor color would mean OSC 12 and restoring the terminal's own afterwards. None of the three can be turned on from where the field sat, which is why they are removed rather than implemented.

  Migration: delete those fields from any `ColorScheme` literal. Nothing else changes -- the colors they held were never on screen to lose.

## [0.0.26] - 2026-09-01

### Fixed

- A history configured with `Enabled: false` stays empty (PR [#91](https://github.com/nao1215/prompt/pull/91), [28e8de1](https://github.com/nao1215/prompt/commit/28e8de1), [#90](https://github.com/nao1215/prompt/issues/90)). `SetHistory` fell into the branch meant for a prompt with no history manager at all, so the entries landed in the list the arrow keys walk while `GetHistory` reported none: pressing Up brought back an entry the getter said was not there. A disabled manager is now given nothing, the way `AddHistory` already treated it.

## [0.0.25] - 2026-09-01

### Fixed

- The cursor is visible again when the prompt gives the terminal back (PR [#85](https://github.com/nao1215/prompt/pull/85), [d668f57](https://github.com/nao1215/prompt/commit/d668f57), [#84](https://github.com/nao1215/prompt/issues/84)). The completion menu hides the cursor while it is drawn and shows it again on the next render without one, and an interrupt returns before that render: Ctrl+C with the menu open handed the terminal back with no cursor, and an application that exits on Ctrl+C left the shell it returned to without one for the rest of that terminal's life.
- A history entry longer than 64KB no longer makes the history file unreadable (PR [#82](https://github.com/nao1215/prompt/pull/82), [c533f95](https://github.com/nao1215/prompt/commit/c533f95), [#80](https://github.com/nao1215/prompt/issues/80)). The read used a `bufio.Scanner`, which refuses a line over that, while nothing bounds an entry -- a paste is content, and what the user submits is whatever they typed -- so the writer could produce a file the reader rejected. The load failed, took the whole history with it, and `New` returns that error: one long pasted statement left the application unable to start until the file was deleted by hand. Reading with a `bufio.Reader` has no line-length limit, and a last line that ends without a newline is kept.
- `New` closes the terminal when it cannot finish (PR [#82](https://github.com/nao1215/prompt/pull/82), [c533f95](https://github.com/nao1215/prompt/commit/c533f95), [#81](https://github.com/nao1215/prompt/issues/81)). It opens the terminal before loading the history, and a load that failed returned the error with nothing left to close it, so the go-tty handle, the descriptor the reader polls, and the pipe that wakes it leaked -- three per attempt for a caller that retries.

## [0.0.24] - 2026-09-01

### Fixed

- The history file is created readable by its owner alone (PR [#76](https://github.com/nao1215/prompt/pull/76), [88ba9b4](https://github.com/nao1215/prompt/commit/88ba9b4), [#74](https://github.com/nao1215/prompt/issues/74)). It was created with `os.Create`, which asks for 0666 and lets the umask decide, so on a common default it was readable by everyone -- while holding what the user typed, which is where a password given on a command line ends up. A file that already exists keeps the mode it was given.
- `MaxFileSize` bounds what is written, and rotation happens once rather than on every save (PR [#76](https://github.com/nao1215/prompt/pull/76), [88ba9b4](https://github.com/nao1215/prompt/commit/88ba9b4), [#75](https://github.com/nao1215/prompt/issues/75)). Rotation wrote a file the size of the one it had just moved aside, so the new file was over the limit the moment it appeared and the next save rotated again: within `MaxBackups` saves every backup held a near-identical copy of the newest history and the oldest entries had been deleted. Above 200 entries it halved the history instead, dropping half of what the running session could recall. The newest entries whose encoded lines fit are now written and the rest stay in the backup, the history held in memory is left alone, and rotation happens the first time the history outgrows the file.
- Fuzzy matching finds characters outside ASCII (PR [#73](https://github.com/nao1215/prompt/pull/73), [4ff7963](https://github.com/nao1215/prompt/commit/4ff7963), [#72](https://github.com/nao1215/prompt/issues/72)). `calculateFuzzyScore` ranged over the input, which yields characters, and indexed the candidate, which yields bytes, then compared the two: one byte of a UTF-8 sequence can never equal the character it belongs to, so every candidate written in another alphabet scored zero on the character-by-character pass. Ctrl+R could not find a command written in Japanese by its characters and `NewFuzzyCompleter` could not offer one, while prefix and substring matches, which are tested first, kept working. An ASCII query keeps the score it had, because for ASCII the two walks visit the same positions.
- Ctrl+D on an empty line reports `ErrEOF`, the error the documentation names (PR [#71](https://github.com/nao1215/prompt/pull/71), [0b69e1f](https://github.com/nao1215/prompt/commit/0b69e1f), [#70](https://github.com/nao1215/prompt/issues/70)). It returned `io.EOF` while every other end of input returned `ErrEOF`, so a REPL written the way the README shows it never ended: the loop breaks on `errors.Is(err, prompt.ErrEOF)`, which was false, so it took the error branch and drew the prompt again on every Ctrl+D. `ErrEOF` now wraps `io.EOF`, so a caller matching either one is right, and its message and identity are unchanged.
- Pressing Down after Up brings back the line that was being typed, instead of emptying the prompt (PR [#69](https://github.com/nao1215/prompt/pull/69), [46dfd9a](https://github.com/nao1215/prompt/commit/46dfd9a), [#68](https://github.com/nao1215/prompt/issues/68)). The index past the newest history entry is where the line being edited belongs, and nothing had saved that line, so looking up an earlier command cost whatever was half typed. It is now saved when Up moves off that index -- leaving the line, rather than moving between entries -- and restored when Down returns there. Typing already puts the prompt back on a fresh line, so an edit made to a history entry stays where it was made.
- `RunWithContext` returns when its context is canceled, rather than on the keystroke after it (PR [#67](https://github.com/nao1215/prompt/pull/67), [33c010c](https://github.com/nao1215/prompt/commit/33c010c), [#66](https://github.com/nao1215/prompt/issues/66)). The context was checked at the top of the read loop and then the prompt blocked in a terminal read, which nothing but a key ends, so a deadline fired one keystroke late and a prompt nobody was typing at never returned at all -- which is the case the documented 30-second timeout example is written for. Where the context can be canceled the read now goes through the shared reader goroutine, whose channel is waited on alongside the context; a rune that goroutine had already taken reaches the next `Run`, as a watcher's type-ahead does. A context that can never be canceled, which is what `Run` passes, still reads the terminal directly and starts no goroutine, because on Windows `Close` does not wait for a reader and a session that never asked for cancellation should not grow one.

## [0.0.23] - 2026-09-01

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
