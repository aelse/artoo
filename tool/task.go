// Package tool provides tool implementations for the agent.
package tool

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// TaskParams defines the parameters for the task tool.
type TaskParams struct {
	Description  string `json:"description"`   // Required: short task description
	Prompt       string `json:"prompt"`        // Required: task for agent
	SubagentType string `json:"subagent_type"` // Required: agent type
}

// Ensure TaskTool implements TypedTool[TaskParams]
var _ TypedTool[TaskParams] = (*TaskTool)(nil)

type TaskTool struct{}

// Call implements TypedTool.Call with strongly-typed parameters
func (t *TaskTool) Call(params TaskParams) (string, error) {
	// Task tool requires agent orchestration infrastructure
	// This is a stub implementation
	return "", fmt.Errorf("Task tool requires agent orchestration framework (not yet implemented in Go version)")
}

func (t *TaskTool) Param() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name: "task",
		Description: anthropic.String(`Launch a new agent to handle complex, multi-step tasks autonomously.

Available agent types and the tools they have access to:
- general-purpose: General-purpose agent for researching complex questions, searching for code, and executing multi-step tasks
- Explore: Fast agent specialized for exploring codebases

When using the Task tool, you must specify a subagent_type parameter to select which agent type to use.

When NOT to use the Task tool:
- If you want to read a specific file path, use the Read or Glob tool instead of the Task tool, to find the match more quickly
- If you are searching for a specific class definition like "class Foo", use the Glob tool instead, to find the match more quickly
- If you are searching for code within a specific file or set of 2-3 files, use the Read tool instead of the Task tool, to find the match more quickly
- Other tasks that are not related to the agent descriptions above

Usage notes:
1. Launch multiple agents concurrently whenever possible, to maximize performance; to do that, use a single message with multiple tool uses
2. When the agent is done, it will return a single message back to you. The result returned by the agent is not visible to the user.
3. Each agent invocation is stateless. You will not be able to send additional messages to the agent.
4. The agent's outputs should generally be trusted
5. Clearly tell the agent whether you expect it to write code or just to do research`),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				"description": map[string]interface{}{
					"type":        "string",
					"description": "A short (3-5 words) description of the task",
				},
				"prompt": map[string]interface{}{
					"type":        "string",
					"description": "The task for the agent to perform",
				},
				"subagent_type": map[string]interface{}{
					"type":        "string",
					"description": "The type of specialized agent to use for this task",
				},
			},
			Required: []string{"description", "prompt", "subagent_type"},
		},
	}
}
