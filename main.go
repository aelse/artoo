// Package main is the entry point for the Artoo agent application.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aelse/artoo/agent"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func main() {
	ctx := context.Background()
	client := anthropic.NewClient(
		option.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
	)
	a := agent.New(client)

	if err := a.Run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Terminated with error: %s\n", err.Error())
	}
}
