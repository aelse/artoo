# Streaming Responses

## Purpose

Display Claude's text as it is generated, token by token, instead of waiting for the entire response to complete. This dramatically improves perceived latency — the user sees text appearing within milliseconds instead of waiting seconds for a full response. It also removes the need for a "Thinking..." spinner during text generation.

## Scope

- Replace the `Messages.New` (non-streaming) call with `Messages.NewStreaming` (streaming).
- Emit text deltas to the UI as they arrive via a new callback.
- Accumulate the full response for conversation history.
- Handle tool use blocks that arrive via streaming.
- The spinner remains for tool execution (which is not streamed).

## Current State

`agent/agent.go` line 62 calls `a.client.Messages.New(...)` which blocks until the full response is complete. Text is emitted to the UI only after the entire response is received, via `cb.OnText(b.Text)`.

The anthropic SDK provides `Messages.NewStreaming(ctx, params)` which returns a `*ssestream.Stream[MessageStreamEventUnion]`. The stream is consumed by calling `stream.Next()` in a loop and `stream.Current()` to get each event. Events include:

- `MessageStartEvent` — response metadata (model, usage)
- `ContentBlockStartEvent` — a new content block starting (text or tool_use)
- `ContentBlockDeltaEvent` — an incremental delta within a block
- `ContentBlockStopEvent` — a content block finished
- `MessageDeltaEvent` — final usage/stop_reason
- `MessageStopEvent` — stream complete

Text deltas arrive as `TextDelta` within `ContentBlockDeltaEvent`, with a `.Text` field containing the incremental text fragment.

Tool use blocks arrive as `ContentBlockStartEvent` with `ToolUse` type, followed by `InputJSONDelta` events carrying partial JSON for the tool input, and finally `ContentBlockStopEvent`.

## Changes Required

### 1. `agent/config.go` — Add streaming callback and config

Add a new callback method for incremental text:

```go
type Callbacks interface {
    OnThinking()
    OnThinkingDone()
    OnText(text string)                   // Full text block (kept for final/summary)
    OnTextDelta(delta string)             // Incremental text fragment (new)
    OnToolCall(name string, input string)
    OnToolResult(name string, output string, isError bool)
}
```

Add a streaming flag to `Config`:

```go
type Config struct {
    Model        string
    MaxTokens    int64
    SystemPrompt string
    Streaming    bool   // Whether to use streaming API (default: true)
}
```

Update `DefaultConfig()` to set `Streaming: true`.

### 2. `agent/agent.go` — Implement streaming API call

Replace the `Messages.New` call in `SendMessage` with a streaming variant when `a.config.Streaming` is true. Extract the current non-streaming path into a helper and add a new streaming path.

**New method: `callStreaming`**

```go
func (a *Agent) callStreaming(ctx context.Context, cb Callbacks) (*anthropic.Message, error) {
    stream := a.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
        Model:     anthropic.Model(a.config.Model),
        MaxTokens: a.config.MaxTokens,
        Messages:  a.conversation.Messages(),
        Tools:     a.toolUnionParams,
    })

    var message anthropic.Message

    for stream.Next() {
        event := stream.Current()

        switch e := event.AsAny().(type) {
        case anthropic.ContentBlockDeltaEvent:
            switch d := e.Delta.AsAny().(type) {
            case anthropic.TextDelta:
                cb.OnTextDelta(d.Text)
            }
        case anthropic.MessageStartEvent:
            message = e.Message
        case anthropic.MessageDeltaEvent:
            message.StopReason = anthropic.MessageStopReason(e.Delta.StopReason)
            if e.Usage != nil {
                message.Usage.OutputTokens = e.Usage.OutputTokens
            }
        }
    }

    if err := stream.Err(); err != nil {
        return nil, err
    }

    // Get the final accumulated message
    message = stream.FinalMessage()

    return &message, nil
}
```

Note: The SDK's `Stream` type provides `FinalMessage()` which returns the fully-accumulated `Message` after the stream ends. This message has the same structure as a non-streaming response, so all downstream code (tool use processing, conversation appending) works unchanged.

**Update `SendMessage` to branch on streaming:**

```go
var message *anthropic.Message
var err error

cb.OnThinking()
if a.config.Streaming {
    cb.OnThinkingDone() // Stop spinner before streaming starts
    message, err = a.callStreaming(ctx, cb)
} else {
    message, err = a.client.Messages.New(ctx, anthropic.MessageNewParams{...})
    cb.OnThinkingDone()
}
```

Key detail: when streaming, call `cb.OnThinkingDone()` **before** starting the stream so the spinner stops and text can flow. When not streaming, call it **after** the response completes.

**Adjust text handling:** When streaming, `OnTextDelta` delivers incremental text. The existing `OnText` call should still fire after the stream completes (for the full text block), so the UI can add a newline or do final formatting.

### 3. `ui/terminal.go` — Handle text deltas

Add the `OnTextDelta` implementation:

```go
func (t *Terminal) OnTextDelta(delta string) {
    _, _ = fmt.Fprint(os.Stdout, delta)
}
```

This writes raw text without a newline — each delta appends to the current line. The existing `OnText` method should be updated to just print a newline (since all the text has already been printed via deltas):

```go
func (t *Terminal) OnText(text string) {
    // When streaming, text has already been printed via OnTextDelta.
    // Just add a newline to finish the line.
    _, _ = fmt.Fprintln(os.Stdout)
}
```

However, when streaming is disabled, `OnTextDelta` is never called and `OnText` should print the full text as before. To handle this, add a `streaming` flag to `Terminal`:

```go
type Terminal struct {
    spinner   *spinnerRunner
    streaming bool
}

func NewTerminal(streaming bool) *Terminal {
    return &Terminal{streaming: streaming}
}

func (t *Terminal) OnText(text string) {
    if t.streaming {
        // Text was already printed via deltas; just finish the line
        _, _ = fmt.Fprintln(os.Stdout)
    } else {
        _, _ = fmt.Fprintf(os.Stdout, "%s: %s\n", claudeStyle.Render("Claude"), text)
    }
}
```

Update the `OnThinking` / `OnThinkingDone` callbacks: when streaming, `OnThinking` should print the "Claude: " prefix before text starts streaming, so deltas appear after it:

```go
func (t *Terminal) OnThinking() {
    if t.streaming {
        // Print prefix; text will stream after OnThinkingDone
        _, _ = fmt.Fprint(os.Stdout, claudeStyle.Render("Claude")+": ")
    } else {
        t.ShowSpinner("Thinking...")
    }
}

func (t *Terminal) OnThinkingDone() {
    if !t.streaming {
        if t.spinner != nil {
            t.spinner.stop()
            t.spinner = nil
        }
    }
    // When streaming, nothing to do — text is about to flow
}
```

### 4. `config.go` — Add streaming env var

| Env Var | Default | Description |
|---------|---------|-------------|
| `ARTOO_STREAMING` | `true` | Enable streaming responses |

```go
Agent: agent.Config{
    // ...
    Streaming: getEnvBool("ARTOO_STREAMING", true),
},
```

### 5. `main.go` — Pass streaming flag to Terminal

```go
term := ui.NewTerminal(cfg.Agent.Streaming)
```

### 6. Tests

Since streaming involves network I/O, tests should focus on the non-network parts:

- `TestCallbacks_OnTextDelta` — verify Terminal.OnTextDelta writes to stdout (capture output).
- `TestOnText_Streaming` — verify OnText just prints newline when streaming=true.
- `TestOnText_NonStreaming` — verify OnText prints full text when streaming=false.
- `TestOnThinking_Streaming` — verify OnThinking prints prefix when streaming=true.
- `TestOnThinking_NonStreaming` — verify OnThinking starts spinner when streaming=false.

## Files Changed

| File | Change |
|------|--------|
| `agent/config.go` | Add `OnTextDelta` to Callbacks, `Streaming` to Config |
| `agent/agent.go` | Add `callStreaming` method, branch in SendMessage |
| `ui/terminal.go` | Add `OnTextDelta`, update `OnText`/`OnThinking` for streaming mode, add streaming flag to Terminal |
| `config.go` | Add `ARTOO_STREAMING` env var |
| `main.go` | Pass streaming flag to Terminal constructor |
| `CONFIG.md` | Document `ARTOO_STREAMING` |

## Verification

1. `go build ./...` compiles cleanly.
2. `go test ./...` passes.
3. Manual test: run artoo with default config, send a message, verify text appears incrementally (word by word) instead of all at once.
4. Manual test: set `ARTOO_STREAMING=false`, verify the old behaviour (spinner, then full text).
5. Manual test: trigger a tool call during streaming (e.g. ask to list files), verify tool use works correctly after streamed text.
6. Manual test: verify the spinner still shows during tool execution (tool calls are not streamed).
