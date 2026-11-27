# Testing

`make test` runs everything below except the long fuzz and property searches, which run on a schedule in CI.

## Examples and unit tests

`example_test.go` holds the examples of the public API; the ones that need a terminal are compiled and not run. The unit tests sit beside the source they test and drive the read loop through a mock terminal (`mock_test.go`) that holds the keys as bytes, so a test of an invalid UTF-8 byte reaches the prompt as one. `keymap_test.go` binds every action a key can be bound to and checks what the read loop does with it.

## Pseudo-terminal tests

`pty_reader_test.go` (not on Windows) runs a prompt in a child process attached to a pseudo-terminal from [creack/pty](https://github.com/creack/pty), and types into it: closing a prompt and opening another, a child that leaves the terminal in raw mode, and a resize while a read is waiting. These are the paths the mock terminal cannot reach, because they depend on what the operating system does with the terminal.

## Fault injection

`write_failure_test.go` runs scripted sessions (typing with a highlighter, the completion menu, reverse search, a long paste, a block taller than the terminal) and makes each of their writes fail in turn. Until Enter accepts the line, a failed write has to end the session with its error. After that, the writes left only tidy the screen, so the line is still returned. `fault_test.go` makes the terminal fail (a read, raw mode, restoring it, reporting its size, closing it) and the history file (a backup that cannot be removed or moved, a directory that cannot be created). `tty_reader_unix_test.go` reads through the interruptible reader over a pseudo-terminal, a pipe, and a directory, whose read fails.

## Property-based tests

The property tests draw their inputs with [rapid](https://pkg.go.dev/pgregory.net/rapid), which shrinks a failure to a small counterexample. They live in `property_test.go`, and their names start with `TestProperty_`, `TestMetamorphic_` or `TestDifferential_`. The drawn text is made of spaces, tabs, line breaks, backslashes and a few letters in and outside ASCII, because those are the characters the line-editing code treats differently.

- `TestProperty_DocumentSplitsAtTheCursor`: the text before and after the cursor put together is the text, and a cursor outside the text is answered with the whole text.
- `TestProperty_HistoryFollowsItsModel`: a random sequence of entries added to the history leaves it equal to a plain model after every step: no empty entry, no repeat of the last one, only the newest `MaxEntries`.
- `TestProperty_HistorySurvivesTheFile`: a history saved to a file and loaded into a second manager comes back entry for entry, line breaks and backslashes included.

## Metamorphic tests

- `TestMetamorphic_FuzzySearchIgnoresTheRest`: removing one candidate from the fuzzy completer's list removes that match and leaves the others in their order, and adding a candidate the query does not match changes nothing. This is what found that tied matches were reordered by unrelated candidates.
- `TestMetamorphic_FuzzySearchIgnoresCase`: the case of the query changes neither what matches nor the order.

## Differential tests

- `TestDifferential_WordBeforeCursorIsTheLastField`: the word before the cursor is the last field `strings.FieldsFunc` finds, or nothing after a separator.
- `TestDifferential_EscapedWordAgreesWithoutBackslashes`: on a line without a backslash, `WordBeforeCursorEscaped` gives the word `WordBeforeCursor` gives.
- `TestDifferential_CurrentLineIsTheCursorsLine`: the current line is the line `strings.Split` gives at the index of the line breaks before the cursor.
- `TestDifferential_FuzzyScoreMatchesASubsequence`: the fuzzy score is above zero exactly when the regular expression `a.*b.*c` built from the input matches the candidate.

## Running the generated tests longer

`go test` draws a hundred inputs per test. Draw more, or replay a failure, with rapid's flags:

```sh
go test . -run '^Test(Property|Metamorphic|Differential)_' -rapid.checks=20000
go test . -run '^TestMetamorphic_FuzzySearchIgnoresTheRest$' -rapid.seed=<seed printed by the failure>
```

The `Property` workflow (`.github/workflows/property.yml`) draws 20,000 inputs per test every night with a new seed. A failure writes the shrunk input under `testdata/rapid/`, which is ignored by git and uploaded by the workflow.

## Fuzzing

The fuzz targets are in `renderer_test.go` (the layout stays on the screen and matches what is drawn), `viewport_test.go`, `highlight_test.go` (highlighting never moves a character) and `history_test.go` (a history line reads back as it was written, and the reverse-search block fits what it draws). `go test` runs their seed corpus and `testdata/fuzz/`; the `Fuzz` workflow finds every target and searches each one every night.

```sh
go test . -run '^$' -fuzz '^FuzzHistoryLineRoundTrip$' -fuzztime 1m
```

## Benchmarks

`bench_test.go` measures the work done on every key press (layout, highlighting, the completion menu, fuzzy completion) and the read of the history file at start-up. The `Benchmark` workflow (`.github/workflows/benchmark.yml`) builds the base and the head of a pull request on one runner and runs them in turn for 8 rounds with `GOMAXPROCS=1`. It fails when a benchmark's median is more than 1.25 times the base's and even its fastest sample is slower than the base's median (`scripts/ci_benchmark.py`).

```sh
python3 scripts/ci_benchmark.py --base origin/main --head HEAD
```

## What a pull request has to pass

- Coverage: octocov fails a pull request that lowers coverage or leaves it under 99%. `example/` and `terminal.go`, which needs a real terminal, are left out.
- Unit tests on Linux, macOS, and Windows with the go.mod floor and the newest Go, as `GOARCH=386`, and under the race detector on Linux.
- Each fuzz target for 15 seconds.
- Mutation testing of the changed lines, which `.gremlins.yaml` fails at or below 90% efficacy.
- The benchmark comparison above.

## Mutation testing

[gremlins](https://github.com/go-gremlins/gremlins) v0.6.0 changes one comparison, bound or arithmetic operator at a time and runs the package's tests; a mutant no test notices is a line the tests execute without checking. The `Mutation` workflow (`.github/workflows/mutation.yml`) mutates the lines a pull request changes, and the whole package once a week. The programs under `example/` are left out.

```sh
gremlins unleash --workers 4 --timeout-coefficient 10 --exclude-files / .
git config diff.context 0; git config diff.interHunkContext 0   # one hunk per edit, so --diff sees every changed line
gremlins unleash --diff origin/main --workers 4 --exclude-files / .
```

Efficacy (killed / (killed + lived)) with `--workers 3 --timeout-coefficient 10` and `GOFLAGS=-count=1`. A timed-out mutant is one whose tests no longer finish, counted apart from both:

| Measured | Killed | Lived | Not covered | Timed out | Efficacy |
|---|---|---|---|---|---|
| 2026-11-19, first full run | 722 | 89 | 35 | 4 | 89.03% |
| 2026-11-27, after the fault injection tests | 671 | 118 | 27 | 11 | 85.04% |

The second run has fewer mutants because unreachable guards were removed, and fewer not covered because the new tests reach most of the rest. The weekly run fails below 85%, and a pull request's changed lines below 90% (`.gremlins.yaml`). Most of the survivors move a boundary by one (`<` to `<=`) in the scrolling and layout arithmetic of `history.go`, `prompt.go` and `renderer.go`, which is where a test is worth adding next. Some cannot be killed: `TextBeforeCursor` returns the whole text for a cursor at the end of it whichever way its bound is compared.
