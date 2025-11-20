package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditTool_Call(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	initialContent := "Line 1\nLine 2\nLine 3\n"
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tests := []struct {
		name           string
		params         EditParams
		expectError    bool
		expectedContent string
	}{
		{
			name: "simple replacement",
			params: EditParams{
				FilePath:  testFile,
				OldString: "Line 2",
				NewString: "Modified Line 2",
			},
			expectedContent: "Line 1\nModified Line 2\nLine 3\n",
		},
		{
			name: "multi-line replacement",
			params: EditParams{
				FilePath:  testFile,
				OldString: "Line 1\nLine 2",
				NewString: "New Line 1\nNew Line 2",
			},
			expectedContent: "New Line 1\nNew Line 2\nLine 3\n",
		},
		{
			name: "same old and new string",
			params: EditParams{
				FilePath:  testFile,
				OldString: "Line 1",
				NewString: "Line 1",
			},
			expectError: true,
		},
		{
			name: "string not found",
			params: EditParams{
				FilePath:  testFile,
				OldString: "Nonexistent",
				NewString: "Something",
			},
			expectError: true,
		},
	}

	tool := &EditTool{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset file content before each test
			if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
				t.Fatalf("failed to reset test file: %v", err)
			}

			output, err := tool.Call(tt.params)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none. Output: %s", output)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify file content
			content, err := os.ReadFile(testFile)
			if err != nil {
				t.Fatalf("failed to read file: %v", err)
			}

			if string(content) != tt.expectedContent {
				t.Errorf("expected content:\n%s\ngot:\n%s", tt.expectedContent, string(content))
			}
		})
	}
}

func TestEditTool_ReplaceAll(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "foo bar foo baz foo"

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tool := &EditTool{}
	replaceAll := true

	_, err := tool.Call(EditParams{
		FilePath:   testFile,
		OldString:  "foo",
		NewString:  "qux",
		ReplaceAll: &replaceAll,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	expected := "qux bar qux baz qux"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestReplace_Simple(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		oldString   string
		newString   string
		replaceAll  bool
		expected    string
		expectError bool
	}{
		{
			name:       "simple exact match",
			content:    "Hello, World!",
			oldString:  "World",
			newString:  "Go",
			expected:   "Hello, Go!",
		},
		{
			name:       "multi-line match",
			content:    "Line 1\nLine 2\nLine 3",
			oldString:  "Line 2",
			newString:  "Modified",
			expected:   "Line 1\nModified\nLine 3",
		},
		{
			name:       "replace all occurrences",
			content:    "a b a c a",
			oldString:  "a",
			newString:  "x",
			replaceAll: true,
			expected:   "x b x c x",
		},
		{
			name:        "string not found",
			content:     "Hello",
			oldString:   "Goodbye",
			newString:   "Hi",
			expectError: true,
		},
		{
			name:        "same old and new",
			content:     "Hello",
			oldString:   "Hello",
			newString:   "Hello",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := replace(tt.content, tt.oldString, tt.newString, tt.replaceAll)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSimpleReplacer(t *testing.T) {
	content := "Hello, World!\nThis is a test."
	find := "World"

	results := simpleReplacer(content, find)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0] != find {
		t.Errorf("expected %q, got %q", find, results[0])
	}
}

func TestLineTrimmedReplacer(t *testing.T) {
	content := "  Line 1\n    Line 2\n  Line 3"
	find := "Line 1\nLine 2"

	results := lineTrimmedReplacer(content, find)

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	// Should match despite different indentation
	if !strings.Contains(results[0], "Line 1") || !strings.Contains(results[0], "Line 2") {
		t.Errorf("unexpected result: %q", results[0])
	}
}

func TestBlockAnchorReplacer(t *testing.T) {
	content := `func example() {
    line1
    line2
    line3
}`

	// Try to match with slightly different middle content
	find := `func example() {
    line1
    modified_line2
    line3
}`

	results := blockAnchorReplacer(content, find)

	// Should find a match using fuzzy matching on middle lines
	if len(results) == 0 {
		t.Error("expected block anchor replacer to find a fuzzy match")
	}
}

func TestWhitespaceNormalizedReplacer(t *testing.T) {
	content := "Hello    World"
	find := "Hello World" // Single space

	results := whitespaceNormalizedReplacer(content, find)

	if len(results) == 0 {
		t.Error("expected whitespace normalized match")
	}
}

func TestIndentationFlexibleReplacer(t *testing.T) {
	content := "    func test() {\n        return true\n    }"
	find := "func test() {\n    return true\n}"

	results := indentationFlexibleReplacer(content, find)

	if len(results) == 0 {
		t.Error("expected indentation flexible match")
	}
}

func TestTrimmedBoundaryReplacer(t *testing.T) {
	content := "  Hello  "
	find := "  Hello  "

	results := trimmedBoundaryReplacer(content, find)

	if len(results) == 0 {
		t.Error("expected trimmed boundary match")
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a        string
		b        string
		expected int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "abcd", 1},
		{"kitten", "sitting", 3},
		{"Saturday", "Sunday", 3},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			result := levenshtein(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("levenshtein(%q, %q) = %d, expected %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestReplace_MultipleOccurrences(t *testing.T) {
	content := "apple banana apple cherry apple"

	// Without replaceAll, should error on multiple occurrences
	_, err := replace(content, "apple", "orange", false)
	if err == nil {
		t.Error("expected error for multiple occurrences without replaceAll")
	}
	if !strings.Contains(err.Error(), "multiple times") {
		t.Errorf("unexpected error message: %v", err)
	}

	// With replaceAll, should replace all
	result, err := replace(content, "apple", "orange", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "orange banana orange cherry orange"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestReplace_LineTrimmedWithIndentation(t *testing.T) {
	content := `function example() {
    if (true) {
        console.log("hello");
    }
}`

	oldString := `if (true) {
    console.log("hello");
}`

	newString := `if (false) {
    console.log("goodbye");
}`

	result, err := replace(content, oldString, newString, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "if (false)") {
		t.Errorf("replacement failed, result: %s", result)
	}
	if !strings.Contains(result, "goodbye") {
		t.Errorf("replacement failed, result: %s", result)
	}
}

func TestEditTool_Param(t *testing.T) {
	tool := &EditTool{}
	param := tool.Param()

	if param.Name != "edit" {
		t.Errorf("expected name to be 'edit', got %q", param.Name)
	}

	if param.Description.Value == "" {
		t.Error("expected description to be set")
	}

	props, ok := param.InputSchema.Properties.(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be a map")
	}

	if _, ok := props["filePath"]; !ok {
		t.Error("expected 'filePath' property to exist")
	}

	if _, ok := props["oldString"]; !ok {
		t.Error("expected 'oldString' property to exist")
	}

	if _, ok := props["newString"]; !ok {
		t.Error("expected 'newString' property to exist")
	}

	if _, ok := props["replaceAll"]; !ok {
		t.Error("expected 'replaceAll' property to exist")
	}

	// Check required fields
	required := param.InputSchema.Required
	if len(required) != 3 {
		t.Errorf("expected 3 required fields, got %d", len(required))
	}
}
