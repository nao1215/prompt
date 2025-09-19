// Package main demonstrates autocomplete functionality using only public APIs.
package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/nao1215/prompt"
)

// scrollTestCompleter provides many suggestions to test scrolling functionality
func scrollTestCompleter(d prompt.Document) []prompt.Suggestion {
	text := d.TextBeforeCursor()
	words := strings.Fields(text)

	// Many commands to test scrolling (15+ suggestions)
	commands := []prompt.Suggestion{
		{Text: "help", Description: "Show help information"},
		{Text: "list", Description: "List all items"},
		{Text: "create", Description: "Create a new item"},
		{Text: "delete", Description: "Delete an existing item"},
		{Text: "update", Description: "Update an existing item"},
		{Text: "status", Description: "Show current status"},
		{Text: "config", Description: "Configure application settings"},
		{Text: "backup", Description: "Create a backup"},
		{Text: "restore", Description: "Restore from backup"},
		{Text: "import", Description: "Import data from file"},
		{Text: "export", Description: "Export data to file"},
		{Text: "search", Description: "Search through items"},
		{Text: "filter", Description: "Filter items by criteria"},
		{Text: "sort", Description: "Sort items"},
		{Text: "validate", Description: "Validate data integrity"},
		{Text: "optimize", Description: "Optimize performance"},
		{Text: "migrate", Description: "Migrate data to new format"},
		{Text: "analyze", Description: "Analyze data patterns"},
		{Text: "report", Description: "Generate reports"},
		{Text: "schedule", Description: "Schedule tasks"},
		{Text: "monitor", Description: "Monitor system health"},
		{Text: "cleanup", Description: "Clean up temporary files"},
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
			// More items to test scrolling
			return []prompt.Suggestion{
				{Text: "item1", Description: "First item"},
				{Text: "item2", Description: "Second item"},
				{Text: "item3", Description: "Third item"},
				{Text: "item4", Description: "Fourth item"},
				{Text: "item5", Description: "Fifth item"},
				{Text: "item6", Description: "Sixth item"},
				{Text: "item7", Description: "Seventh item"},
				{Text: "item8", Description: "Eighth item"},
				{Text: "item9", Description: "Ninth item"},
				{Text: "item10", Description: "Tenth item"},
				{Text: "item11", Description: "Eleventh item"},
				{Text: "item12", Description: "Twelfth item"},
				{Text: "item13", Description: "Thirteenth item"},
				{Text: "item14", Description: "Fourteenth item"},
				{Text: "item15", Description: "Fifteenth item"},
			}
		case "create":
			return []prompt.Suggestion{
				{Text: "project", Description: "Create a new project"},
				{Text: "file", Description: "Create a new file"},
				{Text: "folder", Description: "Create a new folder"},
				{Text: "document", Description: "Create a new document"},
				{Text: "template", Description: "Create from template"},
				{Text: "database", Description: "Create new database"},
				{Text: "table", Description: "Create new table"},
				{Text: "index", Description: "Create new index"},
				{Text: "view", Description: "Create new view"},
				{Text: "procedure", Description: "Create stored procedure"},
				{Text: "function", Description: "Create new function"},
				{Text: "trigger", Description: "Create new trigger"},
			}
		}
	}

	return []prompt.Suggestion{}
}

func main() {
	fmt.Println("Scroll Test Autocomplete Example")
	fmt.Println("================================")
	fmt.Println("This example tests the scrolling functionality with many suggestions.")
	fmt.Println("")
	fmt.Println("How to test:")
	fmt.Println("1. Press Tab to see all 23 commands (tests basic scrolling)")
	fmt.Println("2. Type 'delete ' or 'update ' then Tab to see 15 items (tests scroll)")
	fmt.Println("3. Type 'create ' then Tab to see 12 creation options")
	fmt.Println("4. Use arrow keys to navigate through suggestions")
	fmt.Println("5. Notice how only 10 suggestions are shown at once")
	fmt.Println("")
	fmt.Println("Type 'exit' to quit")
	fmt.Println()

	// Create prompt with autocomplete using only public APIs
	p, err := prompt.New("app> ",
		prompt.WithCompleter(scrollTestCompleter),
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
			fmt.Println("Available commands (23 total - test scrolling!):")
			fmt.Println("  help      - Show this help")
			fmt.Println("  list      - List items")
			fmt.Println("  create    - Create new item (12 options)")
			fmt.Println("  delete    - Delete item (15 items)")
			fmt.Println("  update    - Update item (15 items)")
			fmt.Println("  status    - Show status")
			fmt.Println("  config    - Configure settings")
			fmt.Println("  backup    - Create backup")
			fmt.Println("  restore   - Restore from backup")
			fmt.Println("  import    - Import data")
			fmt.Println("  export    - Export data")
			fmt.Println("  search    - Search items")
			fmt.Println("  filter    - Filter items")
			fmt.Println("  sort      - Sort items")
			fmt.Println("  validate  - Validate data")
			fmt.Println("  optimize  - Optimize performance")
			fmt.Println("  migrate   - Migrate data")
			fmt.Println("  analyze   - Analyze patterns")
			fmt.Println("  report    - Generate reports")
			fmt.Println("  schedule  - Schedule tasks")
			fmt.Println("  monitor   - Monitor system")
			fmt.Println("  cleanup   - Clean up files")
			fmt.Println("  exit      - Exit program")
			fmt.Println("")
			fmt.Println("Scroll test instructions:")
			fmt.Println("- Press Tab with empty input to see all commands")
			fmt.Println("- Type 'delete ' + Tab to see scrollable item list")
			fmt.Println("- Use arrow keys to scroll through suggestions")
		case "status":
			fmt.Println("Status: Running")
		case "list":
			fmt.Println("Items: item1, item2, item3")
		default:
			fmt.Printf("Executed: %s\n", result)
		}
	}
}
