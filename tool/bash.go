// Package tool provides tool implementations for the agent.
package tool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

const (
	maxOutputLength  = 30000
	defaultTimeout   = 2 * time.Minute
	maxTimeout       = 10 * time.Minute
)

// BashParams defines the parameters for the bash tool.
type BashParams struct {
	Command     string `json:"command"`               // Required: command to execute
	Timeout     *int   `json:"timeout,omitempty"`     // Optional: timeout in milliseconds
	Description string `json:"description,omitempty"` // Optional: description of command
}

// Ensure BashTool implements TypedTool[BashParams]
var _ TypedTool[BashParams] = (*BashTool)(nil)

type BashTool struct{}

// Call implements TypedTool.Call with strongly-typed parameters
func (t *BashTool) Call(params BashParams) (string, error) {
	// Determine timeout
	timeout := defaultTimeout
	if params.Timeout != nil {
		requestedTimeout := time.Duration(*params.Timeout) * time.Millisecond
		if requestedTimeout > maxTimeout {
			timeout = maxTimeout
		} else {
			timeout = requestedTimeout
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Execute command
	cmd := exec.CommandContext(ctx, "bash", "-c", params.Command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Combine stdout and stderr
	output := stdout.String() + stderr.String()

	// Check for timeout
	timedOut := false
	if ctx.Err() == context.DeadlineExceeded {
		timedOut = true
	}

	// Truncate if too long
	if len(output) > maxOutputLength {
		output = output[:maxOutputLength]
		output += "\n\n(Output was truncated due to length limit)"
	}

	if timedOut {
		output += fmt.Sprintf("\n\n(Command timed out after %v)", timeout)
	}

	// Return output even if command failed
	if err != nil && !timedOut {
		exitError, ok := err.(*exec.ExitError)
		if ok {
			output += fmt.Sprintf("\n\n(Command exited with code %d)", exitError.ExitCode())
		} else {
			return "", fmt.Errorf("executing command: %w", err)
		}
	}

	return output, nil
}

func (t *BashTool) Param() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name: "bash",
		Description: anthropic.String(`Executes a given bash command in a persistent shell session with optional timeout, ensuring proper handling and security measures.

IMPORTANT: This tool is for terminal operations like git, npm, docker, etc. DO NOT use it for file operations (reading, writing, editing, searching, finding files) - use the specialized tools for this instead.

Before executing the command, please follow these steps:

1. Directory Verification:
   - If the command will create new directories or files, first use ls to verify the parent directory exists and is the correct location
   - For example, before running "mkdir foo/bar", first use ls foo to check that "foo" exists and is the intended parent directory

2. Command Execution:
   - Always quote file paths that contain spaces with double quotes (e.g., cd "path with spaces/file.txt")
   - After ensuring proper quoting, execute the command.
   - Capture the output of the command.

Usage notes:
  - The command argument is required.
  - You can specify an optional timeout in milliseconds (up to 600000ms / 10 minutes). If not specified, commands will timeout after 120000ms (2 minutes).
  - It is very helpful if you write a clear, concise description of what this command does in 5-10 words.
  - If the output exceeds 30000 characters, output will be truncated before being returned to you.
  - Avoid using Bash with the find, grep, cat, head, tail, sed, awk, or echo commands, unless explicitly instructed or when these commands are truly necessary for the task. Instead, always prefer using the dedicated tools for these commands:
    - File search: Use Glob (NOT find or ls)
    - Content search: Use Grep (NOT grep or rg)
    - Read files: Use Read (NOT cat/head/tail)
    - Edit files: Use Edit (NOT sed/awk)
    - Write files: Use Write (NOT echo >/cat <<EOF)
  - When issuing multiple commands:
    - If the commands are independent and can run in parallel, make multiple Bash tool calls in a single message.
    - If the commands depend on each other and must run sequentially, use a single Bash call with '&&' to chain them together (e.g., git add . && git commit -m "message" && git push).
    - Use ';' only when you need to run commands sequentially but don't care if earlier commands fail
    - DO NOT use newlines to separate commands (newlines are ok in quoted strings)
  - Try to maintain your current working directory throughout the session by using absolute paths and avoiding usage of cd. You may use cd if the User explicitly requests it.`),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The command to execute",
				},
				"timeout": map[string]interface{}{
					"type":        "integer",
					"description": "Optional timeout in milliseconds (max 600000)",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Clear, concise description of what this command does in 5-10 words, in active voice. Examples:\nInput: ls\nOutput: List files in current directory\n\nInput: git status\nOutput: Show working tree status\n\nInput: npm install\nOutput: Install package dependencies\n\nInput: mkdir foo\nOutput: Create directory 'foo'",
				},
			},
			Required: []string{"command"},
		},
	}
}
