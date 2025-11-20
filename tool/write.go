// Package tool provides tool implementations for the agent.
package tool

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/anthropics/anthropic-sdk-go"
)

// WriteParams defines the parameters for the write tool.
type WriteParams struct {
	FilePath string `json:"filePath"` // Required: absolute path to file
	Content  string `json:"content"`  // Required: content to write
}

// Ensure WriteTool implements TypedTool[WriteParams]
var _ TypedTool[WriteParams] = (*WriteTool)(nil)

type WriteTool struct{}

// Call implements TypedTool.Call with strongly-typed parameters
func (t *WriteTool) Call(params WriteParams) (string, error) {
	// Ensure absolute path
	filePath := params.FilePath
	if !filepath.IsAbs(filePath) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getting current directory: %w", err)
		}
		filePath = filepath.Join(cwd, filePath)
	}

	// Check if file exists
	_, err := os.Stat(filePath)
	fileExists := !os.IsNotExist(err)

	// Write the file
	err = os.WriteFile(filePath, []byte(params.Content), 0644)
	if err != nil {
		return "", fmt.Errorf("writing file: %w", err)
	}

	if fileExists {
		return fmt.Sprintf("File overwritten: %s", filePath), nil
	}
	return fmt.Sprintf("File created: %s", filePath), nil
}

func (t *WriteTool) Param() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name: "write",
		Description: anthropic.String(`Writes a file to the local filesystem.

Usage:
- This tool will overwrite the existing file if there is one at the provided path.
- If this is an existing file, you MUST use the Read tool first to read the file's contents. This tool will fail if you did not read the file first.
- ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.
- NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User.
- Only use emojis if the user explicitly requests it. Avoid writing emojis to files unless asked.`),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				"filePath": map[string]interface{}{
					"type":        "string",
					"description": "The absolute path to the file to write (must be absolute, not relative)",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The content to write to the file",
				},
			},
			Required: []string{"filePath", "content"},
		},
	}
}
