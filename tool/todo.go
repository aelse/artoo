// Package tool provides tool implementations for the agent.
package tool

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
)

// TodoItem represents a single todo item.
type TodoItem struct {
	Content    string `json:"content"`    // Task description
	Status     string `json:"status"`     // pending, in_progress, completed, cancelled
	ActiveForm string `json:"activeForm"` // Present continuous form (e.g., "Running tests")
}

// TodoWriteParams defines the parameters for the todowrite tool.
type TodoWriteParams struct {
	Todos []TodoItem `json:"todos"` // Required: updated todo list
}

// TodoReadParams defines the parameters for the todoread tool (empty).
type TodoReadParams struct{}

// In-memory todo storage (simplified - in production would use session storage)
var (
	todoStore     = make(map[string][]TodoItem)
	todoStoreLock sync.RWMutex
)

// Ensure TodoWriteTool implements TypedTool[TodoWriteParams]
var _ TypedTool[TodoWriteParams] = (*TodoWriteTool)(nil)

// Ensure TodoReadTool implements TypedTool[TodoReadParams]
var _ TypedTool[TodoReadParams] = (*TodoReadTool)(nil)

type TodoWriteTool struct{}

type TodoReadTool struct{}

// Call implements TypedTool.Call for TodoWriteTool
func (t *TodoWriteTool) Call(params TodoWriteParams) (string, error) {
	// Validate todos
	for i, todo := range params.Todos {
		if todo.Content == "" {
			return "", fmt.Errorf("todo item %d has empty content", i)
		}
		if todo.Status != "pending" && todo.Status != "in_progress" &&
		   todo.Status != "completed" && todo.Status != "cancelled" {
			return "", fmt.Errorf("todo item %d has invalid status: %s", i, todo.Status)
		}
		if todo.ActiveForm == "" {
			return "", fmt.Errorf("todo item %d has empty activeForm", i)
		}
	}

	// Store todos (using "default" session for now)
	sessionID := "default"
	todoStoreLock.Lock()
	todoStore[sessionID] = params.Todos
	todoStoreLock.Unlock()

	// Count non-completed todos
	activeCount := 0
	for _, todo := range params.Todos {
		if todo.Status != "completed" {
			activeCount++
		}
	}

	// Return formatted output
	output, err := json.MarshalIndent(params.Todos, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling todos: %w", err)
	}

	return fmt.Sprintf("%d active todos\n\n%s", activeCount, string(output)), nil
}

// Call implements TypedTool.Call for TodoReadTool
func (t *TodoReadTool) Call(params TodoReadParams) (string, error) {
	// Read todos (using "default" session for now)
	sessionID := "default"
	todoStoreLock.RLock()
	todos, exists := todoStore[sessionID]
	todoStoreLock.RUnlock()

	if !exists || len(todos) == 0 {
		return "No todos found", nil
	}

	// Count non-completed todos
	activeCount := 0
	for _, todo := range todos {
		if todo.Status != "completed" {
			activeCount++
		}
	}

	// Return formatted output
	output, err := json.MarshalIndent(todos, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling todos: %w", err)
	}

	return fmt.Sprintf("%d active todos\n\n%s", activeCount, string(output)), nil
}

func (t *TodoWriteTool) Param() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name: "todowrite",
		Description: anthropic.String(`Use this tool to create and manage a structured task list for your current coding session. This helps you track progress, organize complex tasks, and demonstrate thoroughness to the user.
It also helps the user understand the progress of the task and overall progress of their requests.

## When to Use This Tool
Use this tool proactively in these scenarios:

1. Complex multi-step tasks - When a task requires 3 or more distinct steps or actions
2. Non-trivial and complex tasks - Tasks that require careful planning or multiple operations
3. User explicitly requests todo list - When the user directly asks you to use the todo list
4. User provides multiple tasks - When users provide a list of things to be done (numbered or comma-separated)
5. After receiving new instructions - Immediately capture user requirements as todos
6. When you start working on a task, mark the todo as in_progress. Ideally you should only have one todo as in_progress at a time.

## When NOT to Use This Tool

Skip using this tool when:
1. There is only a single, straightforward task
2. The task is trivial and tracking it provides no organizational benefit
3. The task can be completed in less than 3 trivial steps
4. The task is purely conversational or informational

## Task States and Management

1. **Task States**: Use these states to track progress:
   - pending: Task not yet started
   - in_progress: Currently working on (limit to ONE task at a time)
   - completed: Task finished successfully
   - cancelled: Task no longer needed

2. **Task Management**:
   - Update task status in real-time as you work
   - Mark tasks complete IMMEDIATELY after finishing (don't batch completions)
   - Only have ONE task in_progress at any time
   - Complete current tasks before starting new ones
   - Cancel tasks that become irrelevant

3. **Task Breakdown**:
   - Create specific, actionable items
   - Break complex tasks into smaller, manageable steps
   - Use clear, descriptive task names
   - Always provide both content and activeForm for each task

When in doubt, use this tool. Being proactive with task management demonstrates attentiveness and ensures you complete all requirements successfully.`),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				"todos": map[string]interface{}{
					"type":        "array",
					"description": "The updated todo list",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"content": map[string]interface{}{
								"type":        "string",
								"description": "Task description (imperative form, e.g., 'Run tests')",
							},
							"status": map[string]interface{}{
								"type":        "string",
								"description": "Task status",
								"enum":        []string{"pending", "in_progress", "completed", "cancelled"},
							},
							"activeForm": map[string]interface{}{
								"type":        "string",
								"description": "Present continuous form shown during execution (e.g., 'Running tests')",
							},
						},
						"required": []string{"content", "status", "activeForm"},
					},
				},
			},
			Required: []string{"todos"},
		},
	}
}

func (t *TodoReadTool) Param() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name: "todoread",
		Description: anthropic.String(`Use this tool to read the current to-do list for the session. This tool should be used proactively and frequently to ensure that you are aware of
the status of the current task list. You should make use of this tool as often as possible, especially in the following situations:
- At the beginning of conversations to see what's pending
- Before starting new tasks to prioritize work
- When the user asks about previous tasks or plans
- Whenever you're uncertain about what to do next
- After completing tasks to update your understanding of remaining work
- After every few messages to ensure you're on track

Usage:
- This tool takes in no parameters. So leave the input blank or empty. DO NOT include a dummy object, placeholder string or a key like "input" or "empty". LEAVE IT BLANK.
- Returns a list of todo items with their status, priority, and content
- Use this information to track progress and plan next steps
- If no todos exist yet, an empty list will be returned`),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{},
		},
	}
}
