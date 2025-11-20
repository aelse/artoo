package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTool_Call(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	testTextFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\n"
	if err := os.WriteFile(testTextFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	testEmptyFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(testEmptyFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}

	testBinaryFile := filepath.Join(tmpDir, "binary.bin")
	binaryContent := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE}
	if err := os.WriteFile(testBinaryFile, binaryContent, 0644); err != nil {
		t.Fatalf("failed to create binary file: %v", err)
	}

	testImageFile := filepath.Join(tmpDir, "image.png")
	if err := os.WriteFile(testImageFile, []byte("fake png content"), 0644); err != nil {
		t.Fatalf("failed to create image file: %v", err)
	}

	tests := []struct {
		name        string
		params      ReadParams
		expectError bool
		expectInOutput []string
		notInOutput    []string
	}{
		{
			name: "read full file",
			params: ReadParams{
				FilePath: testTextFile,
			},
			expectInOutput: []string{
				"<file>",
				"</file>",
				"00001| Line 1",
				"00002| Line 2",
				"00003| Line 3",
				"00004| Line 4",
				"00005| Line 5",
			},
		},
		{
			name: "read with offset",
			params: ReadParams{
				FilePath: testTextFile,
				Offset:   intPtr(2), // Start from line 3 (0-based)
			},
			expectInOutput: []string{
				"00003| Line 3",
				"00004| Line 4",
				"00005| Line 5",
			},
			notInOutput: []string{
				"00001| Line 1",
				"00002| Line 2",
			},
		},
		{
			name: "read with limit",
			params: ReadParams{
				FilePath: testTextFile,
				Limit:    intPtr(2),
			},
			expectInOutput: []string{
				"00001| Line 1",
				"00002| Line 2",
				"Use 'offset' parameter to read beyond line 2",
			},
			notInOutput: []string{
				"00003| Line 3",
			},
		},
		{
			name: "read with offset and limit",
			params: ReadParams{
				FilePath: testTextFile,
				Offset:   intPtr(1),
				Limit:    intPtr(2),
			},
			expectInOutput: []string{
				"00002| Line 2",
				"00003| Line 3",
			},
			notInOutput: []string{
				"00001| Line 1",
				"00004| Line 4",
			},
		},
		{
			name: "read empty file",
			params: ReadParams{
				FilePath: testEmptyFile,
			},
			expectInOutput: []string{
				"<file>",
				"</file>",
				"(File is empty)",
			},
		},
		{
			name: "read binary file",
			params: ReadParams{
				FilePath: testBinaryFile,
			},
			expectError: true,
		},
		{
			name: "read image file",
			params: ReadParams{
				FilePath: testImageFile,
			},
			expectInOutput: []string{
				"Image file detected",
				"PNG",
			},
		},
		{
			name: "file not found",
			params: ReadParams{
				FilePath: filepath.Join(tmpDir, "nonexistent.txt"),
			},
			expectError: true,
		},
	}

	tool := &ReadTool{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			for _, expected := range tt.expectInOutput {
				if !strings.Contains(output, expected) {
					t.Errorf("expected output to contain %q, but it didn't.\nOutput:\n%s", expected, output)
				}
			}

			for _, notExpected := range tt.notInOutput {
				if strings.Contains(output, notExpected) {
					t.Errorf("expected output NOT to contain %q, but it did.\nOutput:\n%s", notExpected, output)
				}
			}
		})
	}
}

func TestReadTool_LongLines(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "long.txt")

	// Create a file with a very long line
	longLine := strings.Repeat("a", 3000)
	content := "Short line\n" + longLine + "\nAnother short line\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tool := &ReadTool{}
	output, err := tool.Call(ReadParams{FilePath: testFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that long line is truncated
	if !strings.Contains(output, "...") {
		t.Error("expected long line to be truncated with '...'")
	}

	// Check that short lines are intact
	if !strings.Contains(output, "Short line") {
		t.Error("expected short line to be present")
	}
	if !strings.Contains(output, "Another short line") {
		t.Error("expected another short line to be present")
	}
}

func TestReadTool_IsBinaryFile(t *testing.T) {
	tmpDir := t.TempDir()
	tool := &ReadTool{}

	tests := []struct {
		name           string
		filename       string
		content        []byte
		expectedBinary bool
	}{
		{
			name:           "text file",
			filename:       "test.txt",
			content:        []byte("Hello, World!\nThis is a text file.\n"),
			expectedBinary: false,
		},
		{
			name:           "file with null byte",
			filename:       "null.txt",
			content:        []byte("Hello\x00World"),
			expectedBinary: true,
		},
		{
			name:           "zip file by extension",
			filename:       "archive.zip",
			content:        []byte("PK\x03\x04"), // ZIP magic number
			expectedBinary: true,
		},
		{
			name:           "empty file",
			filename:       "empty.txt",
			content:        []byte(""),
			expectedBinary: false,
		},
		{
			name:           "file with high non-printable ratio",
			filename:       "binary.dat",
			content:        []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x01, 0x02},
			expectedBinary: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile := filepath.Join(tmpDir, tt.filename)
			if err := os.WriteFile(testFile, tt.content, 0644); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			isBinary, err := tool.isBinaryFile(testFile)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if isBinary != tt.expectedBinary {
				t.Errorf("expected isBinary=%v, got %v", tt.expectedBinary, isBinary)
			}
		})
	}
}

func TestReadTool_IsImageFile(t *testing.T) {
	tool := &ReadTool{}

	tests := []struct {
		filename      string
		expectedType  string
	}{
		{"image.jpg", "JPEG"},
		{"image.jpeg", "JPEG"},
		{"image.png", "PNG"},
		{"image.gif", "GIF"},
		{"image.bmp", "BMP"},
		{"image.webp", "WebP"},
		{"image.JPG", "JPEG"}, // Test case insensitivity
		{"not-image.txt", ""},
		{"file.go", ""},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := tool.isImageFile(tt.filename)
			if result != tt.expectedType {
				t.Errorf("expected %q, got %q", tt.expectedType, result)
			}
		})
	}
}

func TestReadTool_FindSimilarFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	files := []string{"test.txt", "test2.txt", "testing.go", "other.md"}
	for _, f := range files {
		path := filepath.Join(tmpDir, f)
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", f, err)
		}
	}

	tool := &ReadTool{}

	tests := []struct {
		name              string
		missingFile       string
		expectSuggestions bool
	}{
		{
			name:              "similar filename - partial match",
			missingFile:       filepath.Join(tmpDir, "test"),
			expectSuggestions: true, // Should find test.txt, test2.txt, testing.go
		},
		{
			name:              "no similar files",
			missingFile:       filepath.Join(tmpDir, "xyz123.txt"),
			expectSuggestions: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestions := tool.findSimilarFiles(tt.missingFile)

			if tt.expectSuggestions {
				if len(suggestions) == 0 {
					t.Error("expected suggestions but got none")
				}
			} else {
				if len(suggestions) > 0 {
					t.Errorf("expected no suggestions, got %v", suggestions)
				}
			}
		})
	}
}

func TestReadTool_Param(t *testing.T) {
	tool := &ReadTool{}
	param := tool.Param()

	if param.Name != "read" {
		t.Errorf("expected name to be 'read', got %q", param.Name)
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

	if _, ok := props["offset"]; !ok {
		t.Error("expected 'offset' property to exist")
	}

	if _, ok := props["limit"]; !ok {
		t.Error("expected 'limit' property to exist")
	}

	// Check required fields
	required := param.InputSchema.Required
	if len(required) != 1 || required[0] != "filePath" {
		t.Errorf("expected required=['filePath'], got %v", required)
	}
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}
