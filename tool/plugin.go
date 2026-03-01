package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

const schemaTimeoutDuration = 5 * time.Second

var (
	errPluginNotFound      = errors.New("plugin not found")
	errPluginIsDirectory   = errors.New("plugin path is a directory")
	errPluginNotExecutable = errors.New("plugin is not executable")
	errPluginEmptyName     = errors.New("plugin has empty name in schema")
	errPluginSchemaFailed  = errors.New("plugin schema failed")
	errInvalidSchemaJSON   = errors.New("invalid schema JSON")
)

// PluginSchema is the JSON structure returned by --schema.
type PluginSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// PluginTool wraps an external executable as a Tool.
type PluginTool struct {
	path    string        // absolute path to executable
	schema  PluginSchema
	timeout time.Duration // execution timeout
}

// NewPluginTool creates a PluginTool by reading the schema from the executable.
func NewPluginTool(path string, timeout time.Duration) (*PluginTool, error) {
	// Verify executable exists and is executable
	info, err := os.Stat(path)
	if err != nil {
		return nil, errors.Join(errPluginNotFound, err)
	}

	if info.IsDir() {
		return nil, errPluginIsDirectory
	}

	if info.Mode()&0111 == 0 {
		return nil, errPluginNotExecutable
	}

	// Read schema
	ctx, cancel := context.WithTimeout(context.Background(), schemaTimeoutDuration)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--schema")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, errors.Join(errPluginSchemaFailed, err)
	}

	var schema PluginSchema
	if err := json.Unmarshal(stdout.Bytes(), &schema); err != nil {
		return nil, errors.Join(errInvalidSchemaJSON, err)
	}

	if schema.Name == "" {
		return nil, errPluginEmptyName
	}

	return &PluginTool{
		path:    path,
		schema:  schema,
		timeout: timeout,
	}, nil
}

// Call executes the plugin, passing input JSON via stdin.
func (p *PluginTool) Call(block anthropic.ToolUseBlock) *anthropic.ContentBlockParamUnion {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.path) //nolint:gosec
	cmd.Stdin = bytes.NewReader([]byte(block.JSON.Input.Raw()))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := fmt.Sprintf("Plugin error: %v", err)
		if stderr.Len() > 0 {
			errMsg = fmt.Sprintf("Plugin error: %v\n%s", err, stderr.String())
		}

		return new(anthropic.NewToolResultBlock(block.ID, errMsg, true))
	}

	return new(anthropic.NewToolResultBlock(block.ID, stdout.String(), false))
}

// Param returns the anthropic tool parameter from the plugin's schema.
func (p *PluginTool) Param() anthropic.ToolParam {
	param := anthropic.ToolParam{
		Name:        p.schema.Name,
		Description: anthropic.String(p.schema.Description),
	}

	if p.schema.InputSchema != nil {
		// Convert map to InputSchemaParam
		properties, _ := p.schema.InputSchema["properties"].(map[string]any)
		required, _ := p.schema.InputSchema["required"].([]any)

		reqStrings := make([]string, 0, len(required))
		for _, r := range required {
			if s, ok := r.(string); ok {
				reqStrings = append(reqStrings, s)
			}
		}

		param.InputSchema = anthropic.ToolInputSchemaParam{
			Properties: properties,
			Required:   reqStrings,
		}
	}

	return param
}
