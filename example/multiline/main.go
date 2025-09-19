// Package main demonstrates multiline input capabilities of the prompt library.
package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/nao1215/prompt"
)

func main() {
	fmt.Println("Multiline Input Example")
	fmt.Println("Enter text:")
	fmt.Println("  - Use backslash (\\) at line end + Enter for line continuation")
	fmt.Println("  - Press Enter without backslash to submit")
	fmt.Println("Type 'exit' to quit")
	fmt.Println()

	// Create prompt for multiline input
	p, err := prompt.New("multi> ",
		prompt.WithMultiline(true),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	for {
		fmt.Println("Enter your text:")

		// Run the prompt
		result, err := p.Run()
		if err != nil {
			if errors.Is(err, prompt.ErrEOF) {
				fmt.Println("\nGoodbye!")
				break
			}
			log.Printf("Error: %v\n", err)
			continue
		}

		// Check for exit command
		if strings.TrimSpace(result) == "exit" {
			fmt.Println("Goodbye!")
			return
		}

		// Display the input with line numbers
		fmt.Println("\n--- Your input ---")
		lines := strings.Split(result, "\n")
		for i, line := range lines {
			fmt.Printf("%3d: %s\n", i+1, line)
		}
		fmt.Printf("\nTotal lines: %d\n", len(lines))
		fmt.Printf("Total characters: %d\n", len(result))
		fmt.Println("--- End of input ---")
	}
}
