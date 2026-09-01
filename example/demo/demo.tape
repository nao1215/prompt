# The animation in the README, recorded with https://github.com/charmbracelet/vhs.
#
# Run it from the repository root, which is what the Output path below is
# relative to:
#
#	make demo
#
# The program it drives is main.go beside this file, so the recording cannot
# show something the library does not do.

Output doc/img/demo.gif

Require go

Set Shell bash
Set FontSize 18
Set Width 1100
Set Height 720
Set Padding 24
Set TypingSpeed 55ms
Set Theme "Dracula"

# Build out of shot, so the recording opens on the prompt rather than on a
# compiler.
Hide
Type "go build -o /tmp/prompt-demo ./example/demo" Enter
Sleep 5s
Type "clear" Enter
Sleep 500ms
Show

Type "/tmp/prompt-demo" Enter
Sleep 1500ms

# Completion knows what can follow what: a keyword here, and the columns of the
# schema once SELECT has been typed.
Type "sel"
Sleep 600ms
Tab
Sleep 900ms
Type " "
Tab
Sleep 1200ms
Down
Sleep 400ms
Down
Sleep 700ms
Enter
Sleep 700ms

# And table names after FROM.
Type ", email from "
Sleep 400ms
Tab
Sleep 1200ms
Enter
Sleep 700ms
Type ";"
Sleep 500ms
Enter
Sleep 1800ms

# A statement that is not finished opens another line instead of running, and
# the keywords are colored as they are typed.
Type "select total, placed_at"
Sleep 600ms
Enter
Sleep 800ms
Type "  from orders"
Sleep 600ms
Enter
Sleep 800ms
Type "  where total > '20';"
Sleep 800ms
Enter
Sleep 1800ms

# Ctrl+R searches what has been run, matching anywhere in the entry.
Ctrl+R
Sleep 900ms
Type "ord"
Sleep 1500ms
Enter
Sleep 900ms
Enter
Sleep 1800ms

Type "\q"
Sleep 400ms
Enter
Sleep 1500ms
