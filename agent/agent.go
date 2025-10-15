// Package agent provides the core agent functionality for interacting with Claude.
package agent

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
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

const (
	maxTokens           = 1024
	spinnerTickInterval = 100 * time.Millisecond
)

var (
	errMinGreaterThanMax = errors.New("min value cannot be greater than max value")

	titleStyle  lipgloss.Style
	userStyle   lipgloss.Style
	claudeStyle lipgloss.Style
	errorStyle  lipgloss.Style
)

func init() {
	titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true) // Bright cyan
	userStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))             // Green
	claudeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))           // Blue
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)  // Red
}

// spinnerRunner manages a simple terminal spinner.
type spinnerRunner struct {
	model   spinner.Model
	message string
	quit    chan bool
	done    sync.WaitGroup
}

// newSpinner creates a new spinner with the given message.
func newSpinner(message string) *spinnerRunner {
	s := spinner.New()
	s.Spinner = spinner.Points
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return &spinnerRunner{
		model:   s,
		message: message,
		quit:    make(chan bool),
	}
}

// start begins the spinner animation.
func (s *spinnerRunner) start() {
	s.done.Add(1)

	go func() {
		defer s.done.Done()

		ticker := time.NewTicker(spinnerTickInterval)
		defer ticker.Stop()

		for {
			select {
			case <-s.quit:
				// Clear the spinner line.
				_, _ = fmt.Fprint(os.Stdout, "\r\033[K")

				return
			case <-ticker.C:
				s.model, _ = s.model.Update(s.model.Tick())
				frame := s.model.View()
				_, _ = fmt.Fprintf(os.Stdout, "\r%s %s", frame, s.message)
			}
		}
	}()
}

// stop ends the spinner animation and clears the line.
func (s *spinnerRunner) stop() {
	close(s.quit)
	s.done.Wait()
}

func New(client anthropic.Client) *Agent {
	return &Agent{
		client:       client,
		conversation: make([]anthropic.MessageParam, 0),
	}
}

func (a *Agent) generateRandomNumber(params RandomNumberParams) (*RandomNumberResponse, error) {
	if params.Min > params.Max {
		return nil, errMinGreaterThanMax
	}

	// Use crypto/rand for secure random number generation.
	rangeSize := params.Max - params.Min + 1
	n, err := rand.Int(rand.Reader, big.NewInt(int64(rangeSize)))

	if err != nil {
		return nil, fmt.Errorf("generating random number: %w", err)
	}

	return &RandomNumberResponse{Number: int(n.Int64()) + params.Min}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	tools := a.setupTools()

	_, _ = fmt.Fprintln(os.Stdout, titleStyle.Render("Artoo Agent")+" - Type 'quit' to exit")

	readyForUserInput := true

	for {
		var userInput string

		if readyForUserInput {
			userInput = a.getUserInput(scanner)
			if userInput == "" {
				break
			}

			if userInput == "quit" || userInput == "exit" {
				break
			}

			a.conversation = append(a.conversation, anthropic.NewUserMessage(
				anthropic.NewTextBlock(userInput),
			))
			readyForUserInput = false
		}

		a.printConversation()

		message, err := a.callClaude(ctx, tools)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stdout, "%s\n", errorStyle.Render(fmt.Sprintf("Error: %v", err)))

			continue
		}

		toolResults, hasToolUse := a.processResponse(message)

		if len(toolResults) > 0 {
			a.conversation = append(a.conversation, anthropic.NewUserMessage(toolResults...))
		}

		if !hasToolUse {
			readyForUserInput = true
		}

		_, _ = fmt.Fprint(os.Stdout, "\n\n")
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	return nil
}

func (a *Agent) setupTools() []anthropic.ToolUnionParam {
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

	return tools
}

func (a *Agent) getUserInput(scanner *bufio.Scanner) string {
	_, _ = fmt.Fprint(os.Stdout, userStyle.Render("You")+": ")

	if !scanner.Scan() {
		return ""
	}

	userInput := strings.TrimSpace(scanner.Text())
	if userInput == "" {
		return userInput
	}

	_, _ = fmt.Fprintln(os.Stdout, "user input: "+userInput)

	return userInput
}

func (a *Agent) printConversation() {
	_, _ = fmt.Fprintf(os.Stdout, "Calling claude with conversation:\n")

	for i := range a.conversation {
		m, err := json.Marshal(a.conversation[i])
		if err != nil {
			_, _ = fmt.Fprintf(os.Stdout, "[%d] error marshalling: %v\n", i, err)

			continue
		}

		_, _ = fmt.Fprintf(os.Stdout, "[%d] %s\n", i, string(m))
	}
}

func (a *Agent) callClaude(ctx context.Context, tools []anthropic.ToolUnionParam) (*anthropic.Message, error) {
	spin := newSpinner("Thinking...")
	spin.start()
	defer spin.stop()

	message, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude4Sonnet20250514,
		MaxTokens: maxTokens,
		Messages:  a.conversation,
		Tools:     tools,
	})

	return message, err
}

func (a *Agent) processResponse(message *anthropic.Message) ([]anthropic.ContentBlockParamUnion, bool) {
	var toolResults []anthropic.ContentBlockParamUnion

	hasToolUse := false

	_, _ = fmt.Fprint(os.Stdout, claudeStyle.Render("Claude")+": ")

	a.printMessageContent(message)
	a.conversation = append(a.conversation, message.ToParam())

	for _, block := range message.Content {
		variant, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok {
			continue
		}

		hasToolUse = true

		result := a.handleToolUse(variant)
		if result != nil {
			toolResults = append(toolResults, *result)
		}
	}

	return toolResults, hasToolUse
}

func (a *Agent) printMessageContent(message *anthropic.Message) {
	for _, block := range message.Content {
		switch block := block.AsAny().(type) {
		case anthropic.TextBlock:
			_, _ = fmt.Fprintln(os.Stdout, "text: "+block.Text)
		case anthropic.ToolUseBlock:
			inputJSON, err := json.Marshal(block.Input)
			if err != nil {
				_, _ = fmt.Fprintln(os.Stdout, "error marshalling input: "+err.Error())

				continue
			}

			_, _ = fmt.Fprintln(os.Stdout, block.Name+": "+string(inputJSON))
		}
	}
}

func (a *Agent) handleToolUse(block anthropic.ToolUseBlock) *anthropic.ContentBlockParamUnion {
	if block.Name == "generate_random_number" {
		spin := newSpinner("Generating random number...")
		spin.start()
		defer spin.stop()

		var params RandomNumberParams

		err := json.Unmarshal([]byte(block.JSON.Input.Raw()), &params)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stdout, "error unmarshalling params: %v\n", err)

			return nil
		}

		randomNumResp, err := a.generateRandomNumber(params)
		if err != nil {
			result := anthropic.NewToolResultBlock(block.ID, fmt.Sprintf("Error: %v", err), true)

			return &result
		}

		_, _ = fmt.Fprintf(os.Stdout, "[Generated random number: %d]\n", randomNumResp.Number)

		result := anthropic.NewToolResultBlock(block.ID, strconv.Itoa(randomNumResp.Number), false)

		b, err := json.Marshal(randomNumResp)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stdout, "error marshalling tool response: "+err.Error())

			return &result
		}

		_, _ = fmt.Fprintln(os.Stdout, string(b))

		return &result
	}

	return nil
}

func Run(ctx context.Context) error {
	client := anthropic.NewClient()
	agent := New(client)

	return agent.Run(ctx)
}
