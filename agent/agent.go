// Package agent provides the core agent functionality for interacting with Claude.
package agent

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"github.com/aelse/artoo/conversation"
	"github.com/aelse/artoo/tool"
	"github.com/anthropics/anthropic-sdk-go"
)

// toolResult holds a tool execution result with its original index
// to preserve ordering after concurrent execution.
type toolResult struct {
	index  int
	result anthropic.ContentBlockParamUnion
}

// Agent manages the conversation with Claude and tool execution.
type Agent struct {
	client          anthropic.Client
	conversation    *conversation.Conversation
	tools           []tool.Tool
	toolMap         map[string]tool.Tool
	toolUnionParams []anthropic.ToolUnionParam
	config          Config
}

// New creates a new Agent with the given client and config.
// Additional tools can be provided via the extraTools parameter.
func New(client anthropic.Client, config Config, extraTools ...tool.Tool) *Agent {
	allTools := make([]tool.Tool, 0, len(tool.AllTools)+len(extraTools))
	allTools = append(allTools, tool.AllTools...)
	allTools = append(allTools, extraTools...)

	return &Agent{
		client:          client,
		conversation:    conversation.New(),
		tools:           allTools,
		toolMap:         makeToolMap(allTools),
		toolUnionParams: makeToolUnionParams(allTools),
		config:          config,
	}
}

// SetConversationConfig updates the conversation's configuration.
// This allows the agent to use custom context management settings.
func (a *Agent) SetConversationConfig(cfg conversation.Config) {
	a.conversation = conversation.NewWithConfig(cfg)
}

// SendMessage sends a user message and handles the agentic loop (API calls + tool use).
// It calls callbacks so the UI layer can observe what happens without the agent
// knowing about terminals.
//
// The loop continues until the assistant stops requesting tools.
func (a *Agent) SendMessage(ctx context.Context, text string, cb Callbacks) (*Response, error) {
	// Append user message to conversation
	a.conversation.Append(anthropic.NewUserMessage(
		anthropic.NewTextBlock(text),
	))

	var finalText string
	var finalStopReason string

	// Tool-use loop: call API, execute any tools, repeat until no more tools
	for {
		// Trim conversation if approaching context window limit before making API call
		a.conversation.Trim()

		cb.OnThinking()
		message, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(a.config.Model),
			MaxTokens: a.config.MaxTokens,
			Messages:  a.conversation.Messages(),
			Tools:     a.toolUnionParams,
		})
		cb.OnThinkingDone()
		if err != nil {
			return nil, err
		}

		// Update token count from API response
		if message.Usage.InputTokens > 0 {
			a.conversation.UpdateTokenCount(int(message.Usage.InputTokens))
		}

		// Append the assistant's response to conversation
		a.conversation.Append(message.ToParam())
		finalStopReason = string(message.StopReason)

		var toolUseBlocks []anthropic.ToolUseBlock
		var toolResults []anthropic.ContentBlockParamUnion
		hasToolUse := false

		// Collect text blocks and tool use blocks separately
		for _, block := range message.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				finalText = b.Text
				cb.OnText(b.Text)

			case anthropic.ToolUseBlock:
				hasToolUse = true
				toolUseBlocks = append(toolUseBlocks, b)

				// Notify callback of tool call with JSON input
				inputJSON, err := json.Marshal(b.Input)
				if err != nil {
					inputJSON = []byte("{}")
				}
				cb.OnToolCall(b.Name, string(inputJSON))
			}
		}

		// Execute tool blocks concurrently if any exist
		if len(toolUseBlocks) > 0 {
			toolResults = a.executeToolsConcurrently(ctx, toolUseBlocks, cb)
		}

		// If there were tool calls, add results to conversation and loop again
		if len(toolResults) > 0 {
			// Append tool results, with truncation applied if needed
			a.conversation.Append(anthropic.NewUserMessage(toolResults...))
		}

		// If no tool use, we're done
		if !hasToolUse {
			break
		}
	}

	return &Response{
		Text:       finalText,
		StopReason: finalStopReason,
	}, nil
}

// executeToolsConcurrently executes tool blocks concurrently,
// returning results in the original order.
func (a *Agent) executeToolsConcurrently(
	_ context.Context,
	blocks []anthropic.ToolUseBlock,
	cb Callbacks,
) []anthropic.ContentBlockParamUnion {
	// Determine concurrency limit
	maxConcurrent := a.config.MaxConcurrentTools
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	// Channel to limit concurrent goroutines
	semaphore := make(chan struct{}, maxConcurrent)
	resultsChan := make(chan toolResult, len(blocks))
	var wg sync.WaitGroup

	// Launch goroutines for each tool block
	for i, block := range blocks {
		wg.Go(func() {
			// Acquire semaphore slot
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := a.executeToolUse(block, cb)
			if result != nil {
				resultsChan <- toolResult{
					index:  i,
					result: *result,
				}
			}
		})
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	resultMap := make(map[int]anthropic.ContentBlockParamUnion)
	for tr := range resultsChan {
		resultMap[tr.index] = tr.result
	}

	// Sort results by original index
	indices := make([]int, 0, len(resultMap))
	for idx := range resultMap {
		indices = append(indices, idx)
	}
	slices.Sort(indices)

	// Build ordered result slice
	results := make([]anthropic.ContentBlockParamUnion, 0, len(resultMap))
	for _, idx := range indices {
		results = append(results, resultMap[idx])
	}

	return results
}

func makeToolUnionParams(tools []tool.Tool) []anthropic.ToolUnionParam {
	tup := make([]anthropic.ToolUnionParam, len(tools))
	for i := range tools {
		toolParam := tools[i].Param()
		tup[i] = anthropic.ToolUnionParam{OfTool: &toolParam}
	}

	return tup
}

func makeToolMap(tools []tool.Tool) map[string]tool.Tool {
	toolMap := make(map[string]tool.Tool)
	for i := range tools {
		t := tools[i]
		toolMap[t.Param().Name] = tools[i]
	}

	return toolMap
}

// executeToolUse calls a tool and notifies the callback of the result.
func (a *Agent) executeToolUse(block anthropic.ToolUseBlock, cb Callbacks) *anthropic.ContentBlockParamUnion {
	var result *anthropic.ContentBlockParamUnion

	t, exists := a.toolMap[block.Name]
	if !exists {
		// Tool not found — return error result
		result = new(anthropic.NewToolResultBlock(block.ID, "Tool not found", true))
	} else {
		result = t.Call(block)
	}

	// Extract output and error status from the result for callback
	if result != nil && result.OfToolResult != nil {
		isError := result.OfToolResult.IsError.Value
		output := ""
		if len(result.OfToolResult.Content) > 0 {
			if result.OfToolResult.Content[0].OfText != nil {
				output = result.OfToolResult.Content[0].OfText.Text
			}
		}
		cb.OnToolResult(block.Name, output, isError)
	}

	return result
}
