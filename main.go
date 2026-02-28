// Package main is the entry point for the Artoo agent application.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aelse/artoo/agent"
	"github.com/aelse/artoo/ui"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func main() {
	ctx := context.Background()

	// Create API client
	client := anthropic.NewClient(
		option.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
	)

	// Create terminal UI
	term := ui.NewTerminal()
	term.PrintTitle()

	// Create agent with default config
	a := agent.New(client, agent.DefaultConfig())

	// REPL loop: read input, send message, repeat
	for {
		input, err := term.ReadInput()
		if err != nil {
			term.PrintError(err)
			break
		}

		// Empty input or quit commands end the loop
		if input == "" || input == "quit" || input == "exit" {
			break
		}

		// Send message to agent
		_, err = a.SendMessage(ctx, input, term)
		if err != nil {
			term.PrintError(err)
		}

		// Print spacing between iterations
		fmt.Println()
	}
}
