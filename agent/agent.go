package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aelse/artoo/ansi"
	"github.com/anthropics/anthropic-sdk-go"
)

type Agent struct {
	client       anthropic.Client
	conversation []anthropic.MessageParam
}

func New(client anthropic.Client) *Agent {
	return &Agent{
		client:       client,
		conversation: make([]anthropic.MessageParam, 0),
	}
}

func (a *Agent) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println(ansi.BrightCyan + "Claude Agent" + ansi.Reset + " - Type 'quit' to exit")

	for {
		fmt.Print(ansi.Green + "You" + ansi.Reset + ": ")

		if !scanner.Scan() {
			break
		}

		userInput := strings.TrimSpace(scanner.Text())

		if userInput == "quit" || userInput == "exit" {
			break
		}

		if userInput == "" {
			continue
		}

		// Add user message to conversation
		a.conversation = append(a.conversation, anthropic.MessageParam{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock(userInput),
			},
		})

		// Call Claude API
		response, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaude4Sonnet20250514,
			MaxTokens: 1024,
			Messages:  a.conversation,
		})

		if err != nil {
			fmt.Printf(ansi.Red+"Error: %v"+ansi.Reset+"\n", err)
			continue
		}

		// Extract and print Claude's response
		var responseContent []anthropic.ContentBlockParamUnion
		fmt.Print(ansi.Blue + "Claude" + ansi.Reset + ": ")

		for _, content := range response.Content {
			if content.Type == "text" {
				fmt.Print(content.Text)
				responseContent = append(responseContent, anthropic.NewTextBlock(content.Text))
			}
		}
		fmt.Print("\n\n")

		// Add Claude's response to conversation
		if len(responseContent) > 0 {
			a.conversation = append(a.conversation, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleAssistant,
				Content: responseContent,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	return nil
}

func Run(ctx context.Context) error {
	client := anthropic.NewClient()
	agent := New(client)
	return agent.Run(ctx)
}
