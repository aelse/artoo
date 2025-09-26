package main

import (
	"context"
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
	a.Run(ctx)
}
