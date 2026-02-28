// Package tool provides tool implementations for the agent.
package tool

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// Default patterns to ignore when listing files
var ignorePatterns = []string{
	"node_modules/",
	"__pycache__/",
	".git/",
	"dist/",
	"build/",
	"target/",
	"vendor/",
	"bin/",
	"obj/",
	".idea/",
	".vscode/",
	".zig-cache/",
	"zig-out",
	".coverage",
	"coverage/",
	"tmp/",
	"temp/",
	".cache/",
	"cache/",
	"logs/",
	".venv/",
	"venv/",
	"env/",
}

const lsLimit = 100

// LsParams defines the parameters for the ls tool.
type LsParams struct {
	Path   *string  `json:"path,omitempty"`   // Optional absolute path to list
	Ignore []string `json:"ignore,omitempty"` // Optional glob patterns to ignore
}

// Ensure LsTool implements TypedTool[LsParams]
var _ TypedTool[LsParams] = (*LsTool)(nil)

type LsTool struct{}

// Call implements TypedTool.Call with strongly-typed parameters
func (t *LsTool) Call(params LsParams) (string, error) {
	// Determine search path
	searchPath := "."
	if params.Path != nil && *params.Path != "" {
		searchPath = *params.Path
	}

	// Get absolute path
	absPath, err := filepath.Abs(searchPath)
	if err != nil {
		return "", fmt.Errorf("getting absolute path: %w", err)
	}

	// Build ignore globs
	ignoreGlobs := make([]string, 0, len(ignorePatterns)+len(params.Ignore))
	for _, pattern := range ignorePatterns {
		ignoreGlobs = append(ignoreGlobs, "!"+pattern+"*")
	}
	for _, pattern := range params.Ignore {
		ignoreGlobs = append(ignoreGlobs, "!"+pattern)
	}

	// Get files using ripgrep
	files, err := t.getFiles(absPath, ignoreGlobs)
	if err != nil {
		return "", fmt.Errorf("listing files: %w", err)
	}

	// Limit results
	truncated := len(files) > lsLimit
	if truncated {
		files = files[:lsLimit]
	}

	// Build and render directory tree
	output := t.renderTree(absPath, files, truncated)
	return output, nil
}

// getFiles uses ripgrep to list files with ignore patterns
func (t *LsTool) getFiles(searchPath string, ignoreGlobs []string) ([]string, error) {
	// Find ripgrep executable
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return nil, fmt.Errorf("ripgrep (rg) not found in PATH: %w", err)
	}

	// Build ripgrep arguments for listing files
	args := []string{"--files"}
	for _, glob := range ignoreGlobs {
		args = append(args, "--glob", glob)
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
			return []string{}, nil
		}
		return nil, fmt.Errorf("ripgrep failed: %s", stderr.String())
	}

	// Parse output - one file per line
	output := stdout.String()
	if output == "" {
		return []string{}, nil
	}

	files := strings.Split(strings.TrimSpace(output), "\n")
	return files, nil
}

// renderTree builds a tree structure representation of files
func (t *LsTool) renderTree(basePath string, files []string, truncated bool) string {
	// Build directory structure
	dirs := make(map[string]bool)
	filesByDir := make(map[string][]string)

	dirs["."] = true

	for _, file := range files {
		dir := filepath.Dir(file)
		if dir == "" {
			dir = "."
		}

		// Add all parent directories
		parts := []string{}
		if dir != "." {
			parts = strings.Split(dir, string(filepath.Separator))
		}

		for i := 0; i <= len(parts); i++ {
			var dirPath string
			if i == 0 {
				dirPath = "."
			} else {
				dirPath = filepath.Join(parts[:i]...)
			}
			dirs[dirPath] = true
		}

		// Add file to its directory
		if filesByDir[dir] == nil {
			filesByDir[dir] = []string{}
		}
		filesByDir[dir] = append(filesByDir[dir], filepath.Base(file))
	}

	// Render the tree
	var output strings.Builder
	output.WriteString(basePath + "/\n")

	// Helper function to render a directory recursively
	var renderDir func(dirPath string, depth int) string
	renderDir = func(dirPath string, depth int) string {
		var result strings.Builder
		indent := strings.Repeat("  ", depth)

		// Render directory name (except for root ".")
		if depth > 0 {
			result.WriteString(fmt.Sprintf("%s%s/\n", indent, filepath.Base(dirPath)))
		}

		childIndent := strings.Repeat("  ", depth+1)

		// Find and sort child directories
		var children []string
		for d := range dirs {
			if filepath.Dir(d) == dirPath && d != dirPath {
				children = append(children, d)
			}
		}
		slices.Sort(children)

		// Render subdirectories first
		for _, child := range children {
			result.WriteString(renderDir(child, depth+1))
		}

		// Render files in this directory
		if files := filesByDir[dirPath]; len(files) > 0 {
			slices.Sort(files)
			for _, file := range files {
				result.WriteString(fmt.Sprintf("%s%s\n", childIndent, file))
			}
		}

		return result.String()
	}

	output.WriteString(renderDir(".", 0))

	if truncated {
		output.WriteString(fmt.Sprintf("\n(Showing first %d files. Results truncated.)\n", lsLimit))
	}

	return output.String()
}

func (t *LsTool) Param() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name:        "list",
		Description: anthropic.String("Lists files and directories in a given path. The path parameter must be absolute; omit it to use the current workspace directory. You can optionally provide an array of glob patterns to ignore with the ignore parameter. You should generally prefer the Glob and Grep tools, if you know which directories to search."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "The absolute path to the directory to list (must be absolute, not relative)",
				},
				"ignore": map[string]any{
					"type":        "array",
					"description": "List of glob patterns to ignore",
					"items": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}
}
