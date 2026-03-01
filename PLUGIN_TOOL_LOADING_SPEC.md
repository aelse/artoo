# Plugin / Dynamic Tool Loading

## Purpose

Allow users to add custom tools without modifying the artoo source code. Currently all tools are hardcoded in `tool/tool.go` via the `AllTools` slice. A plugin system lets users extend artoo with domain-specific tools (e.g. database queries, deployment scripts, project-specific commands) by dropping executable files into a directory.

## Scope

- Load external tools from a configurable directory at startup.
- External tools are executable files (scripts or binaries) that follow a simple protocol.
- Each plugin declares its tool definition (name, description, parameters) via a `--schema` flag.
- Tool execution passes JSON input via stdin and reads the result from stdout.
- Plugins are discovered once at startup — no hot-reloading.
- Built-in tools continue to work alongside plugins.
- Plugin tool names must not conflict with built-in tool names.

## Current State

Tools are defined as Go types implementing `TypedTool[P]`, wrapped via `WrapTypedTool`, and registered in `AllTools`:

```go
// tool/tool.go
var AllTools = []Tool{
    WrapTypedTool(&RandomNumberTool{}),
    WrapTypedTool(&GrepTool{}),
    WrapTypedTool(&LsTool{}),
}
```

The `Tool` interface requires two methods:

```go
type Tool interface {
    Call(block anthropic.ToolUseBlock) *anthropic.ContentBlockParamUnion
    Param() anthropic.ToolParam
}
```

The agent builds its tool list in `New()`:

```go
func New(client anthropic.Client, config Config) *Agent {
    allTools := tool.AllTools
    return &Agent{
        tools:           allTools,
        toolMap:         makeToolMap(allTools),
        toolUnionParams: makeToolUnionParams(allTools),
        // ...
    }
}
```

## Plugin Protocol

Each plugin is an executable file that supports two modes:

### Schema mode: `./my-tool --schema`

Prints a JSON object describing the tool:

```json
{
  "name": "deploy",
  "description": "Deploy the application to a specified environment",
  "input_schema": {
    "type": "object",
    "properties": {
      "environment": {
        "type": "string",
        "description": "Target environment (staging, production)"
      },
      "version": {
        "type": "string",
        "description": "Version tag to deploy"
      }
    },
    "required": ["environment"]
  }
}
```

The JSON structure maps directly to `anthropic.ToolParam`.

### Execution mode: `./my-tool`

Reads JSON input from stdin (the tool's input parameters as a JSON object). Writes the tool result to stdout as plain text. Exit code 0 means success; non-zero means error. Stderr is ignored (available for plugin logging).

Example execution:

```bash
echo '{"environment":"staging","version":"v1.2.3"}' | ./deploy
# stdout: Deployed v1.2.3 to staging successfully
# exit code: 0
```

## Changes Required

### 1. New file: `tool/plugin.go` — Plugin tool implementation

Create a `PluginTool` type that implements the `Tool` interface by shelling out to an external executable.

```go
package tool

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "time"

    "github.com/anthropics/anthropic-sdk-go"
)

// PluginSchema is the JSON structure returned by --schema.
type PluginSchema struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    InputSchema map[string]any `json:"input_schema"`
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
        return nil, fmt.Errorf("plugin not found: %w", err)
    }
    if info.IsDir() {
        return nil, fmt.Errorf("plugin path is a directory: %s", path)
    }
    if info.Mode()&0111 == 0 {
        return nil, fmt.Errorf("plugin is not executable: %s", path)
    }

    // Read schema
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    cmd := exec.CommandContext(ctx, path, "--schema")
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("plugin schema failed for %s: %w (stderr: %s)", path, err, stderr.String())
    }

    var schema PluginSchema
    if err := json.Unmarshal(stdout.Bytes(), &schema); err != nil {
        return nil, fmt.Errorf("invalid schema JSON from %s: %w", path, err)
    }

    if schema.Name == "" {
        return nil, fmt.Errorf("plugin %s has empty name in schema", path)
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

    cmd := exec.CommandContext(ctx, p.path)
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
```

### 2. New file: `tool/plugin_loader.go` — Discovery and loading

```go
package tool

import (
    "fmt"
    "os"
    "path/filepath"
    "time"
)

const defaultPluginTimeout = 30 * time.Second

// LoadPlugins discovers and loads all plugin tools from a directory.
// Returns the loaded tools and any errors encountered (non-fatal per plugin).
func LoadPlugins(dir string, timeout time.Duration) ([]Tool, []error) {
    if dir == "" {
        return nil, nil
    }

    entries, err := os.ReadDir(dir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil // Directory doesn't exist — no plugins
        }
        return nil, []error{fmt.Errorf("reading plugin directory %s: %w", dir, err)}
    }

    if timeout == 0 {
        timeout = defaultPluginTimeout
    }

    var tools []Tool
    var errs []error

    for _, entry := range entries {
        if entry.IsDir() {
            continue
        }

        path := filepath.Join(dir, entry.Name())
        plugin, err := NewPluginTool(path, timeout)
        if err != nil {
            errs = append(errs, fmt.Errorf("loading plugin %s: %w", entry.Name(), err))
            continue
        }

        tools = append(tools, plugin)
    }

    return tools, errs
}

// MergeTools combines built-in tools with plugin tools.
// Returns an error if any plugin name conflicts with a built-in tool.
func MergeTools(builtIn []Tool, plugins []Tool) ([]Tool, error) {
    names := make(map[string]bool)
    for _, t := range builtIn {
        names[t.Param().Name] = true
    }

    for _, p := range plugins {
        name := p.Param().Name
        if names[name] {
            return nil, fmt.Errorf("plugin tool %q conflicts with built-in tool", name)
        }
        names[name] = true
    }

    result := make([]Tool, 0, len(builtIn)+len(plugins))
    result = append(result, builtIn...)
    result = append(result, plugins...)
    return result, nil
}
```

### 3. `agent/agent.go` — Accept external tools

Update `New` to accept an optional list of additional tools:

```go
func New(client anthropic.Client, config Config, extraTools ...tool.Tool) *Agent {
    allTools := make([]tool.Tool, 0, len(tool.AllTools)+len(extraTools))
    allTools = append(allTools, tool.AllTools...)
    allTools = append(allTools, extraTools...)

    return &Agent{
        client:          client,
        conversation:    conversation.New(),
        tools:           allTools,
        toolMap:         makeToolMap(allTools),
        toolUnionParams: makeToolUnionParams(allTools),
        config:          config,
    }
}
```

### 4. `agent/config.go` — Add plugin config

```go
type Config struct {
    Model          string
    MaxTokens      int64
    PluginDir      string        // Directory containing plugin executables
    PluginTimeout  time.Duration // Execution timeout per plugin call
}
```

### 5. `config.go` — Add environment variables

| Env Var | Default | Description |
|---------|---------|-------------|
| `ARTOO_PLUGIN_DIR` | `~/.artoo/plugins` | Directory containing plugin executables |
| `ARTOO_PLUGIN_TIMEOUT` | `30` | Plugin execution timeout in seconds |

```go
Agent: agent.Config{
    // ...
    PluginDir:     getEnv("ARTOO_PLUGIN_DIR", filepath.Join(homeDir, ".artoo", "plugins")),
    PluginTimeout: time.Duration(getEnvInt("ARTOO_PLUGIN_TIMEOUT", 30)) * time.Second,
},
```

### 6. `main.go` — Load plugins at startup

After loading config and before creating the agent, load plugins:

```go
// Load plugins
var extraTools []tool.Tool
plugins, errs := tool.LoadPlugins(cfg.Agent.PluginDir, cfg.Agent.PluginTimeout)
if len(errs) > 0 {
    for _, err := range errs {
        fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
    }
}
if len(plugins) > 0 {
    merged, err := tool.MergeTools(tool.AllTools, plugins)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
    extraTools = plugins
    if cfg.Debug {
        fmt.Fprintf(os.Stderr, "Debug: Loaded %d plugins\n", len(plugins))
        for _, p := range plugins {
            fmt.Fprintf(os.Stderr, "Debug:   - %s\n", p.Param().Name)
        }
    }
    _ = merged // validation only; extraTools passed to agent
}

a := agent.New(client, cfg.Agent, extraTools...)
```

### 7. Tests

**`tool/plugin_test.go`:**

- `TestNewPluginTool_Schema` — create a test script that outputs valid schema JSON on `--schema`, verify PluginTool fields.
- `TestNewPluginTool_MissingFile` — non-existent path returns error.
- `TestNewPluginTool_NotExecutable` — file without execute permission returns error.
- `TestNewPluginTool_InvalidSchema` — script outputs invalid JSON, verify error.
- `TestPluginTool_Call_Success` — script reads stdin and echoes it back, verify result is success with correct output.
- `TestPluginTool_Call_Error` — script exits with code 1, verify result has isError=true.
- `TestPluginTool_Call_Timeout` — script sleeps forever, plugin has short timeout, verify error.

**`tool/plugin_loader_test.go`:**

- `TestLoadPlugins_EmptyDir` — empty directory returns no tools.
- `TestLoadPlugins_MissingDir` — non-existent directory returns no tools (not an error).
- `TestLoadPlugins_MultiplePlugins` — directory with 2 valid scripts, verify both loaded.
- `TestLoadPlugins_PartialFailure` — directory with 1 valid and 1 invalid script, verify 1 tool loaded and 1 error.
- `TestMergeTools_NoConflict` — built-in + plugin with unique names, verify merged list.
- `TestMergeTools_Conflict` — plugin with same name as built-in, verify error.

## Example Plugin

A simple bash plugin (`~/.artoo/plugins/weather`):

```bash
#!/bin/bash

if [ "$1" = "--schema" ]; then
    cat <<'EOF'
{
  "name": "weather",
  "description": "Get the current weather for a city",
  "input_schema": {
    "type": "object",
    "properties": {
      "city": {
        "type": "string",
        "description": "City name"
      }
    },
    "required": ["city"]
  }
}
EOF
    exit 0
fi

# Read input from stdin
INPUT=$(cat)
CITY=$(echo "$INPUT" | jq -r '.city')

# Fetch weather (example)
curl -s "https://wttr.in/${CITY}?format=3"
```

## Files Changed

| File | Change |
|------|--------|
| `tool/plugin.go` | New file: PluginTool, PluginSchema, NewPluginTool |
| `tool/plugin_loader.go` | New file: LoadPlugins, MergeTools |
| `tool/plugin_test.go` | New file: tests for PluginTool |
| `tool/plugin_loader_test.go` | New file: tests for LoadPlugins, MergeTools |
| `agent/agent.go` | Update New() to accept extraTools variadic parameter |
| `agent/config.go` | Add PluginDir, PluginTimeout to Config |
| `config.go` | Add ARTOO_PLUGIN_DIR, ARTOO_PLUGIN_TIMEOUT env vars |
| `main.go` | Load plugins at startup, pass to agent |
| `CONFIG.md` | Document new env vars and plugin protocol |

## Verification

1. `go build ./...` compiles cleanly.
2. `go test ./...` passes.
3. Manual test: create `~/.artoo/plugins/` directory with a simple test script. Start artoo, ask Claude to use the plugin tool, verify it works.
4. Manual test: create a plugin with a name that conflicts with a built-in tool, verify startup error.
5. Manual test: create a non-executable file in the plugins directory, verify warning on startup but artoo continues.
6. Manual test: verify artoo works normally when the plugins directory doesn't exist.
7. Manual test: set `ARTOO_DEBUG=true`, verify plugin loading is logged.
