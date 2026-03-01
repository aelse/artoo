// Package tool provides tool implementations for the agent.
package tool

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// GrepParams defines the parameters for the grep tool.
type GrepParams struct {
	Pattern string  `json:"pattern"`           // The regex pattern to search for
	Path    *string `json:"path,omitempty"`    // Optional directory to search in
	Include *string `json:"include,omitempty"` // Optional file pattern to include
}

// Number of fields in ripgrep output format: filepath|lineNum|lineText.
const grepOutputFieldCount = 3

// grepMatch represents a single match from ripgrep.
type grepMatch struct {
	path     string
	modTime  int64
	lineNum  int
	lineText string
}

// Ensure GrepTool implements TypedTool[GrepParams].
var _ TypedTool[GrepParams] = (*GrepTool)(nil)

type GrepTool struct{}

// Call implements TypedTool.Call with strongly-typed parameters.
func (t *GrepTool) Call(params GrepParams) (string, error) {
	if params.Pattern == "" {
		return "", errors.New("pattern is required")
	}

	// Determine search path
	searchPath := "."
	if params.Path != nil && *params.Path != "" {
		searchPath = *params.Path
	}

	// Find ripgrep executable
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return "", fmt.Errorf("ripgrep (rg) not found in PATH: %w", err)
	}

	// Build ripgrep arguments
	args := []string{
		"-nH",                       // Show line numbers and filenames
		"--field-match-separator=|", // Use | as separator
		params.Pattern,
	}

	if params.Include != nil && *params.Include != "" {
		args = append(args, "--glob", *params.Include)
	}

	args = append(args, searchPath)

	// Execute ripgrep
	cmd := exec.CommandContext(context.Background(), rgPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run()
	exitCode := cmd.ProcessState.ExitCode()

	// Exit code 1 means no matches found
	if exitCode == 1 {
		return "No files found", nil
	}

	// Other non-zero exit codes are errors
	if exitCode != 0 {
		return "", fmt.Errorf("ripgrep failed: %s", stderr.String())
	}

	// Parse output
	matches, err := t.parseRipgrepOutput(stdout.String())
	if err != nil {
		return "", fmt.Errorf("parsing ripgrep output: %w", err)
	}

	if len(matches) == 0 {
		return "No files found", nil
	}

	// Sort matches by modification time (most recent first)
	slices.SortFunc(matches, func(a, b grepMatch) int {
		return cmp.Compare(b.modTime, a.modTime)
	})

	// Limit and truncate results
	limit := 100
	truncated := len(matches) > limit
	if truncated {
		matches = matches[:limit]
	}

	// Format output
	return t.formatOutput(params.Pattern, matches, truncated), nil
}

// parseRipgrepOutput parses the output from ripgrep into matches.
func (t *GrepTool) parseRipgrepOutput(output string) ([]grepMatch, error) {
	var matches []grepMatch

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse format: filepath|lineNum|lineText
		parts := strings.SplitN(line, "|", grepOutputFieldCount)
		if len(parts) < grepOutputFieldCount {
			continue
		}

		filePath := parts[0]
		lineNumStr := parts[1]
		lineText := parts[2]

		lineNum, err := strconv.Atoi(lineNumStr)
		if err != nil {
			continue
		}

		// Get file modification time
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		matches = append(matches, grepMatch{
			path:     filePath,
			modTime:  info.ModTime().Unix(),
			lineNum:  lineNum,
			lineText: lineText,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return matches, nil
}

// formatOutput formats the matches into a human-readable output.
func (t *GrepTool) formatOutput(_ string, matches []grepMatch, truncated bool) string {
	var output strings.Builder

	fmt.Fprintf(&output, "Found %d matches\n", len(matches))

	currentFile := ""
	for _, match := range matches {
		if currentFile != match.path {
			if currentFile != "" {
				output.WriteString("\n")
			}

			currentFile = match.path
			output.WriteString(match.path + ":\n")
		}

		fmt.Fprintf(&output, "  Line %d: %s\n", match.lineNum, match.lineText)
	}

	if truncated {
		output.WriteString("\n(Results are truncated. Consider using a more specific path or pattern.)\n")
	}

	return output.String()
}

func (t *GrepTool) Param() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name: "grep",
		Description: anthropic.String(`- Fast content search tool that works with any codebase size
- Searches file contents using regular expressions
- Supports full regex syntax (eg. "log.*Error", "function\s+\w+", etc.)
- Filter files by pattern with the include parameter (eg. "*.js", "*.{ts,tsx}")
- Returns file paths with at least one match sorted by modification time
- Use this tool when you need to find files containing specific patterns
- If you need to identify/count the number of matches within files, use the Bash tool with 'rg' (ripgrep) directly. Do NOT use 'grep'.
- When you are doing an open ended search that may require multiple rounds of globbing and grepping, use the Task tool instead`),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "The regex pattern to search for in file contents",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "The directory to search in. Defaults to the current working directory.",
				},
				"include": map[string]any{
					"type":        "string",
					"description": "File pattern to include in the search (e.g. \"*.js\", \"*.{ts,tsx}\")",
				},
			},
			Required: []string{"pattern"},
		},
	}
}
