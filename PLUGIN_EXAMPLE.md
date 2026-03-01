# Plugin System - Example: DateTime Plugin

This directory includes an example plugin script (`PLUGIN_EXAMPLE_DATETIME.sh`) that demonstrates how to create custom tools for Artoo using the plugin system.

## What is a Plugin?

A plugin is an executable script (bash, Python, Go, etc.) that:
1. Responds to `--schema` flag with JSON describing the tool
2. Reads JSON input from stdin
3. Writes the result to stdout
4. Uses exit code 0 for success, non-zero for errors

## Example: DateTime Plugin

The `PLUGIN_EXAMPLE_DATETIME.sh` script provides a tool that returns the current date and time in various formats.

### Installation

```bash
# Create the plugins directory (if it doesn't exist)
mkdir -p ~/.artoo/plugins

# Copy the example plugin
cp PLUGIN_EXAMPLE_DATETIME.sh ~/.artoo/plugins/datetime

# Make it executable
chmod +x ~/.artoo/plugins/datetime
```

### Usage

The plugin supports the following formats:
- **iso8601** (default): `2026-03-01T09:06:16Z`
- **unix**: `1772355976` (seconds since epoch)
- **rfc2822**: `Sun, 01 Mar 2026 09:06:16 +0000`
- **custom strftime**: any valid `strftime` format (e.g., `%Y-%m-%d %H:%M:%S`)

#### From Artoo

Once installed, the plugin will be loaded automatically when Artoo starts. You can ask Claude to use it:

```
You: "What time is it right now?"
Claude: I'll check the current time for you.
[Tool: datetime] → "2026-03-01T09:06:16Z"
```

You can also request specific formats:

```
You: "Give me today's date in a readable format like 'Monday, January 02, 2006'"
Claude: I'll get that for you.
[Tool: datetime with format "%A, %B %d, %Y"] → "Sunday, March 01, 2026"
```

#### Manual Testing

```bash
# Test the schema
./PLUGIN_EXAMPLE_DATETIME.sh --schema

# Default format (ISO8601)
echo '{}' | ./PLUGIN_EXAMPLE_DATETIME.sh

# Unix timestamp
echo '{"format":"unix"}' | ./PLUGIN_EXAMPLE_DATETIME.sh

# Custom format
echo '{"format":"%Y-%m-%d %H:%M:%S"}' | ./PLUGIN_EXAMPLE_DATETIME.sh
```

## Creating Your Own Plugin

Here's a template for creating a custom plugin:

```bash
#!/bin/bash
# My Custom Plugin

if [ "$1" = "--schema" ]; then
    # Output JSON schema describing the tool
    cat <<'EOF'
{
  "name": "my-tool",
  "description": "Description of what the tool does",
  "input_schema": {
    "type": "object",
    "properties": {
      "param1": {
        "type": "string",
        "description": "Description of parameter 1"
      },
      "param2": {
        "type": "integer",
        "description": "Description of parameter 2"
      }
    },
    "required": ["param1"]
  }
}
EOF
    exit 0
fi

# Read JSON input from stdin
INPUT=$(cat)

# Extract parameters using jq or similar
PARAM1=$(echo "$INPUT" | jq -r '.param1')
PARAM2=$(echo "$INPUT" | jq -r '.param2 // 0')

# Do your work here
RESULT="You provided: $PARAM1 and $PARAM2"

# Write result to stdout
echo "$RESULT"

# Exit with 0 for success, non-zero for errors
exit 0
```

### Plugin Schema Format

The schema must be valid JSON with these fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique tool name (a-z, 0-9, hyphens, underscores) |
| `description` | string | Yes | Human-readable description |
| `input_schema` | object | Yes | JSON Schema describing input parameters |

The `input_schema` follows the [JSON Schema](https://json-schema.org/) format:

```json
{
  "type": "object",
  "properties": {
    "field_name": {
      "type": "string|integer|boolean|number",
      "description": "Field description"
    }
  },
  "required": ["field_name"]  // Fields that must be provided
}
```

### Plugin Best Practices

1. **Keep it focused**: One tool should do one thing well
2. **Clear naming**: Use hyphenated lowercase names (e.g., `send-email`, `query-db`)
3. **Proper error handling**: Exit with non-zero codes on failure
4. **Reasonable timeouts**: Design plugins to complete within 30 seconds by default
5. **JSON-compliant input/output**: Ensure all JSON is valid
6. **Logging to stderr**: Use stderr for debugging, stdout for results
7. **No side effects**: Prefer read-only operations or clearly document changes

### Examples of Plugins to Create

- **Database Query**: Execute SELECT queries against a database
- **Email Sender**: Send emails with customizable subject/body
- **Slack Integration**: Post messages to Slack channels
- **File Operations**: Read/write files on the system
- **API Caller**: Call custom APIs with parameter substitution
- **Calculator**: Advanced mathematical operations
- **Text Processor**: Format conversion (Markdown to HTML, JSON prettify, etc.)
- **System Info**: CPU, memory, disk usage
- **Weather Data**: Real-time weather for a location
- **News Fetcher**: Recent news headlines from a topic

## Configuration

Set these environment variables to customize plugin behavior:

```bash
# Directory where plugins are located (default: ~/.artoo/plugins)
export ARTOO_PLUGIN_DIR=/path/to/plugins

# Timeout for plugin execution in seconds (default: 30)
export ARTOO_PLUGIN_TIMEOUT=60
```

## Troubleshooting

### Plugin not loading

Check the debug output:
```bash
ARTOO_DEBUG=true artoo
```

Look for messages like:
```
Debug: Loaded 1 plugins
Debug:   - my-tool
```

### Schema parsing fails

Verify your schema is valid JSON:
```bash
./my-plugin --schema | jq empty
```

If it returns an error, fix the JSON.

### Plugin execution fails

Test the plugin directly:
```bash
echo '{"param":"value"}' | ./my-plugin
```

Check stderr for error messages and exit code:
```bash
./my-plugin; echo "Exit code: $?"
```

### Name conflicts

Plugin names must not match built-in tools. Check available built-in tools:
- `ls`
- `grep`
- `random-number`

If your plugin conflicts, Artoo will exit with an error on startup.

## File Locations

- **Plugin directory**: `~/.artoo/plugins/`
- **Example plugin**: `PLUGIN_EXAMPLE_DATETIME.sh` (this repo)
- **Environment vars**: See `config.go` for all available settings

## Security Considerations

⚠️ **Important**: Plugins are executable files that run with your user permissions.

- Only install plugins from trusted sources
- Review plugin code before installing
- Plugins can read/write files, make network calls, etc.
- Be cautious with plugins that require sensitive credentials
- Consider using environment variables to pass secrets

## Additional Resources

- [Plugin Loading Specification](PLUGIN_TOOL_LOADING_SPEC.md)
- [JSON Schema Documentation](https://json-schema.org/)
- [jq Manual](https://stedolan.github.io/jq/manual/) (for parsing JSON in bash)
