// Package tool provides tool implementations for the agent.
package tool

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// Similarity thresholds for block anchor fallback matching
const (
	singleCandidateSimilarityThreshold   = 0.0
	multipleCandidatesSimilarityThreshold = 0.3
)

// EditParams defines the parameters for the edit tool.
type EditParams struct {
	FilePath   string `json:"filePath"`             // Required: path to file
	OldString  string `json:"oldString"`            // Required: text to replace
	NewString  string `json:"newString"`            // Required: replacement text
	ReplaceAll *bool  `json:"replaceAll,omitempty"` // Optional: replace all occurrences
}

// Ensure EditTool implements TypedTool[EditParams]
var _ TypedTool[EditParams] = (*EditTool)(nil)

type EditTool struct{}

// Call implements TypedTool.Call with strongly-typed parameters
func (t *EditTool) Call(params EditParams) (string, error) {
	if params.OldString == params.NewString {
		return "", fmt.Errorf("oldString and newString must be different")
	}

	// Ensure absolute path
	filePath := params.FilePath
	if !strings.HasPrefix(filePath, "/") {
		absPath, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("getting current directory: %w", err)
		}
		filePath = absPath + "/" + filePath
	}

	// Check if file exists
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("File %s not found", filePath)
	}
	if err != nil {
		return "", fmt.Errorf("checking file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("Path is a directory, not a file: %s", filePath)
	}

	// Read file content
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading file: %w", err)
	}
	contentOld := string(contentBytes)

	// Handle empty oldString (write new file)
	if params.OldString == "" {
		err = os.WriteFile(filePath, []byte(params.NewString), info.Mode())
		if err != nil {
			return "", fmt.Errorf("writing file: %w", err)
		}
		return fmt.Sprintf("File written: %s", filePath), nil
	}

	// Perform replacement
	replaceAll := false
	if params.ReplaceAll != nil {
		replaceAll = *params.ReplaceAll
	}

	contentNew, err := replace(contentOld, params.OldString, params.NewString, replaceAll)
	if err != nil {
		return "", err
	}

	// Write the updated content
	err = os.WriteFile(filePath, []byte(contentNew), info.Mode())
	if err != nil {
		return "", fmt.Errorf("writing file: %w", err)
	}

	return fmt.Sprintf("File edited successfully: %s", filePath), nil
}

func (t *EditTool) Param() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name: "edit",
		Description: anthropic.String(`Performs exact string replacements in files.

Usage:
- You must use your Read tool at least once in the conversation before editing. This tool will error if you attempt an edit without reading the file.
- When editing text from Read tool output, ensure you preserve the exact indentation (tabs/spaces) as it appears AFTER the line number prefix. The line number prefix format is: spaces + line number + tab. Everything after that tab is the actual file content to match. Never include any part of the line number prefix in the oldString or newString.
- ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.
- The edit will FAIL if oldString is not found in the file.
- The edit will FAIL if oldString is found multiple times in the file. Either provide a larger string with more surrounding context to make it unique or use replaceAll to change every instance of oldString.
- Use replaceAll for replacing and renaming strings across the file. This parameter is useful if you want to rename a variable for instance.`),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				"filePath": map[string]interface{}{
					"type":        "string",
					"description": "The absolute path to the file to modify",
				},
				"oldString": map[string]interface{}{
					"type":        "string",
					"description": "The text to replace",
				},
				"newString": map[string]interface{}{
					"type":        "string",
					"description": "The text to replace it with (must be different from oldString)",
				},
				"replaceAll": map[string]interface{}{
					"type":        "boolean",
					"description": "Replace all occurrences of oldString (default false)",
				},
			},
			Required: []string{"filePath", "oldString", "newString"},
		},
	}
}

// Replacer is a function that yields potential matches for the find string
type replacer func(content, find string) []string

// replace performs fuzzy string replacement using multiple strategies
func replace(content, oldString, newString string, replaceAll bool) (string, error) {
	if oldString == newString {
		return "", fmt.Errorf("oldString and newString must be different")
	}

	replacers := []replacer{
		simpleReplacer,
		lineTrimmedReplacer,
		blockAnchorReplacer,
		whitespaceNormalizedReplacer,
		indentationFlexibleReplacer,
		trimmedBoundaryReplacer,
	}

	var notFound = true

	for _, replacerFunc := range replacers {
		matches := replacerFunc(content, oldString)

		for _, search := range matches {
			index := strings.Index(content, search)
			if index == -1 {
				continue
			}

			notFound = false

			if replaceAll {
				return strings.ReplaceAll(content, search, newString), nil
			}

			// Check if there are multiple occurrences
			lastIndex := strings.LastIndex(content, search)
			if index != lastIndex {
				continue // Multiple occurrences, try next match
			}

			// Single occurrence, replace it
			return content[:index] + newString + content[index+len(search):], nil
		}
	}

	if notFound {
		return "", fmt.Errorf("oldString not found in content")
	}

	return "", fmt.Errorf("oldString found multiple times and requires more code context to uniquely identify the intended match")
}

// simpleReplacer tries exact string match
func simpleReplacer(content, find string) []string {
	if strings.Contains(content, find) {
		return []string{find}
	}
	return nil
}

// lineTrimmedReplacer matches lines with trimmed whitespace
func lineTrimmedReplacer(content, find string) []string {
	originalLines := strings.Split(content, "\n")
	searchLines := strings.Split(find, "\n")

	// Remove trailing empty line
	if len(searchLines) > 0 && searchLines[len(searchLines)-1] == "" {
		searchLines = searchLines[:len(searchLines)-1]
	}

	var results []string

	for i := 0; i <= len(originalLines)-len(searchLines); i++ {
		matches := true

		for j := 0; j < len(searchLines); j++ {
			originalTrimmed := strings.TrimSpace(originalLines[i+j])
			searchTrimmed := strings.TrimSpace(searchLines[j])

			if originalTrimmed != searchTrimmed {
				matches = false
				break
			}
		}

		if matches {
			// Calculate character indices
			matchStartIndex := 0
			for k := 0; k < i; k++ {
				matchStartIndex += len(originalLines[k]) + 1
			}

			matchEndIndex := matchStartIndex
			for k := 0; k < len(searchLines); k++ {
				matchEndIndex += len(originalLines[i+k])
				if k < len(searchLines)-1 {
					matchEndIndex++ // Add newline
				}
			}

			results = append(results, content[matchStartIndex:matchEndIndex])
		}
	}

	return results
}

// blockAnchorReplacer uses first and last lines as anchors with fuzzy middle matching
func blockAnchorReplacer(content, find string) []string {
	originalLines := strings.Split(content, "\n")
	searchLines := strings.Split(find, "\n")

	if len(searchLines) < 3 {
		return nil
	}

	// Remove trailing empty line
	if len(searchLines) > 0 && searchLines[len(searchLines)-1] == "" {
		searchLines = searchLines[:len(searchLines)-1]
	}

	firstLineSearch := strings.TrimSpace(searchLines[0])
	lastLineSearch := strings.TrimSpace(searchLines[len(searchLines)-1])
	searchBlockSize := len(searchLines)

	// Find candidate positions
	type candidate struct {
		startLine int
		endLine   int
	}
	var candidates []candidate

	for i := 0; i < len(originalLines); i++ {
		if strings.TrimSpace(originalLines[i]) != firstLineSearch {
			continue
		}

		// Look for matching last line
		for j := i + 2; j < len(originalLines); j++ {
			if strings.TrimSpace(originalLines[j]) == lastLineSearch {
				candidates = append(candidates, candidate{startLine: i, endLine: j})
				break
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Single candidate with relaxed threshold
	if len(candidates) == 1 {
		c := candidates[0]
		actualBlockSize := c.endLine - c.startLine + 1

		similarity := 0.0
		linesToCheck := min(searchBlockSize-2, actualBlockSize-2)

		if linesToCheck > 0 {
			for j := 1; j < searchBlockSize-1 && j < actualBlockSize-1; j++ {
				originalLine := strings.TrimSpace(originalLines[c.startLine+j])
				searchLine := strings.TrimSpace(searchLines[j])
				maxLen := max(len(originalLine), len(searchLine))
				if maxLen == 0 {
					continue
				}
				distance := levenshtein(originalLine, searchLine)
				similarity += (1.0 - float64(distance)/float64(maxLen)) / float64(linesToCheck)

				if similarity >= singleCandidateSimilarityThreshold {
					break
				}
			}
		} else {
			similarity = 1.0
		}

		if similarity >= singleCandidateSimilarityThreshold {
			return []string{extractBlock(content, originalLines, c.startLine, c.endLine)}
		}
		return nil
	}

	// Multiple candidates - find best match
	var bestMatch *candidate
	maxSimilarity := -1.0

	for _, c := range candidates {
		actualBlockSize := c.endLine - c.startLine + 1
		similarity := 0.0
		linesToCheck := min(searchBlockSize-2, actualBlockSize-2)

		if linesToCheck > 0 {
			for j := 1; j < searchBlockSize-1 && j < actualBlockSize-1; j++ {
				originalLine := strings.TrimSpace(originalLines[c.startLine+j])
				searchLine := strings.TrimSpace(searchLines[j])
				maxLen := max(len(originalLine), len(searchLine))
				if maxLen == 0 {
					continue
				}
				distance := levenshtein(originalLine, searchLine)
				similarity += 1.0 - float64(distance)/float64(maxLen)
			}
			similarity /= float64(linesToCheck)
		} else {
			similarity = 1.0
		}

		if similarity > maxSimilarity {
			maxSimilarity = similarity
			bestMatch = &c
		}
	}

	if maxSimilarity >= multipleCandidatesSimilarityThreshold && bestMatch != nil {
		return []string{extractBlock(content, originalLines, bestMatch.startLine, bestMatch.endLine)}
	}

	return nil
}

// whitespaceNormalizedReplacer normalizes whitespace for matching
func whitespaceNormalizedReplacer(content, find string) []string {
	normalizeWhitespace := func(text string) string {
		return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(text, " "))
	}

	normalizedFind := normalizeWhitespace(find)
	var results []string

	// Single line matches
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if normalizeWhitespace(line) == normalizedFind {
			results = append(results, line)
		}
	}

	// Multi-line matches
	findLines := strings.Split(find, "\n")
	if len(findLines) > 1 {
		for i := 0; i <= len(lines)-len(findLines); i++ {
			block := strings.Join(lines[i:i+len(findLines)], "\n")
			if normalizeWhitespace(block) == normalizedFind {
				results = append(results, block)
			}
		}
	}

	return results
}

// indentationFlexibleReplacer ignores leading indentation
func indentationFlexibleReplacer(content, find string) []string {
	removeIndentation := func(text string) string {
		lines := strings.Split(text, "\n")
		var nonEmptyLines []string
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				nonEmptyLines = append(nonEmptyLines, line)
			}
		}

		if len(nonEmptyLines) == 0 {
			return text
		}

		minIndent := math.MaxInt32
		for _, line := range nonEmptyLines {
			leadingSpaces := len(line) - len(strings.TrimLeft(line, " \t"))
			if leadingSpaces < minIndent {
				minIndent = leadingSpaces
			}
		}

		var result []string
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				result = append(result, line)
			} else {
				if len(line) >= minIndent {
					result = append(result, line[minIndent:])
				} else {
					result = append(result, line)
				}
			}
		}

		return strings.Join(result, "\n")
	}

	normalizedFind := removeIndentation(find)
	contentLines := strings.Split(content, "\n")
	findLines := strings.Split(find, "\n")

	var results []string

	for i := 0; i <= len(contentLines)-len(findLines); i++ {
		block := strings.Join(contentLines[i:i+len(findLines)], "\n")
		if removeIndentation(block) == normalizedFind {
			results = append(results, block)
		}
	}

	return results
}

// trimmedBoundaryReplacer tries matching with trimmed boundaries
func trimmedBoundaryReplacer(content, find string) []string {
	trimmedFind := strings.TrimSpace(find)

	if trimmedFind == find {
		return nil // Already trimmed
	}

	var results []string

	if strings.Contains(content, trimmedFind) {
		results = append(results, trimmedFind)
	}

	// Try finding blocks with trimmed content
	lines := strings.Split(content, "\n")
	findLines := strings.Split(find, "\n")

	for i := 0; i <= len(lines)-len(findLines); i++ {
		block := strings.Join(lines[i:i+len(findLines)], "\n")
		if strings.TrimSpace(block) == trimmedFind {
			results = append(results, block)
		}
	}

	return results
}

// extractBlock extracts text block from content using line indices
func extractBlock(content string, lines []string, startLine, endLine int) string {
	matchStartIndex := 0
	for k := 0; k < startLine; k++ {
		matchStartIndex += len(lines[k]) + 1
	}

	matchEndIndex := matchStartIndex
	for k := startLine; k <= endLine; k++ {
		matchEndIndex += len(lines[k])
		if k < endLine {
			matchEndIndex++ // Add newline
		}
	}

	return content[matchStartIndex:matchEndIndex]
}

// levenshtein calculates the Levenshtein distance between two strings
func levenshtein(a, b string) int {
	if a == "" || b == "" {
		return max(len(a), len(b))
	}

	// Create matrix
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := 0; j <= len(b); j++ {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}

// Helper functions
func min(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
