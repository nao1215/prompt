// Package main demonstrates history management features of the prompt library.
package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/nao1215/prompt"
)

func main() {
	fmt.Println("History Example with File Persistence")
	fmt.Println("Use Up/Down arrow keys to navigate history")
	fmt.Println("Type 'history' to see command history")
	fmt.Println("Type 'clear' to clear history")
	fmt.Println("Type 'exit' or 'quit' to exit")
	fmt.Printf("History is automatically saved to %s\n", prompt.GetDefaultHistoryFile())
	fmt.Println()

	// Create prompt with file-based history persistence
	// History will be loaded from the file automatically if it exists.
	// As you use the prompt, commands will be saved to the history file.
	// You can specify history file paths in various formats:
	// - XDG compliant (recommended): prompt.GetDefaultHistoryFile()
	// - Absolute path: "/home/user/.my_app_history"
	// - Home directory: "~/.my_app_history"
	// - Relative path: "./app_history" (converted to absolute)
	p, err := prompt.New("history> ",
		prompt.WithFileHistory(prompt.GetDefaultHistoryFile(), 1000), // XDG compliant: ~/.config/prompt/history
	)
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	for {
		// Run the prompt with history enabled
		result, err := p.Run()
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
			currentHistory := p.GetHistory()
			for i, cmd := range currentHistory {
				fmt.Printf("  %3d: %s\n", i+1, cmd)
			}
		case "clear":
			// Clear history
			p.ClearHistory()
			fmt.Println("History cleared")
		default:
			// Add command to history
			p.AddHistory(result)
			fmt.Printf("Executed: %s\n", result)
		}
	}
}
