// Package prompt reads lines from a terminal for an interactive program: a
// REPL, a shell, a tool that asks a question. It draws a prefix, edits the line
// the user types, offers completion on Tab, walks a history with the arrow keys
// and searches it with Ctrl+R, and hands the line back.
//
// It runs on Linux, macOS and Windows, and it is the successor of the archived
// c-bata/go-prompt: the same idea, without the panics and the leaked
// goroutines, and without go-prompt's API, which this package does not
// reproduce.
//
// # Reading a line
//
//	p, err := prompt.New("> ")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer p.Close()
//
//	line, err := p.Run(context.Background())
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(line)
//
// New opens the terminal and Close gives it back, so a program that forgets
// Close leaves the terminal as it found it only by luck; call it whatever Run
// returned. Run takes the terminal into raw mode for one entry and restores it
// before returning, which is what lets a program print between prompts as it
// always did.
//
// What Run returns says how the entry ended. Enter returns the line without its
// line break. Ctrl+C discards the line and returns ErrInterrupted. Ctrl+D on an
// empty line, the input reaching its end, and Close from another goroutine
// return ErrEOF, which matches io.EOF as well, so a loop that stops on either is
// right. The context ending returns its own error, and it is the only way to
// bound the wait, since a key the terminal has not sent cannot be waited for
// with a deadline of its own.
//
// # Options
//
// New takes options. WithCompleter names a function asked on Tab; WithTheme
// names the colors, from the Theme* variables or a ColorScheme of the caller's;
// WithKeyMap names the key bindings; WithMemoryHistory and WithFileHistory say
// how many entries the arrow keys walk and whether they outlive the process,
// and WithoutHistory says to keep none, which is what a prompt asking for a
// password or a token wants, since Run remembers every line it returns;
// WithMultiline, WithIsComplete, WithAutoIndent and WithContinuationPrefix
// shape an entry of several lines; WithHighlighter colors runs of the input as
// it is drawn; WithWordEscape reads a backslash before a space as part of the
// word being completed; WithPersistentRawMode keeps raw mode across Run calls
// for a program that never prints between them.
//
//	p, err := prompt.New("$ ",
//		prompt.WithCompleter(prompt.NewFuzzyCompleter([]string{"git status", "git commit"})),
//		prompt.WithMemoryHistory(100),
//		prompt.WithTheme(prompt.ThemeDracula),
//	)
//
// What an option sets can be changed while the prompt lives, for the few
// things a program changes: SetPrefix, SetTheme, and the history through
// AddHistory, SetHistory and History.
//
// # Completion
//
// A completer is a function from a Document, which is the whole input and
// where the cursor is, to a list of Suggestions. It is asked on Tab. By default
// the prompt takes the word before the cursor and keeps a suggestion only when
// that word is a case-sensitive prefix of it, and accepting a suggestion
// replaces that word. A completer that matches by another rule -- a qualified
// name, a case-insensitive match -- sets Suggestion.Replace to the span it
// stands for, counted in runes like Document.CursorPosition, and the prompt
// applies that span literally instead:
//
//	func completer(d prompt.Document) []prompt.Suggestion {
//		word := d.WordBeforeCursor()
//		start := d.CursorPosition - len([]rune(word))
//
//		var out []prompt.Suggestion
//		for _, kw := range []string{"SELECT", "INSERT", "UPDATE"} {
//			if strings.HasPrefix(strings.ToLower(kw), strings.ToLower(word)) {
//				out = append(out, prompt.Suggestion{
//					Text:    kw,
//					Replace: &prompt.Range{Start: start, End: d.CursorPosition},
//				})
//			}
//		}
//		return out
//	}
//
// The menu stands for the word before the cursor, so editing the line or
// moving the cursor off that word ends it and the next Tab asks again. It lists
// at most ten candidates, and fewer when the terminal has fewer rows to spare
// under the line being typed; Up and Down scroll a longer list. NewFuzzyCompleter
// and NewFileCompleter are two completers this package ships.
//
// # Keys
//
// The default key map binds what a shell binds: Enter submits; Ctrl+C
// discards the entry; Ctrl+D on an empty entry ends the input; Tab completes
// and Escape closes the menu; Up and Down walk the history, or the lines of a
// multiline entry; Left, Right, Home, End, Ctrl+A and Ctrl+E move the cursor,
// and Ctrl+Left and Ctrl+Right move it by a word; Backspace and Delete delete
// a character, Ctrl+W the word before the cursor, Ctrl+K to the end of the
// line, and Ctrl+U the line the cursor is on; Ctrl+R searches the history,
// where Tab and the arrow keys move through the matches, Enter accepts and
// Escape cancels; Ctrl+L clears the screen and leaves the scrollback.
//
// A KeyMap from NewDefaultKeyMap can be changed with Bind and BindSequence and
// given to New with WithKeyMap. ActionHistoryUp and ActionHistoryDown are bound
// to no key by default, because on a multiline entry the arrow keys are moving
// the cursor; a shell puts them on Ctrl+P and Ctrl+N:
//
//	keyMap := prompt.NewDefaultKeyMap()
//	keyMap.Bind('\x10', prompt.ActionHistoryUp)   // Ctrl+P
//	keyMap.Bind('\x0E', prompt.ActionHistoryDown) // Ctrl+N
//	keyMap.BindSequence("OP", prompt.ActionComplete) // F1
//
// # Multiline entries
//
// With WithMultiline, an entry may hold line breaks: Enter inserts one while
// the WithIsComplete predicate says the entry is not finished, and submits it
// once it is. A line ending in an odd number of backslashes is continued
// whatever the predicate says, and in either mode, with the last of them taken
// out of the entry; an even number is data, so an entry that ends in a
// backslash is written by typing two. Up and Down then move between lines, and Home and End to the
// ends of the line the cursor is on. An entry taller than the terminal is
// drawn as the rows around the cursor the terminal has room for, redrawn in
// place, so what scrolls off the top of the screen is the program's output
// rather than the prompt's. Whichever line the cursor was on, an entry ends at
// its foot, so what the program prints next starts below it.
//
// # What a byte does
//
// A key the terminal sends as a byte below a space is what the key map binds it
// to, and a byte it binds to nothing is dropped: putting a raw control byte
// into the line would be worse than losing it. Ctrl+D is the one whose answer
// depends on the line -- it ends the input when there is nothing on it, and
// does nothing when there is.
//
// A byte that is not valid UTF-8 is typed as U+FFFD, the replacement character.
// The entry an application receives then holds a character nobody pressed, and
// the byte that produced it is gone; that is the answer rather than dropping
// the byte, because a paste of something that is not text should be visible in
// the line rather than silently shorter, and rather than keeping the byte,
// because a caller reading the entry would otherwise hold a string that is not
// valid UTF-8.
//
// # Pasting
//
// A terminal that supports bracketed paste wraps a paste in markers, and
// between them every key is content: a newline does not submit, Tab does not
// complete, and Ctrl+C does not interrupt. That is what keeps a pasted
// statement from running itself a line at a time. An escape sequence inside a
// paste -- which copied terminal output is full of -- keeps its text and loses
// the ESC that introduced it.
//
// # Between prompts
//
// A REPL spends most of its time not reading the terminal. During that time
// Ctrl+C either waits in the terminal's buffer as a byte, or, once the prompt
// has given raw mode back, arrives as a SIGINT that ends the process.
// WatchInterrupt watches for both while the program works between prompts and
// cancels the context it returns instead; the watch runs until its stop
// function is called, and Run is not to be called while one is active.
//
// # Goroutines
//
// A Prompt is used from one goroutine. The exception is ending a session,
// because a prompt waiting for a key cannot end itself: canceling the context
// given to Run returns context.Canceled, and Close from another goroutine
// returns ErrEOF. Close may be called more than once.
package prompt
