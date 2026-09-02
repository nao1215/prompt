// Package main provides a shell-like file explorer example using the prompt library.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nao1215/prompt"
)

// The shell's command names, so the completer and the dispatcher cannot drift
// apart.
const (
	cmdLs   = "ls"
	cmdCd   = "cd"
	cmdCat  = "cat"
	cmdPwd  = "pwd"
	cmdExit = "exit"
)

func main() {
	fmt.Println("Shell-like File Explorer Example")
	fmt.Println("================================")
	fmt.Println("Commands:")
	fmt.Println("  ls [path]    - List directory contents")
	fmt.Println("  cd [path]    - Change directory")
	fmt.Println("  cat [file]   - Show file contents")
	fmt.Println("  pwd          - Show current directory")
	fmt.Println("  exit/quit    - Exit")
	fmt.Println()
	fmt.Println("Use Tab for file/directory completion!")
	fmt.Println("Use ↑/↓ arrow keys to navigate suggestions")
	fmt.Println()

	// Create a smart completer that handles command context
	completer := createShellCompleter()

	p, err := prompt.New("shell> ",
		prompt.WithCompleter(completer),
		prompt.WithMemoryHistory(1000),
	)
	if err != nil {
		log.Fatalf("failed to create prompt: %v", err)
	}
	defer p.Close()

	for {
		// Update prompt with current directory
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "unknown"
		}
		p.SetPrefix(fmt.Sprintf("shell:%s> ", filepath.Base(cwd)))

		result, err := p.Run()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			break
		}

		result = strings.TrimSpace(result)

		// Handle exit commands
		if result == cmdExit || result == "quit" {
			fmt.Println("Goodbye!")
			break
		}

		if result == "" {
			continue
		}

		// Add to history
		p.AddHistory(result)

		// Parse and execute command
		executeCommand(result)
		fmt.Println()
	}
}

func createShellCompleter() func(prompt.Document) []prompt.Suggestion {
	fileCompleter := prompt.NewFileCompleter()

	return func(d prompt.Document) []prompt.Suggestion {
		text := d.TextBeforeCursor()

		// Split into words
		words := strings.Fields(text)
		if len(words) == 0 {
			// No command yet, suggest commands
			return []prompt.Suggestion{
				{Text: "ls ", Description: "list directory contents"},
				{Text: "cd ", Description: "change directory"},
				{Text: "cat ", Description: "show file contents"},
				{Text: cmdPwd, Description: "print working directory"},
				{Text: cmdExit, Description: "exit shell"},
			}
		}

		cmd := words[0]

		// If we're typing the first word and it's incomplete, suggest commands
		if len(words) == 1 && !strings.HasSuffix(text, " ") {
			commands := []prompt.Suggestion{
				{Text: cmdLs, Description: "list directory contents"},
				{Text: cmdCd, Description: "change directory"},
				{Text: cmdCat, Description: "show file contents"},
				{Text: cmdPwd, Description: "print working directory"},
				{Text: cmdExit, Description: "exit shell"},
			}

			var filtered []prompt.Suggestion
			for _, c := range commands {
				if strings.HasPrefix(c.Text, cmd) {
					filtered = append(filtered, c)
				}
			}
			return filtered
		}

		// For file/directory arguments, use file completer. It completes the word
		// before the cursor, so the command in front of the path is no trouble
		// and the document goes to it as it is. Offering the command with the
		// path would be worse than useless: a suggestion is measured against the
		// word being typed, and "cat /etc/hosts" does not start with "/etc/ho".
		if cmd == cmdLs || cmd == cmdCd || cmd == cmdCat {
			return fileCompleter(d)
		}

		return nil
	}
}

func executeCommand(input string) {
	words := strings.Fields(input)
	if len(words) == 0 {
		return
	}

	cmd := words[0]
	args := words[1:]

	switch cmd {
	case cmdPwd:
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Println(cwd)
		}

	case cmdLs:
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		fmt.Printf("Contents of %s:\n", path)
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				fmt.Printf("  %s/\n", name)
			} else {
				fmt.Printf("  %s\n", name)
			}
		}

	case cmdCd:
		if len(args) == 0 {
			fmt.Println("Error: cd requires a directory argument")
			return
		}

		err := os.Chdir(args[0])
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			cwd, err := os.Getwd()
			if err != nil {
				cwd = "unknown"
			}
			fmt.Printf("Changed to: %s\n", cwd)
		}

	case cmdCat:
		if len(args) == 0 {
			fmt.Println("Error: cat requires a file argument")
			return
		}

		content, err := os.ReadFile(args[0])
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		// Limit output for large files
		if len(content) > 1000 {
			fmt.Printf("File content (first 1000 bytes):\n%s\n... (truncated)\n", content[:1000])
		} else {
			fmt.Printf("File content:\n%s\n", content)
		}

	default:
		// Try to execute as external command
		// #nosec G204 - This is an example program that intentionally executes user input
		execCmd := exec.CommandContext(context.Background(), cmd, args...)
		output, err := execCmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error executing '%s': %v\n", cmd, err)
		} else {
			fmt.Print(string(output))
		}
	}
}
