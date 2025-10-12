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

type RandomNumberResponse struct {
	Number int `json:"number"`
}

func New(client anthropic.Client) *Agent {
	rand.Seed(time.Now().UnixNano())
	return &Agent{
		client:       client,
		conversation: make([]anthropic.MessageParam, 0),
	}
}

func (a *Agent) generateRandomNumber(params RandomNumberParams) (*RandomNumberResponse, error) {
	if params.Min > params.Max {
		return nil, fmt.Errorf("min value cannot be greater than max value")
	}
	return &RandomNumberResponse{rand.Intn(params.Max-params.Min+1) + params.Min}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)

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

	fmt.Println(ansi.BrightCyan + "Artoo Agent" + ansi.Reset + " - Type 'quit' to exit")

	readyForUserInput := true

	for {
		var userInput string
		if readyForUserInput {
			fmt.Print(ansi.Green + "You" + ansi.Reset + ": ")

			if !scanner.Scan() {
				break
			}

			userInput = strings.TrimSpace(scanner.Text())

			if userInput == "quit" || userInput == "exit" {
				break
			}

			if userInput == "" {
				continue
			}

			fmt.Println("user input:", userInput)

			// Add user message to conversation
			a.conversation = append(a.conversation, anthropic.NewUserMessage(
				anthropic.NewTextBlock(userInput),
			))

			readyForUserInput = false
		}

		fmt.Printf("Calling claude with conversation:\n")
		for i := range a.conversation {
			m, _ := json.Marshal(a.conversation[i])
			fmt.Printf("[%d] %s\n", i, string(m))
		}

		// Call Claude API
		message, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
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
		var toolResults []anthropic.ContentBlockParamUnion
		hasToolUse := false

		fmt.Print(ansi.Blue + "Claude" + ansi.Reset + ": ")

		for _, block := range message.Content {
			switch block := block.AsAny().(type) {
			case anthropic.TextBlock:
				fmt.Println("text: " + block.Text)
			case anthropic.ToolUseBlock:
				inputJSON, _ := json.Marshal(block.Input)
				fmt.Println(block.Name + ": " + string(inputJSON))
			}
		}

		a.conversation = append(a.conversation, message.ToParam())

		for _, block := range message.Content {
			switch variant := block.AsAny().(type) {
			case anthropic.ToolUseBlock:
				hasToolUse = true
				fmt.Print("[user (" + block.Name + ")]: ")
				var response interface{}
				switch block.Name {
				case "generate_random_number":
					var params RandomNumberParams
					err := json.Unmarshal([]byte(variant.JSON.Input.Raw()), &params)
					if err != nil {
						panic(err)
					}
					randomNumResp, err := a.generateRandomNumber(params)
					if err != nil {
						toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, fmt.Sprintf("Error: %v", err), true))
					} else {
						fmt.Printf("\n[Generated random number: %d]", randomNumResp.Number)
						response = randomNumResp
						toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, fmt.Sprintf("%d", randomNumResp.Number), false))
					}
				}

				b, err := json.Marshal(response)
				if err != nil {
					panic("error marshalling tool response")
				}
				println(string(b))
				//toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, string(b), false))
			}
		}

		if len(toolResults) > 0 {
			a.conversation = append(a.conversation, anthropic.NewUserMessage(toolResults...))
		}

		// If tools were used, send the results back to Claude by not setting ready for user input
		if !hasToolUse {
			readyForUserInput = true
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
