// Package main demonstrates history management features of the prompt library.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/nao1215/prompt"
)

func main() {
	fmt.Println("History Example with File Persistence")
	fmt.Println("Use Up/Down arrow keys to navigate history")
	fmt.Println("Type 'history' to see command history")
	fmt.Println("Type 'clear' to clear history")
	fmt.Println("Type 'exit' or 'quit' to exit")
	historyFile, err := defaultHistoryFile()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("History is automatically saved to %s\n", historyFile)
	fmt.Println()

	// Create prompt with file-based history persistence
	// History will be loaded from the file automatically if it exists.
	// As you use the prompt, commands will be saved to the history file.
	// You can specify history file paths in various formats:
	// - Absolute path: "/home/user/.my_app_history"
	// - Home directory: "~/.my_app_history"
	// - Relative path: "./app_history" (converted to absolute)
	p, err := prompt.New("history> ",
		prompt.WithFileHistory(historyFile, 1000),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	for {
		// Run the prompt with history enabled
		result, err := p.Run(context.Background())
		if err != nil {
			if errors.Is(err, prompt.ErrEOF) {
				fmt.Println("\nGoodbye!")
				break
			}
			log.Printf("Error: %v\n", err)
			continue
		}

		// Trim whitespace
		result = strings.TrimSpace(result)
		if result == "" {
			continue
		}

		// Handle special commands
		switch result {
		case "exit", "quit":
			fmt.Println("Goodbye!")
			return
		case "history":
			fmt.Println("Command History:")
			// Get current history from prompt
			currentHistory := p.History()
			for i, cmd := range currentHistory {
				fmt.Printf("  %3d: %s\n", i+1, cmd)
			}
		case "clear":
			// Clear history
			p.SetHistory(nil)
			fmt.Println("History cleared")
		default:
			// Add command to history
			p.AddHistory(result)
			fmt.Printf("Executed: %s\n", result)
		}
	}
}

// defaultHistoryFile is where this example keeps its history: under the
// user's configuration directory, which is $XDG_CONFIG_HOME on Linux.
func defaultHistoryFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "prompt", "history"), nil
}
