# System Prompt Configuration

## Purpose

Allow the user to set a system prompt that shapes Claude's behaviour for the entire conversation. Currently the agent sends no system prompt at all — Claude uses its default personality. A configurable system prompt lets users tailor the agent for specific tasks (e.g. "You are a Go code reviewer", "Respond only in bullet points", "Always use Australian English").

## Scope

- Add a `SystemPrompt` field to the agent's configuration.
- Load it from an environment variable (`ARTOO_SYSTEM_PROMPT`) and optionally from a file path (`ARTOO_SYSTEM_PROMPT_FILE`).
- Pass the system prompt to the Claude API on every request.
- No runtime mutation of the system prompt — it is set once at startup.

## Current State

The Claude API call in `agent/agent.go` line 62–67 constructs `MessageNewParams` without a `System` field:

```go
message, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
    Model:     anthropic.Model(a.config.Model),
    MaxTokens: a.config.MaxTokens,
    Messages:  a.conversation.Messages(),
    Tools:     a.toolUnionParams,
})
```

The `MessageNewParams` struct in the anthropic SDK accepts a `System` field of type `[]anthropic.TextBlockParam`.

Configuration is loaded in `config.go` via `LoadConfig()` and stored in `AppConfig`.

## Changes Required

### 1. `agent/config.go` — Add SystemPrompt field

Add a `SystemPrompt` field to the `Config` struct:

```go
type Config struct {
    Model        string
    MaxTokens    int64
    SystemPrompt string // Optional system prompt text
}
```

No change to `DefaultConfig()` — the default is an empty string (no system prompt).

### 2. `agent/agent.go` — Pass system prompt to API

In `SendMessage`, when building `MessageNewParams`, conditionally include the system prompt:

```go
params := anthropic.MessageNewParams{
    Model:     anthropic.Model(a.config.Model),
    MaxTokens: a.config.MaxTokens,
    Messages:  a.conversation.Messages(),
    Tools:     a.toolUnionParams,
}

if a.config.SystemPrompt != "" {
    params.System = []anthropic.TextBlockParam{
        anthropic.NewTextBlock(a.config.SystemPrompt),
    }
}

message, err := a.client.Messages.New(ctx, params)
```

Note: The system prompt is **not** stored in the conversation messages. It is sent as a separate field on every API call. This is how the Claude API expects it.

### 3. `config.go` — Load from environment

Add two new environment variable lookups in `LoadConfig()`:

| Env Var | Purpose |
|---------|---------|
| `ARTOO_SYSTEM_PROMPT` | Inline system prompt text |
| `ARTOO_SYSTEM_PROMPT_FILE` | Path to a file containing the system prompt |

If both are set, `ARTOO_SYSTEM_PROMPT` takes precedence. If `ARTOO_SYSTEM_PROMPT_FILE` is set, read the file contents at startup and use that as the system prompt.

```go
func loadSystemPrompt() string {
    if prompt := os.Getenv("ARTOO_SYSTEM_PROMPT"); prompt != "" {
        return prompt
    }
    if path := os.Getenv("ARTOO_SYSTEM_PROMPT_FILE"); path != "" {
        data, err := os.ReadFile(path)
        if err != nil {
            fmt.Fprintf(os.Stderr, "Warning: could not read system prompt file %s: %v\n", path, err)
            return ""
        }
        return strings.TrimSpace(string(data))
    }
    return ""
}
```

Wire into `LoadConfig()`:

```go
Agent: agent.Config{
    Model:        getEnv("ARTOO_MODEL", "claude-sonnet-4-20250514"),
    MaxTokens:    getEnvInt64("ARTOO_MAX_TOKENS", 8192),
    SystemPrompt: loadSystemPrompt(),
},
```

Add `"strings"` to the imports in `config.go`.

### 4. `main.go` — Debug logging

In the debug block, add the system prompt info:

```go
if cfg.Debug {
    fmt.Fprintf(os.Stderr, "Debug: Model=%s MaxTokens=%d MaxContext=%d SystemPrompt=%q\n",
        cfg.Agent.Model, cfg.Agent.MaxTokens, cfg.Conversation.MaxContextTokens,
        truncateForLog(cfg.Agent.SystemPrompt, 80))
}
```

Add a helper `truncateForLog(s string, maxLen int) string` that truncates long strings for display.

### 5. `CONFIG.md` — Documentation

Add entries to the configuration table:

| Variable | Default | Description |
|----------|---------|-------------|
| `ARTOO_SYSTEM_PROMPT` | (empty) | System prompt text sent with every API call |
| `ARTOO_SYSTEM_PROMPT_FILE` | (empty) | Path to file containing the system prompt |

Add usage examples showing how to set a system prompt inline and via file.

### 6. Tests

Add to `config_test.go`:

- `TestLoadSystemPrompt_Inline` — set `ARTOO_SYSTEM_PROMPT`, verify it appears in `cfg.Agent.SystemPrompt`.
- `TestLoadSystemPrompt_File` — write a temp file, set `ARTOO_SYSTEM_PROMPT_FILE`, verify contents loaded.
- `TestLoadSystemPrompt_InlinePrecedence` — set both, verify inline wins.
- `TestLoadSystemPrompt_Empty` — set neither, verify empty string.
- `TestLoadSystemPrompt_FileMissing` — set `ARTOO_SYSTEM_PROMPT_FILE` to a non-existent path, verify empty string returned (not a crash).

## Files Changed

| File | Change |
|------|--------|
| `agent/config.go` | Add `SystemPrompt` field to `Config` |
| `agent/agent.go` | Conditionally set `System` field in `MessageNewParams` |
| `config.go` | Add `loadSystemPrompt()`, wire into `LoadConfig()` |
| `main.go` | Add system prompt to debug output |
| `CONFIG.md` | Document new env vars |
| `config_test.go` | 5 new tests |

## Verification

1. `go build ./...` compiles cleanly.
2. `go test ./...` passes, including new tests.
3. Manual test: set `ARTOO_SYSTEM_PROMPT="Always respond in haiku format"`, run artoo, verify Claude responds in haiku.
4. Manual test: create a file `prompt.txt` with a system prompt, set `ARTOO_SYSTEM_PROMPT_FILE=prompt.txt`, verify it works.
5. Manual test: set neither, verify agent works exactly as before.
