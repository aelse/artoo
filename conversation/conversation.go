// Package conversation provides conversation management functionality.
package conversation

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// Config holds conversation configuration.
type Config struct {
	MaxContextTokens int // e.g. 180_000 for Sonnet's 200k window, with headroom
	ToolResultMaxChars int // max chars for tool results before truncation (e.g. 10_000)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxContextTokens:   180_000, // Sonnet 200k window with 20k headroom
		ToolResultMaxChars: 10_000,  // Truncate tool results larger than 10k chars
	}
}

// Conversation manages the message history for an agent conversation
// with context window management.
type Conversation struct {
	messages         []anthropic.MessageParam
	config           Config
	totalInputTokens int // Updated from API response usage
}

// New creates a new empty Conversation with default config.
func New() *Conversation {
	return NewWithConfig(DefaultConfig())
}

// NewWithConfig creates a new Conversation with a custom config.
func NewWithConfig(config Config) *Conversation {
	return &Conversation{
		messages: make([]anthropic.MessageParam, 0),
		config:   config,
	}
}

// Append adds a message parameter to the conversation.
func (c *Conversation) Append(message anthropic.MessageParam) {
	c.messages = append(c.messages, message)
}

// AppendToolResult adds a tool result, truncating it if it exceeds the max character limit.
func (c *Conversation) AppendToolResult(result anthropic.ContentBlockParamUnion) {
	// Truncate large tool results before appending
	truncated := c.truncateToolResult(result)
	c.Append(anthropic.NewUserMessage(truncated))
}

// truncateToolResult checks if a tool result exceeds the character limit and truncates if needed.
func (c *Conversation) truncateToolResult(result anthropic.ContentBlockParamUnion) anthropic.ContentBlockParamUnion {
	if result.OfToolResult == nil {
		return result
	}

	toolResult := result.OfToolResult
	if len(toolResult.Content) == 0 {
		return result
	}

	// Extract text content
	textContent := toolResult.Content[0]
	if textContent.OfText == nil {
		return result
	}

	text := textContent.OfText.Text

	// Check if truncation is needed
	if len(text) > c.config.ToolResultMaxChars {
		truncated := text[:c.config.ToolResultMaxChars]
		truncated += fmt.Sprintf("\n\n(Output truncated from %d to %d characters)",
			len(text), c.config.ToolResultMaxChars)

		// Create a new tool result with truncated text
		newBlock := anthropic.NewToolResultBlock(
			toolResult.ToolUseID,
			truncated,
			toolResult.IsError.Value,
		)
		return newBlock
	}

	return result
}

// UpdateTokenCount updates the token count from an API response.
// This should be called after each API call with the response's InputTokens.
func (c *Conversation) UpdateTokenCount(inputTokens int) {
	c.totalInputTokens = inputTokens
}

// Trim removes old messages if token count approaches the limit.
// It preserves the system message (if present) and the most recent messages.
// Trimming happens when totalInputTokens exceeds 75% of MaxContextTokens.
func (c *Conversation) Trim() {
	if c.config.MaxContextTokens == 0 {
		return // No limit set
	}

	// Calculate trim threshold (75% of max)
	trimThreshold := (c.config.MaxContextTokens * 75) / 100

	if c.totalInputTokens <= trimThreshold {
		return // Not yet at threshold
	}

	// Keep system message (if present at index 0) and recent messages
	// Remove oldest user/assistant pairs from the front
	startIndex := 0

	// Check if first message is a system message (has a Role field that's "user" but only text)
	// For now, we'll just keep the first message as-is to preserve any system context
	if len(c.messages) > 0 {
		startIndex = 1 // Keep first message
	}

	// Remove messages until we're below the trim threshold
	// We'll do this greedily from the oldest (after system message)
	for len(c.messages) > startIndex+2 && c.totalInputTokens > trimThreshold {
		// Remove the oldest non-system message
		c.messages = append(c.messages[:startIndex], c.messages[startIndex+1:]...)
		// Rough estimate: each message pair is ~5-10% of typical load
		// This is approximate; exact token count comes from API responses
		c.totalInputTokens = (c.totalInputTokens * 90) / 100
	}
}

// MessageCount returns the number of messages in the conversation.
func (c *Conversation) MessageCount() int {
	return len(c.messages)
}

// EstimatedTokens returns the estimated input tokens used by the conversation.
// This is updated from actual API responses via UpdateTokenCount.
func (c *Conversation) EstimatedTokens() int {
	return c.totalInputTokens
}

// Messages returns the slice of messages for use with the Claude API.
// Callers should ensure Trim() has been called before this if context
// management is desired.
func (c *Conversation) Messages() []anthropic.MessageParam {
	return c.messages
}

// Len returns the number of messages in the conversation.
func (c *Conversation) Len() int {
	return len(c.messages)
}

// Get returns the message at the specified index.
func (c *Conversation) Get(index int) anthropic.MessageParam {
	return c.messages[index]
}
