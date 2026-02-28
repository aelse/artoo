// Package agent provides the core agent functionality for interacting with Claude.
package agent

import (
	"context"
	"encoding/json"

	"github.com/aelse/artoo/conversation"
	"github.com/aelse/artoo/tool"
	"github.com/anthropics/anthropic-sdk-go"
)

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
func New(client anthropic.Client, config Config) *Agent {
	allTools := tool.AllTools
	return &Agent{
		client:          client,
		conversation:    conversation.New(),
		tools:           allTools,
		toolMap:         makeToolMap(allTools),
		toolUnionParams: makeToolUnionParams(allTools),
		config:          config,
	}
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

		// Append the assistant's response to conversation
		a.conversation.Append(message.ToParam())
		finalStopReason = string(message.StopReason)

		var toolResults []anthropic.ContentBlockParamUnion
		hasToolUse := false

		// Process response content (text + tool use blocks)
		for _, block := range message.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				finalText = b.Text
				cb.OnText(b.Text)

			case anthropic.ToolUseBlock:
				hasToolUse = true

				// Notify callback of tool call with JSON input
				inputJSON, _ := json.Marshal(b.Input)
				cb.OnToolCall(b.Name, string(inputJSON))

				// Execute the tool
				result := a.executeToolUse(b, cb)
				if result != nil {
					toolResults = append(toolResults, *result)
				}
			}
		}

		// If there were tool calls, add results to conversation and loop again
		if len(toolResults) > 0 {
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
	t, exists := a.toolMap[block.Name]
	if !exists {
		// Tool not found — return error result
		return new(anthropic.NewToolResultBlock(block.ID, "Tool not found", true))
	}

	result := t.Call(block)

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

