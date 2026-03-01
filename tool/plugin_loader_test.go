package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoadPlugins_EmptyDir verifies that an empty directory returns no tools.
func TestLoadPlugins_EmptyDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	tools, errs := LoadPlugins(tmpDir, 5*time.Second)

	if len(tools) != 0 {
		t.Errorf("Expected 0 tools, got %d", len(tools))
	}

	if len(errs) != 0 {
		t.Errorf("Expected 0 errors, got %d: %v", len(errs), errs)
	}
}

// TestLoadPlugins_MissingDir verifies that a non-existent directory returns no tools
// and no error (gracefully handled).
func TestLoadPlugins_MissingDir(t *testing.T) {
	t.Parallel()

	nonexistentDir := "/nonexistent/plugin/directory"

	tools, errs := LoadPlugins(nonexistentDir, 5*time.Second)

	if len(tools) != 0 {
		t.Errorf("Expected 0 tools, got %d", len(tools))
	}

	if len(errs) != 0 {
		t.Errorf("Expected 0 errors for missing directory, got %d: %v", len(errs), errs)
	}
}

// TestLoadPlugins_MultiplePlugins verifies that multiple valid plugins are loaded.
func TestLoadPlugins_MultiplePlugins(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create first valid plugin
	script1 := filepath.Join(tmpDir, "plugin1")
	content1 := `#!/bin/bash
if [ "$1" = "--schema" ]; then
    cat <<'EOF'
{"name": "plugin1", "description": "Plugin 1", "inputSchema": {"type": "object", "properties": {}}}
EOF
    exit 0
fi
`
	if err := os.WriteFile(script1, []byte(content1), 0600); err != nil {
		t.Fatalf("Failed to write plugin1: %v", err)
	}

	if err := os.Chmod(script1, 0700); err != nil { //nolint:gosec
		t.Fatalf("Failed to chmod plugin1: %v", err)
	}

	// Create second valid plugin
	script2 := filepath.Join(tmpDir, "plugin2")
	content2 := `#!/bin/bash
if [ "$1" = "--schema" ]; then
    cat <<'EOF'
{"name": "plugin2", "description": "Plugin 2", "inputSchema": {"type": "object", "properties": {}}}
EOF
    exit 0
fi
`
	if err := os.WriteFile(script2, []byte(content2), 0600); err != nil {
		t.Fatalf("Failed to write plugin2: %v", err)
	}

	if err := os.Chmod(script2, 0700); err != nil { //nolint:gosec
		t.Fatalf("Failed to chmod plugin2: %v", err)
	}

	tools, errs := LoadPlugins(tmpDir, 5*time.Second)

	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}

	if len(errs) != 0 {
		t.Errorf("Expected 0 errors, got %d: %v", len(errs), errs)
	}

	// Verify tool names
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Param().Name] = true
	}

	if !names["plugin1"] {
		t.Errorf("Expected plugin1 to be loaded")
	}

	if !names["plugin2"] {
		t.Errorf("Expected plugin2 to be loaded")
	}
}

// TestLoadPlugins_PartialFailure verifies that one valid plugin is loaded
// even when another plugin fails to load.
func TestLoadPlugins_PartialFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create valid plugin
	validScript := filepath.Join(tmpDir, "valid")
	validContent := `#!/bin/bash
if [ "$1" = "--schema" ]; then
    cat <<'EOF'
{"name": "valid", "description": "Valid Plugin", "inputSchema": {"type": "object", "properties": {}}}
EOF
    exit 0
fi
`
	if err := os.WriteFile(validScript, []byte(validContent), 0600); err != nil {
		t.Fatalf("Failed to write valid plugin: %v", err)
	}

	if err := os.Chmod(validScript, 0700); err != nil { //nolint:gosec
		t.Fatalf("Failed to chmod valid plugin: %v", err)
	}

	// Create invalid plugin (invalid JSON in schema)
	invalidScript := filepath.Join(tmpDir, "invalid")
	invalidContent := `#!/bin/bash
if [ "$1" = "--schema" ]; then
    echo "invalid json {{"
    exit 0
fi
`
	if err := os.WriteFile(invalidScript, []byte(invalidContent), 0600); err != nil {
		t.Fatalf("Failed to write invalid plugin: %v", err)
	}

	if err := os.Chmod(invalidScript, 0700); err != nil { //nolint:gosec
		t.Fatalf("Failed to chmod invalid plugin: %v", err)
	}

	tools, errs := LoadPlugins(tmpDir, 5*time.Second)

	if len(tools) != 1 {
		t.Errorf("Expected 1 tool (valid one), got %d", len(tools))
	}

	if len(errs) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errs))
	}

	if tools[0].Param().Name != "valid" {
		t.Errorf("Expected loaded tool to be 'valid', got %q", tools[0].Param().Name)
	}
}

// TestMergeTools_NoConflict verifies that built-in and plugin tools with unique names
// are merged successfully.
func TestMergeTools_NoConflict(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "plugin")

	scriptContent := `#!/bin/bash
if [ "$1" = "--schema" ]; then
    cat <<'EOF'
{"name": "custom-plugin", "description": "Custom Plugin", "inputSchema": {"type": "object", "properties": {}}}
EOF
    exit 0
fi
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0600); err != nil {
		t.Fatalf("Failed to write plugin: %v", err)
	}

	if err := os.Chmod(scriptPath, 0700); err != nil { //nolint:gosec
		t.Fatalf("Failed to chmod plugin: %v", err)
	}

	plugin, err := NewPluginTool(scriptPath, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to create plugin: %v", err)
	}

	plugins := []Tool{plugin}

	merged, err := MergeTools(AllTools, plugins)
	if err != nil {
		t.Fatalf("MergeTools failed: %v", err)
	}

	expectedLen := len(AllTools) + len(plugins)
	if len(merged) != expectedLen {
		t.Errorf("Expected %d tools in merged list, got %d", expectedLen, len(merged))
	}

	// Verify all tool names are unique
	names := make(map[string]bool)
	for _, tool := range merged {
		name := tool.Param().Name
		if names[name] {
			t.Errorf("Duplicate tool name: %q", name)
		}
		names[name] = true
	}

	// Verify custom plugin is in the merged list
	if !names["custom-plugin"] {
		t.Errorf("Expected 'custom-plugin' in merged tools")
	}
}

// TestMergeTools_Conflict verifies that a plugin with the same name as a built-in tool
// returns an error.
func TestMergeTools_Conflict(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "conflicting-plugin")

	// Create a plugin with the same name as a built-in tool (grep)
	scriptContent := `#!/bin/bash
if [ "$1" = "--schema" ]; then
    cat <<'EOF'
{"name": "grep", "description": "Conflicting Plugin", "inputSchema": {"type": "object", "properties": {}}}
EOF
    exit 0
fi
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0600); err != nil {
		t.Fatalf("Failed to write plugin: %v", err)
	}

	if err := os.Chmod(scriptPath, 0700); err != nil { //nolint:gosec
		t.Fatalf("Failed to chmod plugin: %v", err)
	}

	plugin, err := NewPluginTool(scriptPath, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to create plugin: %v", err)
	}

	plugins := []Tool{plugin}

	_, err = MergeTools(AllTools, plugins)
	if err == nil {
		t.Errorf("Expected error for conflicting tool name, got nil")
	}

	if err != nil && !strings.Contains(err.Error(), "conflicts") {
		t.Errorf("Expected conflict error message, got: %v", err)
	}
}

// TestLoadPlugins_EmptyString verifies that an empty plugin directory string
// returns no tools and no errors.
func TestLoadPlugins_EmptyString(t *testing.T) {
	t.Parallel()
	tools, errs := LoadPlugins("", 5*time.Second)

	if len(tools) != 0 {
		t.Errorf("Expected 0 tools for empty dir, got %d", len(tools))
	}
	if len(errs) != 0 {
		t.Errorf("Expected 0 errors for empty dir, got %d", len(errs))
	}
}

// TestLoadPlugins_IgnoresDirectories verifies that subdirectories in the plugin
// directory are ignored (only files are loaded).
func TestLoadPlugins_IgnoresDirectories(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a subdirectory (should be ignored)
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0700); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Create a valid plugin file
	pluginScript := filepath.Join(tmpDir, "valid-plugin")
	content := `#!/bin/bash
if [ "$1" = "--schema" ]; then
    cat <<'EOF'
{"name": "valid", "description": "Valid", "inputSchema": {"type": "object", "properties": {}}}
EOF
    exit 0
fi
`
	if err := os.WriteFile(pluginScript, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to write plugin: %v", err)
	}

	if err := os.Chmod(pluginScript, 0700); err != nil { //nolint:gosec
		t.Fatalf("Failed to chmod plugin: %v", err)
	}

	tools, errs := LoadPlugins(tmpDir, 5*time.Second)

	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}
	if len(errs) != 0 {
		t.Errorf("Expected 0 errors, got %d: %v", len(errs), errs)
	}
}
