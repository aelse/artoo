// Package main is the entry point for the Artoo agent application.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aelse/artoo/agent"
	"github.com/aelse/artoo/tool"
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

	// Load plugins and create agent
	extraTools := loadAndValidatePlugins(cfg)
	a := agent.New(client, cfg.Agent, extraTools...)

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

func loadAndValidatePlugins(cfg AppConfig) []tool.Tool {
	plugins, errs := tool.LoadPlugins(cfg.Agent.PluginDir, cfg.Agent.PluginTimeout)
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}

	if len(plugins) == 0 {
		return nil
	}

	merged, err := tool.MergeTools(tool.AllTools, plugins)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if cfg.Debug {
		fmt.Fprintf(os.Stderr, "Debug: Loaded %d plugins\n", len(plugins))
		for _, p := range plugins {
			fmt.Fprintf(os.Stderr, "Debug:   - %s\n", p.Param().Name)
		}
	}

	_ = merged // validation only; extraTools passed to agent

	return plugins
}
