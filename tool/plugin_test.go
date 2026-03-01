package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// TestNewPluginTool_Schema creates a test script that outputs valid schema JSON on --schema,
// then verifies PluginTool fields are correctly set.
func TestNewPluginTool_Schema(t *testing.T) {
	t.Parallel()

	// Create temporary directory and script
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test-tool")

	scriptContent := `#!/bin/bash
if [ "$1" = "--schema" ]; then
    cat <<'EOF'
{
  "name": "test-tool",
  "description": "A test tool",
  "inputSchema": {
    "type": "object",
    "properties": {
      "input": {
        "type": "string",
        "description": "Test input"
      }
    },
    "required": ["input"]
  }
}
EOF
    exit 0
fi
`

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0600); err != nil {
		t.Fatalf("Failed to write test script: %v", err)
	}

	if err := os.Chmod(scriptPath, 0700); err != nil { //nolint:gosec
		t.Fatalf("Failed to chmod test script: %v", err)
	}

	// Create PluginTool
	pt, err := NewPluginTool(scriptPath, 5*time.Second)
	if err != nil {
		t.Fatalf("NewPluginTool failed: %v", err)
	}

	// Verify fields
	if pt.schema.Name != "test-tool" {
		t.Errorf("Expected name 'test-tool', got %q", pt.schema.Name)
	}
	if pt.schema.Description != "A test tool" {
		t.Errorf("Expected description 'A test tool', got %q", pt.schema.Description)
	}
	if pt.timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", pt.timeout)
	}
}

// TestNewPluginTool_MissingFile verifies that non-existent path returns error.
func TestNewPluginTool_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := NewPluginTool("/nonexistent/path/to/plugin", 5*time.Second)
	if err == nil {
		t.Errorf("Expected error for missing file, got nil")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' in error message, got: %v", err)
	}
}

// TestNewPluginTool_NotExecutable verifies that file without execute permission returns error.
func TestNewPluginTool_NotExecutable(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "not-executable")

	// Create a non-executable file
	if err := os.WriteFile(filePath, []byte("test"), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := NewPluginTool(filePath, 5*time.Second)
	if err == nil {
		t.Errorf("Expected error for non-executable file, got nil")
	}

	if !strings.Contains(err.Error(), "not executable") {
		t.Errorf("Expected 'not executable' in error message, got: %v", err)
	}
}

// TestNewPluginTool_InvalidSchema verifies that invalid JSON schema returns error.
func TestNewPluginTool_InvalidSchema(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "invalid-schema")

	// Create a script that outputs invalid JSON
	scriptContent := `#!/bin/bash
if [ "$1" = "--schema" ]; then
    echo "invalid json {"
    exit 0
fi
`

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0600); err != nil {
		t.Fatalf("Failed to write test script: %v", err)
	}

	if err := os.Chmod(scriptPath, 0700); err != nil { //nolint:gosec
		t.Fatalf("Failed to chmod test script: %v", err)
	}

	_, err := NewPluginTool(scriptPath, 5*time.Second)
	if err == nil {
		t.Errorf("Expected error for invalid schema JSON, got nil")
	}

	if !strings.Contains(err.Error(), "invalid schema JSON") {
		t.Errorf("Expected 'invalid schema JSON' in error message, got: %v", err)
	}
}

// TestPluginTool_Call_Success verifies that a plugin can be called successfully
// and returns the expected result.
func TestPluginTool_Call_Success(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "echo-tool")

	// Create a script that reads from stdin and echoes the input field
	scriptContent := `#!/bin/bash
if [ "$1" = "--schema" ]; then
    cat <<'EOF'
{
  "name": "echo",
  "description": "Echo tool",
  "inputSchema": {
    "type": "object",
    "properties": {
      "text": {"type": "string"}
    },
    "required": ["text"]
  }
}
EOF
    exit 0
fi
jq -r '.text' | cat
`

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0600); err != nil {
		t.Fatalf("Failed to write test script: %v", err)
	}

	if err := os.Chmod(scriptPath, 0700); err != nil { //nolint:gosec
		t.Fatalf("Failed to chmod test script: %v", err)
	}

	pt, err := NewPluginTool(scriptPath, 5*time.Second)
	if err != nil {
		t.Fatalf("NewPluginTool failed: %v", err)
	}

	// Create a ToolUseBlock with test input
	block := anthropic.ToolUseBlock{
		ID:    "test-call-1",
		Name:  "echo",
		Input: json.RawMessage(`{"text":"hello world"}`),
	}

	// Call the plugin
	result := pt.Call(block)

	// Verify result
	if result == nil {
		t.Fatalf("Expected non-nil result, got nil")
	}
	if result.OfToolResult == nil {
		t.Fatalf("Expected ToolResultBlock, got nil")
	}
	if result.OfToolResult.IsError.Value {
		t.Errorf("Expected success (isError=false), got error")
	}
	if len(result.OfToolResult.Content) == 0 {
		t.Fatalf("Expected content in result, got none")
	}
	if result.OfToolResult.Content[0].OfText == nil {
		t.Fatalf("Expected text content, got nil")
	}
}

// TestPluginTool_Call_Error verifies that plugin exit code 1 returns an error result.
func TestPluginTool_Call_Error(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "error-tool")

	// Create a script that exits with error code
	scriptContent := `#!/bin/bash
if [ "$1" = "--schema" ]; then
    cat <<'EOF'
{
  "name": "error",
  "description": "Error tool",
  "inputSchema": {
    "type": "object",
    "properties": {}
  }
}
EOF
    exit 0
fi
echo "Plugin failed" >&2
exit 1
`

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0600); err != nil {
		t.Fatalf("Failed to write test script: %v", err)
	}

	if err := os.Chmod(scriptPath, 0700); err != nil { //nolint:gosec
		t.Fatalf("Failed to chmod test script: %v", err)
	}

	pt, err := NewPluginTool(scriptPath, 5*time.Second)
	if err != nil {
		t.Fatalf("NewPluginTool failed: %v", err)
	}

	block := anthropic.ToolUseBlock{
		ID:    "test-call-error",
		Name:  "error",
		Input: json.RawMessage(`{}`),
	}

	result := pt.Call(block)

	if result == nil {
		t.Fatalf("Expected non-nil result, got nil")
	}
	if result.OfToolResult == nil {
		t.Fatalf("Expected ToolResultBlock, got nil")
	}
	if !result.OfToolResult.IsError.Value {
		t.Errorf("Expected error (isError=true), got success")
	}
}

// TestPluginTool_Call_Timeout verifies that plugin timeout returns an error result.
// Note: Tests that a slow-running plugin is terminated by timeout.
func TestPluginTool_Call_Timeout(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "timeout-tool")

	// Create a script that blocks indefinitely on read (respects SIGTERM better than sleep)
	scriptContent := `#!/bin/bash
if [ "$1" = "--schema" ]; then
    cat <<'EOF'
{
  "name": "timeout",
  "description": "Timeout tool",
  "inputSchema": {
    "type": "object",
    "properties": {}
  }
}
EOF
    exit 0
fi
# Read from /dev/zero forever - will be killed by timeout
while true; do dd if=/dev/zero bs=1 count=1 2>/dev/null; done
`

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0600); err != nil {
		t.Fatalf("Failed to write test script: %v", err)
	}

	if err := os.Chmod(scriptPath, 0700); err != nil { //nolint:gosec
		t.Fatalf("Failed to chmod test script: %v", err)
	}

	pt, err := NewPluginTool(scriptPath, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("NewPluginTool failed: %v", err)
	}

	startTime := time.Now()
	block := anthropic.ToolUseBlock{
		ID:    "test-call-timeout",
		Name:  "timeout",
		Input: json.RawMessage(`{}`),
	}

	result := pt.Call(block)
	elapsed := time.Since(startTime)

	if result == nil {
		t.Fatalf("Expected non-nil result, got nil")
	}
	if result.OfToolResult == nil {
		t.Fatalf("Expected ToolResultBlock, got nil")
	}
	if !result.OfToolResult.IsError.Value {
		t.Errorf("Expected error (isError=true) from timeout, got success")
	}
	// Verify timeout actually happened (should be close to timeout duration)
	if elapsed > time.Second {
		t.Errorf("Plugin execution took too long (%v), timeout may not have worked", elapsed)
	}
}

// TestPluginTool_Param verifies that Param() returns correct anthropic.ToolParam.
func TestPluginTool_Param(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "param-tool")

	scriptContent := `#!/bin/bash
if [ "$1" = "--schema" ]; then
    cat <<'EOF'
{
  "name": "my-tool",
  "description": "My tool description",
  "inputSchema": {
    "type": "object",
    "properties": {
      "arg1": {"type": "string"},
      "arg2": {"type": "integer"}
    },
    "required": ["arg1"]
  }
}
EOF
    exit 0
fi
`

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0600); err != nil {
		t.Fatalf("Failed to write test script: %v", err)
	}

	if err := os.Chmod(scriptPath, 0700); err != nil { //nolint:gosec
		t.Fatalf("Failed to chmod test script: %v", err)
	}

	pt, err := NewPluginTool(scriptPath, 5*time.Second)
	if err != nil {
		t.Fatalf("NewPluginTool failed: %v", err)
	}

	param := pt.Param()

	if param.Name != "my-tool" {
		t.Errorf("Expected name 'my-tool', got %q", param.Name)
	}
	if param.Description.Value != "My tool description" {
		t.Errorf("Expected description 'My tool description', got %q", param.Description.Value)
	}
	if len(param.InputSchema.Required) != 1 || param.InputSchema.Required[0] != "arg1" {
		t.Errorf("Expected required=['arg1'], got %v", param.InputSchema.Required)
	}
}
