// Package tool provides tool implementations for the agent.
package tool

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

const (
	defaultReadLimit = 2000
	maxLineLength    = 2000
)

// Binary file extensions that should not be read as text
var binaryExtensions = map[string]bool{
	".zip":  true,
	".tar":  true,
	".gz":   true,
	".exe":  true,
	".dll":  true,
	".so":   true,
	".class": true,
	".jar":  true,
	".war":  true,
	".7z":   true,
	".doc":  true,
	".docx": true,
	".xls":  true,
	".xlsx": true,
	".ppt":  true,
	".pptx": true,
	".odt":  true,
	".ods":  true,
	".odp":  true,
	".bin":  true,
	".dat":  true,
	".obj":  true,
	".o":    true,
	".a":    true,
	".lib":  true,
	".wasm": true,
	".pyc":  true,
	".pyo":  true,
}

// Image file extensions
var imageExtensions = map[string]string{
	".jpg":  "JPEG",
	".jpeg": "JPEG",
	".png":  "PNG",
	".gif":  "GIF",
	".bmp":  "BMP",
	".webp": "WebP",
}

// ReadParams defines the parameters for the read tool.
type ReadParams struct {
	FilePath string `json:"filePath"`        // Required: path to file
	Offset   *int   `json:"offset,omitempty"` // Optional: line number to start (0-based)
	Limit    *int   `json:"limit,omitempty"`  // Optional: number of lines to read
}

// Ensure ReadTool implements TypedTool[ReadParams]
var _ TypedTool[ReadParams] = (*ReadTool)(nil)

type ReadTool struct{}

// Call implements TypedTool.Call with strongly-typed parameters
func (t *ReadTool) Call(params ReadParams) (string, error) {
	// Ensure absolute path
	filePath := params.FilePath
	if !filepath.IsAbs(filePath) {
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return "", fmt.Errorf("getting absolute path: %w", err)
		}
		filePath = absPath
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Try to provide helpful suggestions
		suggestions := t.findSimilarFiles(filePath)
		if len(suggestions) > 0 {
			return "", fmt.Errorf("File not found: %s\n\nDid you mean one of these?\n%s",
				filePath, strings.Join(suggestions, "\n"))
		}
		return "", fmt.Errorf("File not found: %s", filePath)
	}

	// Check if it's an image (we'll return a simplified message for now)
	if imageType := t.isImageFile(filePath); imageType != "" {
		return fmt.Sprintf("Image file detected (%s): %s\n\nNote: Image viewing not yet implemented in this tool.", imageType, filePath), nil
	}

	// Check if it's binary
	isBinary, err := t.isBinaryFile(filePath)
	if err != nil {
		return "", fmt.Errorf("checking if file is binary: %w", err)
	}
	if isBinary {
		return "", fmt.Errorf("Cannot read binary file: %s", filePath)
	}

	// Read and format the file
	output, err := t.readAndFormatFile(filePath, params.Offset, params.Limit)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}

	return output, nil
}

// findSimilarFiles looks for similar filenames in the same directory
func (t *ReadTool) findSimilarFiles(filePath string) []string {
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)
	baseLower := strings.ToLower(base)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var suggestions []string
	for _, entry := range entries {
		name := entry.Name()
		nameLower := strings.ToLower(name)
		if strings.Contains(nameLower, baseLower) || strings.Contains(baseLower, nameLower) {
			suggestions = append(suggestions, filepath.Join(dir, name))
			if len(suggestions) >= 3 {
				break
			}
		}
	}

	return suggestions
}

// isImageFile checks if the file is an image based on extension
func (t *ReadTool) isImageFile(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	if imageType, ok := imageExtensions[ext]; ok {
		return imageType
	}
	return ""
}

// isBinaryFile checks if a file is binary
func (t *ReadTool) isBinaryFile(filePath string) (bool, error) {
	// First check extension
	ext := strings.ToLower(filepath.Ext(filePath))
	if binaryExtensions[ext] {
		return true, nil
	}

	// Check file size
	info, err := os.Stat(filePath)
	if err != nil {
		return false, err
	}
	if info.Size() == 0 {
		return false, nil
	}

	// Read first 4KB and check for binary content
	file, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	bufferSize := 4096
	if info.Size() < int64(bufferSize) {
		bufferSize = int(info.Size())
	}

	buffer := make([]byte, bufferSize)
	n, err := file.Read(buffer)
	if err != nil {
		return false, err
	}
	buffer = buffer[:n]

	// Check for null bytes or high percentage of non-printable characters
	nonPrintableCount := 0
	for _, b := range buffer {
		// Null byte means binary
		if b == 0 {
			return true, nil
		}
		// Count non-printable characters (excluding tab, newline, carriage return)
		if b < 9 || (b > 13 && b < 32) {
			nonPrintableCount++
		}
	}

	// If >30% non-printable characters, consider it binary
	return float64(nonPrintableCount)/float64(len(buffer)) > 0.3, nil
}

// readAndFormatFile reads a file with offset/limit and formats with line numbers
func (t *ReadTool) readAndFormatFile(filePath string, offset, limit *int) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Determine offset and limit
	startLine := 0
	if offset != nil {
		startLine = *offset
	}

	lineLimit := defaultReadLimit
	if limit != nil {
		lineLimit = *limit
	}

	// Read all lines
	var allLines []string
	scanner := bufio.NewScanner(file)

	// Increase buffer size for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024) // 1MB max line length

	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scanning file: %w", err)
	}

	// Check if file is empty
	if len(allLines) == 0 {
		return "<file>\n(File is empty)\n</file>", nil
	}

	// Apply offset and limit
	endLine := startLine + lineLimit
	if endLine > len(allLines) {
		endLine = len(allLines)
	}

	if startLine >= len(allLines) {
		return "", fmt.Errorf("offset %d is beyond file length (%d lines)", startLine, len(allLines))
	}

	selectedLines := allLines[startLine:endLine]

	// Truncate long lines and format with line numbers
	var formattedLines []string
	for i, line := range selectedLines {
		// Truncate if needed
		if len(line) > maxLineLength {
			line = line[:maxLineLength] + "..."
		}

		// Format: 00001| content
		lineNum := startLine + i + 1
		formatted := fmt.Sprintf("%05d| %s", lineNum, line)
		formattedLines = append(formattedLines, formatted)
	}

	// Build output
	var output bytes.Buffer
	output.WriteString("<file>\n")
	output.WriteString(strings.Join(formattedLines, "\n"))

	// Add continuation message if there are more lines
	if endLine < len(allLines) {
		output.WriteString(fmt.Sprintf("\n\n(File has more lines. Use 'offset' parameter to read beyond line %d)", endLine))
	}

	output.WriteString("\n</file>")

	return output.String(), nil
}

func (t *ReadTool) Param() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name:        "read",
		Description: anthropic.String("Reads a file from the local filesystem. You can access any file directly by using this tool. The filePath parameter must be an absolute path, not a relative path. By default, it reads up to 2000 lines starting from the beginning of the file. You can optionally specify a line offset and limit (especially handy for long files), but it's recommended to read the whole file by not providing these parameters. Any lines longer than 2000 characters will be truncated. Results are returned using cat -n format, with line numbers starting at 1."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				"filePath": map[string]interface{}{
					"type":        "string",
					"description": "The absolute path to the file to read (must be absolute, not relative)",
				},
				"offset": map[string]interface{}{
					"type":        "integer",
					"description": "The line number to start reading from (0-based). Only provide if the file is too large to read at once",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "The number of lines to read (defaults to 2000). Only provide if the file is too large to read at once",
				},
			},
			Required: []string{"filePath"},
		},
	}
}
