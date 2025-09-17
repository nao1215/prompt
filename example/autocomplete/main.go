// Package main demonstrates autocomplete functionality using only public APIs.
package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/nao1215/prompt"
)

// simpleCompleter provides basic autocomplete using only public APIs
func simpleCompleter(d prompt.Document) []prompt.Suggestion {
	text := d.TextBeforeCursor()
	words := strings.Fields(text)

	// Available commands
	commands := []prompt.Suggestion{
		{Text: "help", Description: "Show help information"},
		{Text: "list", Description: "List all items"},
		{Text: "create", Description: "Create a new item"},
		{Text: "delete", Description: "Delete an existing item"},
		{Text: "update", Description: "Update an existing item"},
		{Text: "status", Description: "Show current status"},
		{Text: "exit", Description: "Exit the program"},
	}

	// If no text or only one word being typed, suggest commands
	if len(words) == 0 || (len(words) == 1 && !strings.HasSuffix(text, " ")) {
		var suggestions []prompt.Suggestion
		prefix := ""
		if len(words) == 1 {
			prefix = strings.ToLower(words[0])
		}

		// Simple prefix matching using only public APIs
		for _, cmd := range commands {
			if prefix == "" || strings.HasPrefix(strings.ToLower(cmd.Text), prefix) {
				suggestions = append(suggestions, cmd)
			}
		}
		return suggestions
	}

	// Context-aware suggestions for specific commands
	if len(words) >= 1 {
		switch words[0] {
		case "delete", "update":
			return []prompt.Suggestion{
				{Text: "item1", Description: "First item"},
				{Text: "item2", Description: "Second item"},
				{Text: "item3", Description: "Third item"},
			}
		case "create":
			return []prompt.Suggestion{
				{Text: "project", Description: "Create a new project"},
				{Text: "file", Description: "Create a new file"},
				{Text: "folder", Description: "Create a new folder"},
			}
		}
	}

	return []prompt.Suggestion{}
}

func main() {
	fmt.Println("Simple Autocomplete Example")
	fmt.Println("==========================")
	fmt.Println("Press Tab to see suggestions")
	fmt.Println("Type 'help' to see available commands")
	fmt.Println("Type 'exit' to quit")
	fmt.Println()

	// Create prompt with autocomplete using only public APIs
	p, err := prompt.New("app> ",
		prompt.WithCompleter(simpleCompleter),
		prompt.WithColorScheme(prompt.ThemeNightOwl),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	for {
		result, err := p.Run()
		if err != nil {
			if errors.Is(err, prompt.ErrEOF) {
				fmt.Println("\nGoodbye!")
				break
			}
			log.Printf("Error: %v\n", err)
			continue
		}

		result = strings.TrimSpace(result)
		if result == "" {
			continue
		}

		// Handle commands
		args := strings.Fields(result)
		switch args[0] {
		case "exit", "quit":
			fmt.Println("Goodbye!")
			return
		case "help":
			fmt.Println("Available commands:")
			fmt.Println("  help    - Show this help")
			fmt.Println("  list    - List items")
			fmt.Println("  create  - Create new item")
			fmt.Println("  delete  - Delete item")
			fmt.Println("  update  - Update item")
			fmt.Println("  status  - Show status")
			fmt.Println("  exit    - Exit program")
		case "status":
			fmt.Println("Status: Running")
		case "list":
			fmt.Println("Items: item1, item2, item3")
		default:
			fmt.Printf("Executed: %s\n", result)
		}
	}
}
