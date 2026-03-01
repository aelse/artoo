// Package agent provides the core agent functionality for interacting with Claude.
package agent

import (
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

const (
	defaultMaxTokens          = 8192
	defaultMaxConcurrentTools = 4
)

// Config holds agent configuration.
type Config struct {
	Model               string        // e.g. "claude-sonnet-4-20250514"
	MaxTokens           int64         // per-response token limit
	MaxConcurrentTools  int           // maximum concurrent tool executions
	PluginDir           string        // Directory containing plugin executables
	PluginTimeout       time.Duration // Execution timeout per plugin call
	Streaming           bool          // Whether to use streaming API (default: true)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Model:              string(anthropic.ModelClaude4Sonnet20250514),
		MaxTokens:          defaultMaxTokens,
		MaxConcurrentTools: defaultMaxConcurrentTools,
		Streaming:          true,
	}
}

// Callbacks is implemented by the UI layer to observe agent events
// without the agent knowing about terminals or styling.
//
// When concurrent tool execution is active (MaxConcurrentTools > 1),
// callback methods may be called from multiple goroutines simultaneously.
// Implementations must be thread-safe.
type Callbacks interface {
	// OnThinking is called when the agent starts thinking (API call starting).
	OnThinking()

	// OnThinkingDone is called when the API response is received.
	OnThinkingDone()

	// OnText is called when the assistant produces text.
	OnText(text string)

	// OnTextDelta is called when a text delta is received (streaming only).
	OnTextDelta(delta string)

	// OnToolCall is called when the assistant calls a tool.
	// input is the JSON-marshaled parameters.
	OnToolCall(name string, input string)

	// OnToolResult is called after a tool completes.
	// May be called from multiple goroutines concurrently.
	OnToolResult(name string, output string, isError bool)
}

// Response is the final output from a SendMessage call.
type Response struct {
	Text       string // The assistant's text response
	StopReason string // Why the assistant stopped (e.g., "end_turn", "tool_use")
}
