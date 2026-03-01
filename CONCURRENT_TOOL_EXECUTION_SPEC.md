# Concurrent Tool Execution

## Purpose

Execute multiple tool calls in parallel when Claude requests more than one tool in a single response. Currently tools run sequentially — if Claude asks to run `grep` and `list` in the same turn, the second waits for the first to finish. Concurrent execution reduces latency proportionally to the number of independent tool calls.

## Scope

- When the API response contains multiple `ToolUseBlock` entries, execute them concurrently.
- Collect all results before appending them to the conversation as a single user message.
- Maintain correct ordering of tool results (matching each result to its tool use ID).
- Add a configurable concurrency limit to avoid spawning too many goroutines.
- The spinner should reflect that multiple tools are running.
- Error in one tool must not cancel or affect other tools.

## Current State

`agent/agent.go` processes tool use blocks sequentially in the `SendMessage` loop:

```go
for _, block := range message.Content {
    switch b := block.AsAny().(type) {
    case anthropic.ToolUseBlock:
        hasToolUse = true
        inputJSON, _ := json.Marshal(b.Input)
        cb.OnToolCall(b.Name, string(inputJSON))
        result := a.executeToolUse(b, cb)
        if result != nil {
            toolResults = append(toolResults, *result)
        }
    }
}
```

Each `executeToolUse` call blocks until complete. If Claude returns 3 tool calls and each takes 1 second, the total is 3 seconds instead of ~1 second.

The `tool.Tool` interface is already stateless — each `Call` receives its own `ToolUseBlock` and returns an independent result. Tools are safe to call concurrently.

## Changes Required

### 1. `agent/agent.go` — Concurrent tool execution

Replace the sequential tool processing with a concurrent pattern. First, collect all tool use blocks, then execute them in parallel, then collect results.

**Extract tool use blocks:**

```go
// Collect tool use blocks from response
var toolUseBlocks []anthropic.ToolUseBlock
for _, block := range message.Content {
    switch b := block.AsAny().(type) {
    case anthropic.TextBlock:
        finalText = b.Text
        cb.OnText(b.Text)
    case anthropic.ToolUseBlock:
        toolUseBlocks = append(toolUseBlocks, b)
    }
}
hasToolUse = len(toolUseBlocks) > 0
```

**Execute concurrently:**

```go
if len(toolUseBlocks) > 0 {
    toolResults = a.executeToolsConcurrently(ctx, toolUseBlocks, cb)
}
```

**New method `executeToolsConcurrently`:**

```go
// toolResult pairs a result with its original index to preserve ordering.
type toolResult struct {
    index  int
    result anthropic.ContentBlockParamUnion
}

func (a *Agent) executeToolsConcurrently(ctx context.Context, blocks []anthropic.ToolUseBlock, cb Callbacks) []anthropic.ContentBlockParamUnion {
    // For a single tool, just run it directly (no goroutine overhead)
    if len(blocks) == 1 {
        inputJSON, _ := json.Marshal(blocks[0].Input)
        cb.OnToolCall(blocks[0].Name, string(inputJSON))
        result := a.executeToolUse(blocks[0], cb)
        if result != nil {
            return []anthropic.ContentBlockParamUnion{*result}
        }
        return nil
    }

    // Notify callbacks of all tool calls upfront
    for _, block := range blocks {
        inputJSON, _ := json.Marshal(block.Input)
        cb.OnToolCall(block.Name, string(inputJSON))
    }

    // Execute tools concurrently with a semaphore for concurrency limit
    results := make([]toolResult, 0, len(blocks))
    var mu sync.Mutex
    var wg sync.WaitGroup

    sem := make(chan struct{}, a.config.MaxConcurrentTools)

    for i, block := range blocks {
        wg.Go(func() {
            sem <- struct{}{}        // Acquire semaphore
            defer func() { <-sem }() // Release semaphore

            result := a.executeToolUse(block, cb)
            if result != nil {
                mu.Lock()
                results = append(results, toolResult{index: i, result: *result})
                mu.Unlock()
            }
        })
    }
    wg.Wait()

    // Sort by original index to maintain order
    slices.SortFunc(results, func(a, b toolResult) int {
        return cmp.Compare(a.index, b.index)
    })

    // Extract just the results
    out := make([]anthropic.ContentBlockParamUnion, len(results))
    for i, r := range results {
        out[i] = r.result
    }
    return out
}
```

Add `"sync"`, `"slices"`, and `"cmp"` to the imports.

### 2. `agent/config.go` — Add concurrency config

Add a field to `Config`:

```go
type Config struct {
    Model              string
    MaxTokens          int64
    MaxConcurrentTools int // Max tools to execute in parallel (default: 4)
}
```

Update `DefaultConfig()`:

```go
func DefaultConfig() Config {
    return Config{
        Model:              string(anthropic.ModelClaude4Sonnet20250514),
        MaxTokens:          8192,
        MaxConcurrentTools: 4,
    }
}
```

### 3. `agent/config.go` — Thread-safe callbacks

Since tool results now arrive concurrently, the `Callbacks` interface methods may be called from multiple goroutines simultaneously. Document this requirement:

```go
// Callbacks is implemented by the UI layer to observe agent events.
// When concurrent tool execution is enabled, OnToolResult may be called
// from multiple goroutines. Implementations must be safe for concurrent use.
type Callbacks interface {
    // ... existing methods ...
}
```

### 4. `ui/terminal.go` — Thread-safe output

Terminal output methods are called from concurrent goroutines. Wrap writes in a mutex to prevent interleaved output:

```go
type Terminal struct {
    spinner *spinnerRunner
    mu      sync.Mutex
}
```

Update all output methods to acquire the lock:

```go
func (t *Terminal) OnToolCall(name string, input string) {
    t.mu.Lock()
    defer t.mu.Unlock()
    _, _ = fmt.Fprintf(os.Stdout, "%s: %s\n", claudeStyle.Render("Tool"), name+": "+input)
}

func (t *Terminal) OnToolResult(name string, output string, isError bool) {
    t.mu.Lock()
    defer t.mu.Unlock()
    status := "OK"
    if isError {
        status = "ERROR"
    }
    _, _ = fmt.Fprintf(os.Stdout, "%s\n", debugStyle.Render(fmt.Sprintf("[%s] %s", status, name)))
}
```

Apply the same pattern to `OnText`, `OnThinking`, `OnThinkingDone`, `PrintAssistant`, `PrintError`, and `ShowSpinner`.

Add `"sync"` to the imports.

### 5. `config.go` — Add environment variable

| Env Var | Default | Description |
|---------|---------|-------------|
| `ARTOO_MAX_CONCURRENT_TOOLS` | `4` | Maximum number of tools to execute in parallel |

```go
Agent: agent.Config{
    // ...
    MaxConcurrentTools: getEnvInt("ARTOO_MAX_CONCURRENT_TOOLS", 4),
},
```

Set to `1` to disable concurrent execution (sequential behaviour).

### 6. Tests

**`agent/agent_test.go`:**

- `TestExecuteToolsConcurrently_Single` — verify a single tool call skips goroutine overhead and returns correct result.
- `TestExecuteToolsConcurrently_Multiple` — provide 3 mock tool use blocks, verify all 3 results are returned in original order.
- `TestExecuteToolsConcurrently_OrderPreserved` — use tools with different sleep durations, verify results come back in input order (not completion order).
- `TestExecuteToolsConcurrently_ErrorDoesNotAffectOthers` — one tool returns an error, others succeed; verify all results present.
- `TestExecuteToolsConcurrently_SemaphoreLimit` — set MaxConcurrentTools=1, verify at most 1 tool runs at a time (use a counter with atomic operations).

**`ui/terminal_test.go`:**

- `TestTerminal_ConcurrentOnToolResult` — call OnToolResult from multiple goroutines simultaneously, verify no panics or data races (run with `-race`).

## Files Changed

| File | Change |
|------|--------|
| `agent/agent.go` | Extract tool use blocks, add `executeToolsConcurrently` method, replace sequential loop |
| `agent/config.go` | Add `MaxConcurrentTools` to Config, update DefaultConfig, document thread-safety |
| `ui/terminal.go` | Add `sync.Mutex` to Terminal, wrap all output methods |
| `config.go` | Add `ARTOO_MAX_CONCURRENT_TOOLS` env var |
| `CONFIG.md` | Document new env var |
| `agent/agent_test.go` | New tests for concurrent execution |
| `ui/terminal_test.go` | New test for thread-safe output |

## Verification

1. `go build ./...` compiles cleanly.
2. `go test ./...` passes.
3. `go test -race ./...` passes (no data races).
4. Manual test: ask Claude to list files and search for a pattern in a single prompt. Verify both tools run and results appear faster than sequential.
5. Manual test: set `ARTOO_MAX_CONCURRENT_TOOLS=1`, verify tools run sequentially (same behaviour as before).
6. Manual test: verify output from concurrent tools is not interleaved (each tool's output is a complete line).
