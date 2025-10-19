# Adding New Tools - Example

This document shows how to add a new tool using the generic TypedTool interface.

## Benefits of the Generic Refactor

- **No JSON unmarshalling boilerplate**: The `toolWrapper` handles all JSON unmarshalling automatically
- **Type safety**: Your tool's `Call` method receives strongly-typed parameters (no type assertions needed)
- **Consistent error handling**: All tools benefit from standardized error wrapping
- **Clean separation**: Tool logic is separate from plumbing code

## Real Examples

### Grep Tool
See [tool/grep.go](tool/grep.go) for a complete implementation of a grep tool that:
- Uses optional parameters (`*string` for nullable fields)
- Executes external commands (ripgrep)
- Parses and formats output
- Handles multiple error cases

The entire tool is ~200 lines with **zero JSON unmarshalling code** - all handled by the generic wrapper!

### List (ls) Tool
See [tool/ls.go](tool/ls.go) and [tool/ls_test.go](tool/ls_test.go) for a complete implementation with tests that:
- Uses array parameters (`[]string` for ignore patterns)
- Builds tree-structured output
- Has comprehensive unit tests (100% coverage)
- Demonstrates proper error handling

### Read Tool
See [tool/read.go](tool/read.go) and [tool/read_test.go](tool/read_test.go) for a file reading tool that:
- Implements offset/limit pagination for large files
- Detects and rejects binary files
- Identifies image files
- Provides helpful suggestions for file-not-found errors
- Formats output with line numbers (cat -n style)
- Comprehensive test suite covering edge cases

### Edit Tool
See [tool/edit.go](tool/edit.go) and [tool/edit_test.go](tool/edit_test.go) for an advanced file editing tool that:
- Performs exact string replacements with fuzzy matching fallbacks
- Implements 6 different matching strategies for robustness:
  - Simple exact match
  - Line-trimmed matching (ignores leading/trailing whitespace per line)
  - Block anchor matching (fuzzy matches blocks using first/last lines as anchors with Levenshtein distance)
  - Whitespace-normalized matching
  - Indentation-flexible matching
  - Trimmed boundary matching
- Handles single occurrence (unique match) vs multiple occurrences (error or replaceAll)
- Comprehensive Levenshtein distance algorithm for fuzzy matching
- 550+ lines with sophisticated pattern matching - all using the generic wrapper!

### Write Tool
See [tool/write.go](tool/write.go) - A simple file writing tool that:
- Creates new files or overwrites existing ones
- Requires absolute paths
- Minimal implementation showing the simplicity of basic tools

### Glob Tool
See [tool/glob.go](tool/glob.go) - A file pattern matching tool that:
- Uses ripgrep for fast file discovery
- Supports glob patterns like `**/*.go`
- Sorts results by modification time
- Limits to 100 results with truncation notice

### Bash Tool
See [tool/bash.go](tool/bash.go) - Command execution tool that:
- Executes bash commands with configurable timeouts
- Captures stdout and stderr
- Handles long-running processes
- Truncates output at 30KB limit

### WebFetch Tool
See [tool/webfetch.go](tool/webfetch.go) - HTTP client tool that:
- Fetches content from URLs
- Supports multiple output formats (text, markdown, html)
- Configurable timeouts
- 5MB response size limit

### MultiEdit Tool
See [tool/multiedit.go](tool/multiedit.go) - Batch editing tool that:
- Applies multiple edits to a single file sequentially
- Uses the Edit tool internally
- Atomic operations (all or nothing)
- Useful for complex refactoring tasks

All implemented tools demonstrate the power of the generic refactor - from simple 75-line tools to complex 550-line implementations, all with **zero JSON boilerplate**!

## Example: Calculator Tool

Here's how to add a calculator tool:

```go
// 1. Define your parameter type
type CalculatorParams struct {
    Operation string  `json:"operation"` // "add", "subtract", "multiply", "divide"
    A         float64 `json:"a"`
    B         float64 `json:"b"`
}

// 2. Create your tool struct
type CalculatorTool struct{}

// 3. Ensure it implements TypedTool[CalculatorParams] (compile-time check)
var _ TypedTool[CalculatorParams] = (*CalculatorTool)(nil)

// 4. Implement Call with typed parameters (no unmarshalling needed!)
func (t *CalculatorTool) Call(params CalculatorParams) (string, error) {
    var result float64

    switch params.Operation {
    case "add":
        result = params.A + params.B
    case "subtract":
        result = params.A - params.B
    case "multiply":
        result = params.A * params.B
    case "divide":
        if params.B == 0 {
            return "", errors.New("division by zero")
        }
        result = params.A / params.B
    default:
        return "", fmt.Errorf("unknown operation: %s", params.Operation)
    }

    return strconv.FormatFloat(result, 'f', -1, 64), nil
}

// 5. Implement Param to define the tool for Claude API
func (t *CalculatorTool) Param() anthropic.ToolParam {
    return anthropic.ToolParam{
        Name:        "calculator",
        Description: anthropic.String("Perform basic arithmetic operations"),
        InputSchema: anthropic.ToolInputSchemaParam{
            Properties: map[string]interface{}{
                "operation": map[string]interface{}{
                    "type":        "string",
                    "description": "Operation to perform: add, subtract, multiply, divide",
                    "enum":        []string{"add", "subtract", "multiply", "divide"},
                },
                "a": map[string]interface{}{
                    "type":        "number",
                    "description": "First operand",
                },
                "b": map[string]interface{}{
                    "type":        "number",
                    "description": "Second operand",
                },
            },
            Required: []string{"operation", "a", "b"},
        },
    }
}

// 6. Register the tool in AllTools (in tool.go)
var AllTools = []Tool{
    WrapTypedTool[RandomNumberParams](&RandomNumberTool{}),
    WrapTypedTool[CalculatorParams](&CalculatorTool{}),  // Add this line
}
```

## What Happens Under the Hood

1. **Registration**: `WrapTypedTool[CalculatorParams](&CalculatorTool{})` creates a `toolWrapper[CalculatorParams]`
2. **At runtime**: When Claude calls the tool, the wrapper:
   - Receives the `ToolUseBlock` with raw JSON
   - Unmarshals JSON into `CalculatorParams`
   - Calls your typed `Call(params CalculatorParams)` method
   - Wraps the result in a `ToolResultBlock`
3. **Your code**: Just implements business logic with clean, typed parameters

## Comparison: Before vs After

### Before (manual unmarshalling)
```go
func (t *CalculatorTool) Call(block anthropic.ToolUseBlock) *anthropic.ContentBlockParamUnion {
    var params CalculatorParams

    // Manual unmarshalling boilerplate
    err := json.Unmarshal([]byte(block.JSON.Input.Raw()), &params)
    if err != nil {
        // Manual error handling
        fmt.Fprintf(os.Stdout, "error unmarshalling params: %v\n", err)
        return nil
    }

    // Business logic
    result := doCalculation(params)

    // Manual response creation
    result := anthropic.NewToolResultBlock(block.ID, result, false)
    return &result
}
```

### After (generic wrapper)
```go
func (t *CalculatorTool) Call(params CalculatorParams) (string, error) {
    // Just business logic - everything else is handled!
    return doCalculation(params), nil
}
```

Much cleaner!
