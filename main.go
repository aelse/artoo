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

	// Load configuration from environment variables
	cfg := LoadConfig()

	// Create API client
	client := anthropic.NewClient(
		option.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
	)

	// Create terminal UI
	term := ui.NewTerminal()
	term.PrintTitle()

	// Create agent with loaded config
	a := agent.New(client, cfg.Agent)

	// Update conversation with config (for context management)
	a.SetConversationConfig(cfg.Conversation)

	// Debug logging if enabled
	if cfg.Debug {
		fmt.Fprintf(os.Stderr, "Debug: Model=%s MaxTokens=%d MaxContext=%d\n",
			cfg.Agent.Model, cfg.Agent.MaxTokens, cfg.Conversation.MaxContextTokens)
	}

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
