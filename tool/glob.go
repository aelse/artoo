// Package tool provides tool implementations for the agent.
package tool

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

const globLimit = 100

// GlobParams defines the parameters for the glob tool.
type GlobParams struct {
	Pattern string  `json:"pattern"`        // Required: glob pattern
	Path    *string `json:"path,omitempty"` // Optional: directory to search
}

// fileWithTime stores a file path with its modification time
type fileWithTime struct {
	path  string
	mtime int64
}

// Ensure GlobTool implements TypedTool[GlobParams]
var _ TypedTool[GlobParams] = (*GlobTool)(nil)

type GlobTool struct{}

// Call implements TypedTool.Call with strongly-typed parameters
func (t *GlobTool) Call(params GlobParams) (string, error) {
	// Determine search path
	searchPath := "."
	if params.Path != nil && *params.Path != "" {
		searchPath = *params.Path
	}

	// Get absolute path
	if !filepath.IsAbs(searchPath) {
		absPath, err := filepath.Abs(searchPath)
		if err != nil {
			return "", fmt.Errorf("getting absolute path: %w", err)
		}
		searchPath = absPath
	}

	// Use ripgrep to find files matching pattern
	files, truncated, err := t.findFiles(searchPath, params.Pattern)
	if err != nil {
		return "", fmt.Errorf("finding files: %w", err)
	}

	// Sort by modification time (most recent first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].mtime > files[j].mtime
	})

	// Build output
	var output strings.Builder

	if len(files) == 0 {
		output.WriteString("No files found")
	} else {
		for _, f := range files {
			output.WriteString(f.path)
			output.WriteString("\n")
		}

		if truncated {
			output.WriteString("\n(Results are truncated. Consider using a more specific path or pattern.)\n")
		}
	}

	return output.String(), nil
}

// findFiles uses ripgrep to find files matching the glob pattern
func (t *GlobTool) findFiles(searchPath, pattern string) ([]fileWithTime, bool, error) {
	// Find ripgrep executable
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return nil, false, fmt.Errorf("ripgrep (rg) not found in PATH: %w", err)
	}

	// Build ripgrep arguments for listing files
	args := []string{
		"--files",
		"--glob", pattern,
	}

	// Execute ripgrep in the search path
	cmd := exec.Command(rgPath, args...)
	cmd.Dir = searchPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		// Check if it's just "no files found"
		if cmd.ProcessState.ExitCode() == 1 {
			return []fileWithTime{}, false, nil
		}
		return nil, false, fmt.Errorf("ripgrep failed: %s", stderr.String())
	}

	// Parse output - one file per line
	output := stdout.String()
	if output == "" {
		return []fileWithTime{}, false, nil
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var files []fileWithTime
	truncated := false

	for _, line := range lines {
		if line == "" {
			continue
		}

		if len(files) >= globLimit {
			truncated = true
			break
		}

		// Get full path and modification time
		fullPath := filepath.Join(searchPath, line)
		info, err := os.Stat(fullPath)
		var mtime int64
		if err == nil {
			mtime = info.ModTime().Unix()
		}

		files = append(files, fileWithTime{
			path:  fullPath,
			mtime: mtime,
		})
	}

	return files, truncated, nil
}

func (t *GlobTool) Param() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name: "glob",
		Description: anthropic.String(`- Fast file pattern matching tool that works with any codebase size
- Supports glob patterns like "**/*.js" or "src/**/*.ts"
- Returns matching file paths sorted by modification time
- Use this tool when you need to find files by name patterns
- When you are doing an open ended search that may require multiple rounds of globbing and grepping, use the Task tool instead
- You have the capability to call multiple tools in a single response. It is always better to speculatively perform multiple searches as a batch that are potentially useful.`),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "The glob pattern to match files against",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The directory to search in. If not specified, the current working directory will be used. IMPORTANT: Omit this field to use the default directory. DO NOT enter \"undefined\" or \"null\" - simply omit it for the default behavior. Must be a valid directory path if provided.",
				},
			},
			Required: []string{"pattern"},
		},
	}
}
