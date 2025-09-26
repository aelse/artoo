package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/aelse/artoo/ansi"
	"github.com/anthropics/anthropic-sdk-go"
)

type Agent struct {
	client       anthropic.Client
	conversation []anthropic.MessageParam
}

type RandomNumberParams struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

func New(client anthropic.Client) *Agent {
	rand.Seed(time.Now().UnixNano())
	return &Agent{
		client:       client,
		conversation: make([]anthropic.MessageParam, 0),
	}
}

func (a *Agent) generateRandomNumber(params RandomNumberParams) (int, error) {
	if params.Min > params.Max {
		return 0, fmt.Errorf("min value cannot be greater than max value")
	}
	return rand.Intn(params.Max-params.Min+1) + params.Min, nil
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

		// Define tools
		toolParams := []anthropic.ToolParam{
			{
				Name:        "generate_random_number",
				Description: anthropic.String("Generate a random number between min and max values (inclusive)"),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"min": map[string]interface{}{
							"type":        "integer",
							"description": "Minimum value (inclusive)",
						},
						"max": map[string]interface{}{
							"type":        "integer",
							"description": "Maximum value (inclusive)",
						},
					},
					Required: []string{"min", "max"},
				},
			},
		}

		tools := make([]anthropic.ToolUnionParam, len(toolParams))
		for i, toolParam := range toolParams {
			tools[i] = anthropic.ToolUnionParam{OfTool: &toolParam}
		}

		// Call Claude API
		response, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaude4Sonnet20250514,
			MaxTokens: 1024,
			Messages:  a.conversation,
			Tools:     tools,
		})

		if err != nil {
			fmt.Printf(ansi.Red+"Error: %v"+ansi.Reset+"\n", err)
			continue
		}

		// Process Claude's response
		var responseContent []anthropic.ContentBlockParamUnion
		var toolResults []anthropic.ContentBlockParamUnion
		hasToolUse := false

		fmt.Print(ansi.Blue + "Claude" + ansi.Reset + ": ")

		for _, content := range response.Content {
			switch block := content.AsAny().(type) {
			case anthropic.TextBlock:
				fmt.Print(block.Text)
				responseContent = append(responseContent, anthropic.NewTextBlock(block.Text))
			case anthropic.ToolUseBlock:
				hasToolUse = true
				responseContent = append(responseContent, anthropic.NewToolUseBlock(block.ID, block.Name, string(block.Input)))

				// Execute the tool
				if block.Name == "generate_random_number" {
					var params RandomNumberParams
					inputJSON, _ := json.Marshal(block.Input)
					if err := json.Unmarshal(inputJSON, &params); err != nil {
						toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, fmt.Sprintf("Error parsing parameters: %v", err), true))
					} else {
						randomNum, err := a.generateRandomNumber(params)
						if err != nil {
							toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, fmt.Sprintf("Error: %v", err), true))
						} else {
							fmt.Printf("\n[Generated random number: %d]", randomNum)
							toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, fmt.Sprintf("%d", randomNum), false))
						}
					}
				}
			}
		}

		// Add Claude's initial response to conversation
		if len(responseContent) > 0 {
			a.conversation = append(a.conversation, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleAssistant,
				Content: responseContent,
			})
		}

		// If tools were used, send the results back to Claude
		if hasToolUse && len(toolResults) > 0 {
			a.conversation = append(a.conversation, anthropic.MessageParam{
				Role:    anthropic.MessageParamRoleUser,
				Content: toolResults,
			})

			// Get Claude's final response
			finalResponse, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
				Model:     anthropic.ModelClaude4Sonnet20250514,
				MaxTokens: 1024,
				Messages:  a.conversation,
				Tools:     tools,
			})

			if err != nil {
				fmt.Printf(ansi.Red+"\nError getting final response: %v"+ansi.Reset+"\n", err)
			} else {
				var finalContent []anthropic.ContentBlockParamUnion
				for _, content := range finalResponse.Content {
					switch block := content.AsAny().(type) {
					case anthropic.TextBlock:
						fmt.Print(block.Text)
						finalContent = append(finalContent, anthropic.NewTextBlock(block.Text))
					}
				}

				if len(finalContent) > 0 {
					a.conversation = append(a.conversation, anthropic.MessageParam{
						Role:    anthropic.MessageParamRoleAssistant,
						Content: finalContent,
					})
				}
			}
		}

		fmt.Print("\n\n")
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
