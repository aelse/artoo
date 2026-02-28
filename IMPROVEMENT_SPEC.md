# Artoo Improvement Specification

## Goals

1. **Separate agent logic from UI** so the core agent loop can be tested, reused, and driven by different frontends (CLI, TUI, API server, tests).
2. **Manage conversation context** so the token window doesn't grow unbounded and blow past model limits or rack up unnecessary cost.
3. **Make configuration explicit** so model, token limits, and behaviour can be changed without editing code.

---

## 1. Separate Agent Logic from UI

### Problem

`agent/agent.go` owns the REPL loop, API calls, tool dispatch, spinners, text input, styling, and direct `os.Stdout` writes — all in one file and one struct. This makes it impossible to test the agent without a terminal, swap the UI, or run the agent headless.

### Proposed Package Structure

```
artoo/
├── main.go                  # Wiring: build config, create agent + UI, run
├── agent/
│   ├── agent.go             # Core agent: send message, process response, tool dispatch
│   └── config.go            # AgentConfig struct
├── conversation/
│   ├── conversation.go      # Message storage, context window management
│   └── conversation_test.go
├── ui/
│   ├── terminal.go          # CLI frontend: input, output, spinners, styles
│   └── events.go            # Event types shared between agent and UI
├── tool/
│   ├── tool.go              # Tool interfaces + wrapper (unchanged)
│   ├── grep.go
│   ├── ls.go
│   └── random_number.go
```

### Agent Package

Strip the agent down to pure logic with no terminal I/O.

```go
// agent/config.go
type Config struct {
    Model     string // e.g. "claude-sonnet-4-20250514"
    MaxTokens int64  // per-response token limit
}
```

```go
// agent/agent.go
type Agent struct {
    client       anthropic.Client
    conversation *conversation.Conversation
    tools        []tool.Tool
    toolMap      map[string]tool.Tool
    config       Config
}

// SendMessage appends a user message, calls the API, processes tool use
// in a loop until there are no more tool calls, then returns the final
// assistant response. It calls the provided callbacks so the UI layer
// can react without the agent knowing about terminals.
func (a *Agent) SendMessage(ctx context.Context, text string, cb Callbacks) (*Response, error)
```

The `Callbacks` interface lets the UI observe what happens without the agent importing any UI code:

```go
type Callbacks interface {
    OnThinking()                          // API call starting
    OnThinkingDone()                      // API call finished
    OnText(text string)                   // Assistant produced text
    OnToolCall(name string, input string) // Tool call starting
    OnToolResult(name string, output string, isError bool) // Tool call finished
}
```

`Response` is a simple struct:

```go
type Response struct {
    Text       string
    StopReason string
}
```

The current `processResponse` / `handleToolUse` / `callClaude` methods become private helpers inside `SendMessage`. The agent loop (read input, send message, repeat) moves out of the agent package entirely.

### UI Package

The `ui` package owns everything terminal-related:

- **Styles** — `lipgloss.Style` definitions, currently globals in `agent/agent.go init()`.
- **Spinner** — the `spinnerRunner` type moves here.
- **Input** — the `inputModel` / bubbletea program moves here.
- **Output** — formatted printing of assistant text, tool calls, errors, debug info.

```go
// ui/terminal.go
type Terminal struct { /* styles, writer */ }

func (t *Terminal) ReadInput() (string, error)       // Bubbletea text input
func (t *Terminal) PrintAssistant(text string)        // Styled output
func (t *Terminal) PrintToolCall(name, input string)  // Debug-styled tool info
func (t *Terminal) PrintToolResult(name, output string, isError bool)
func (t *Terminal) PrintError(err error)
func (t *Terminal) ShowSpinner(message string) func() // Returns stop function
```

`Terminal` implements `agent.Callbacks` so it can be passed directly to `SendMessage`.

### Main Package (the REPL)

`main.go` becomes the simple REPL loop that wires everything together:

```go
func main() {
    cfg := loadConfig()          // From env / flags / file
    client := anthropic.NewClient(...)
    term := ui.NewTerminal()
    a := agent.New(client, cfg)

    for {
        input, err := term.ReadInput()
        if input == "quit" || input == "exit" { break }

        resp, err := a.SendMessage(ctx, input, term)
        if err != nil { term.PrintError(err) }
    }
}
```

### Migration Steps

1. Create `agent/config.go` with `Config` struct. Thread it through `Agent`.
2. Create `ui/` package. Move `spinnerRunner`, `inputModel`, style vars, and all `fmt.Fprint(os.Stdout, ...)` calls there.
3. Define `Callbacks` interface in `agent/`. Implement it on `ui.Terminal`.
4. Refactor `Agent.Run` into `Agent.SendMessage` — remove the loop and all direct I/O. The internal tool-use loop stays (call API, execute tools, call API again if needed).
5. Move the REPL loop into `main.go`.
6. Remove `agent.Run(ctx)` top-level function (currently unused convenience wrapper at bottom of agent.go).
7. Remove `printConversation()` debug dump — replace with an optional `OnDebug` callback or a debug flag on the terminal.

---

## 2. Conversation Context Management

### Problem

`conversation.Conversation` is an append-only `[]anthropic.MessageParam` with no limits. Every message — including large tool results — accumulates forever. On long sessions this will:

- Exceed the model's context window and cause API errors.
- Send unnecessary tokens on every request, increasing latency and cost.

### Proposed Changes to `conversation/conversation.go`

#### 2a. Token Budget Awareness

Add a configurable token budget and track usage:

```go
type Config struct {
    MaxContextTokens int // e.g. 180_000 for Sonnet's 200k window, with headroom
}

type Conversation struct {
    messages         []anthropic.MessageParam
    config           Config
    totalInputTokens int // Updated from API response usage
}
```

After each API response, update the running token count from `message.Usage.InputTokens`. This gives an accurate server-side count without needing a local tokenizer.

#### 2b. Sliding Window with System Message Preservation

When `totalInputTokens` approaches `MaxContextTokens`, trim the oldest messages (after any system message) until usage drops below a target threshold (e.g. 75% of max). The most recent messages are always preserved because they contain the active tool-use exchange.

```go
func (c *Conversation) Trim() {
    // Keep system message (index 0) if present
    // Remove oldest user/assistant pairs from the front
    // Reset totalInputTokens estimate (will be corrected on next API call)
}
```

Trimming should happen *before* sending the next request, not after — so the request that would exceed the window never gets sent.

#### 2c. Truncate Large Tool Results

Tool outputs (especially `ls` and `grep`) can be very large. Before appending a tool result, truncate it if it exceeds a configurable limit (e.g. 10,000 characters), appending a note that the output was truncated. This is a cheap safeguard independent of the sliding window.

```go
func (c *Conversation) AppendToolResult(result anthropic.ContentBlockParamUnion, maxChars int) {
    // If text content exceeds maxChars, truncate and append "(truncated)" note
    // Then append normally
}
```

#### 2d. Expose Conversation Metrics

Add methods so the UI or agent can report on context usage:

```go
func (c *Conversation) MessageCount() int
func (c *Conversation) EstimatedTokens() int
```

### What This Does NOT Include

- **Summarisation** — compressing old messages with an LLM call. This adds complexity, cost, and latency. The sliding window is simpler and sufficient for a CLI agent. Summarisation can be added later if needed.
- **Persistence** — saving/loading conversation history to disk. Worth doing eventually but orthogonal to the context window problem.

---

## 3. Configuration

### Problem

Model name, max tokens, and tool limits are all hardcoded constants.

### Proposed Approach

A single `Config` struct loaded in `main.go` from environment variables, with sensible defaults:

| Setting | Env Var | Default |
|---|---|---|
| Model | `ARTOO_MODEL` | `claude-sonnet-4-20250514` |
| Max response tokens | `ARTOO_MAX_TOKENS` | `8192` |
| Max context tokens | `ARTOO_MAX_CONTEXT_TOKENS` | `180000` |
| Tool result max chars | `ARTOO_TOOL_RESULT_MAX_CHARS` | `10000` |
| Debug output | `ARTOO_DEBUG` | `false` |

No config file, no YAML parsing, no flags library. Environment variables are sufficient for now and add zero dependencies.

---

## 4. Summary of Changes by File

| Current File | What Happens |
|---|---|
| `main.go` | Becomes the REPL loop. Loads config, creates agent + terminal, runs loop. |
| `agent/agent.go` | Loses all UI code. Gains `Callbacks` interface. `Run` becomes `SendMessage`. |
| `agent/config.go` | New file. `Config` struct for model, tokens. |
| `conversation/conversation.go` | Gains token tracking, `Trim()`, tool result truncation, metrics. |
| `ui/terminal.go` | New file. All styles, spinner, input, output printing. Implements `Callbacks`. |
| `ui/events.go` | New file. Shared event/callback types if needed. |
| `tool/` | No changes to tool implementations. Tool interface unchanged. |

### Out of Scope

These are worth doing later but are not part of this restructure:

- System prompt configuration
- Conversation persistence / history
- Streaming responses
- Concurrent tool execution
- Plugin/dynamic tool loading
- Full TUI (scrollable output, panels)
