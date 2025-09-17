// Package main demonstrates basic usage of the prompt library.
package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/nao1215/prompt"
)

func main() {
	// Create a simple prompt with default settings
	p, err := prompt.New(">>> ")
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	fmt.Println("Basic Prompt Example")
	fmt.Println("Type 'exit' or 'quit' to exit")
	fmt.Println("Press Ctrl+D to exit")
	fmt.Println()

	for {
		// Run the prompt and get user input
		result, err := p.Run()
		if err != nil {
			if errors.Is(err, prompt.ErrEOF) {
				fmt.Println("\nGoodbye!")
				break
			}
			log.Printf("Error: %v\n", err)
			continue
		}

		// Handle exit commands
		if result == "exit" || result == "quit" {
			fmt.Println("Goodbye!")
			break
		}

		// Echo the input back
		fmt.Printf("You typed: %s\n", result)
	}
}
