package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLsTool_Call(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Create test directory structure:
	// tmpDir/
	//   file1.txt
	//   file2.go
	//   subdir/
	//     file3.txt
	//     nested/
	//       file4.txt
	//   .git/
	//     config (should be ignored)

	testFiles := []string{
		"file1.txt",
		"file2.go",
		"subdir/file3.txt",
		"subdir/nested/file4.txt",
		".git/config",
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(tmpDir, file)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		err = os.WriteFile(fullPath, []byte("test content"), 0644)
		if err != nil {
			t.Fatalf("failed to create file %s: %v", file, err)
		}
	}

	tests := []struct {
		name           string
		params         LsParams
		expectError    bool
		expectedInTree []string
		notInTree      []string
	}{
		{
			name:   "list current directory",
			params: LsParams{Path: &tmpDir},
			expectedInTree: []string{
				"file1.txt",
				"file2.go",
				"subdir/",
				"file3.txt",
				"nested/",
				"file4.txt",
			},
			notInTree: []string{
				".git/", // Should be ignored by default
				"config",
			},
		},
		{
			name:   "list with nil path (uses current dir)",
			params: LsParams{Path: nil},
			// Should work without error
			expectError: false,
		},
		{
			name: "list with custom ignore pattern",
			params: LsParams{
				Path:   &tmpDir,
				Ignore: []string{"*.txt"},
			},
			expectedInTree: []string{
				"file2.go",
			},
			notInTree: []string{
				"file1.txt",
				"file3.txt",
				"file4.txt",
			},
		},
		{
			name: "list with directory ignore pattern",
			params: LsParams{
				Path:   &tmpDir,
				Ignore: []string{"subdir/"},
			},
			expectedInTree: []string{
				"file1.txt",
				"file2.go",
			},
			notInTree: []string{
				"subdir/",
				"file3.txt",
				"nested/",
			},
		},
	}

	tool := &LsTool{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := tool.Call(tt.params)

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

			// Check that expected strings are in the output
			for _, expected := range tt.expectedInTree {
				if !strings.Contains(output, expected) {
					t.Errorf("expected output to contain %q, but it didn't.\nOutput:\n%s", expected, output)
				}
			}

			// Check that unwanted strings are not in the output
			for _, notExpected := range tt.notInTree {
				if strings.Contains(output, notExpected) {
					t.Errorf("expected output NOT to contain %q, but it did.\nOutput:\n%s", notExpected, output)
				}
			}
		})
	}
}

func TestLsTool_RenderTree(t *testing.T) {
	tool := &LsTool{}

	tests := []struct {
		name     string
		basePath string
		files    []string
		truncate bool
		expected []string // Strings that should be in the output
	}{
		{
			name:     "simple flat directory",
			basePath: "/test",
			files:    []string{"file1.txt", "file2.go"},
			truncate: false,
			expected: []string{
				"/test/",
				"file1.txt",
				"file2.go",
			},
		},
		{
			name:     "nested directories",
			basePath: "/test",
			files: []string{
				"file1.txt",
				"subdir/file2.txt",
				"subdir/nested/file3.txt",
			},
			truncate: false,
			expected: []string{
				"/test/",
				"subdir/",
				"nested/",
				"file1.txt",
				"file2.txt",
				"file3.txt",
			},
		},
		{
			name:     "truncated results",
			basePath: "/test",
			files:    []string{"file1.txt"},
			truncate: true,
			expected: []string{
				"/test/",
				"file1.txt",
				"Results truncated",
			},
		},
		{
			name:     "empty directory",
			basePath: "/test",
			files:    []string{},
			truncate: false,
			expected: []string{
				"/test/",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := tool.renderTree(tt.basePath, tt.files, tt.truncate)

			for _, expected := range tt.expected {
				if !strings.Contains(output, expected) {
					t.Errorf("expected output to contain %q, but it didn't.\nOutput:\n%s", expected, output)
				}
			}
		})
	}
}

func TestLsTool_RenderTreeStructure(t *testing.T) {
	tool := &LsTool{}

	files := []string{
		"a.txt",
		"b/c.txt",
		"b/d/e.txt",
	}

	output := tool.renderTree("/root", files, false)

	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Verify the tree structure has proper indentation
	// Expected structure:
	// /root/
	//   b/
	//     d/
	//       e.txt
	//     c.txt
	//   a.txt

	// Check that directories come before their files
	bDirIndex := -1
	cFileIndex := -1
	dDirIndex := -1
	eFileIndex := -1

	for i, line := range lines {
		if strings.Contains(line, "b/") && strings.Contains(line, "  b/") {
			bDirIndex = i
		}
		if strings.Contains(line, "c.txt") {
			cFileIndex = i
		}
		if strings.Contains(line, "d/") {
			dDirIndex = i
		}
		if strings.Contains(line, "e.txt") {
			eFileIndex = i
		}
	}

	if bDirIndex == -1 {
		t.Error("b/ directory not found in output")
	}
	if dDirIndex == -1 {
		t.Error("d/ directory not found in output")
	}

	// d/ should come before e.txt (subdirectory before its files)
	if dDirIndex >= eFileIndex && eFileIndex != -1 {
		t.Errorf("expected d/ (index %d) to come before e.txt (index %d)", dDirIndex, eFileIndex)
	}

	// b/ should come before c.txt (directory before its files)
	if bDirIndex >= cFileIndex && cFileIndex != -1 {
		t.Errorf("expected b/ (index %d) to come before c.txt (index %d)", bDirIndex, cFileIndex)
	}
}

func TestLsTool_Param(t *testing.T) {
	tool := &LsTool{}
	param := tool.Param()

	if param.Name != "list" {
		t.Errorf("expected name to be 'list', got %q", param.Name)
	}

	// Check description is set (it's an Opt[string] type)
	if param.Description.Value == "" {
		t.Error("expected description to be set")
	}

	// Check that required properties exist
	if param.InputSchema.Properties == nil {
		t.Fatal("expected properties to be set")
	}

	props, ok := param.InputSchema.Properties.(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be a map")
	}

	if _, ok := props["path"]; !ok {
		t.Error("expected 'path' property to exist")
	}

	if _, ok := props["ignore"]; !ok {
		t.Error("expected 'ignore' property to exist")
	}
}
