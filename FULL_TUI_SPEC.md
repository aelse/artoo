# Full TUI (Scrollable Output, Panels)

## Purpose

Replace the current line-by-line terminal output with a full terminal user interface built on Bubble Tea. The current UI writes directly to stdout with `fmt.Fprint` — output scrolls off screen, there is no way to scroll back, and the layout is a flat stream of text. A full TUI provides a scrollable conversation view, a fixed input area, and structured panels for tool output, making long sessions much more usable.

## Scope

- Replace the raw stdout output with a Bubble Tea application that owns the full terminal.
- Split the terminal into panels: a scrollable conversation view and a fixed input area.
- Support scrolling back through conversation history with keyboard navigation.
- Display tool calls and results in a collapsible/distinct section.
- Maintain the existing `agent.Callbacks` interface — the TUI implements it.
- Keep the existing simple terminal mode available as a fallback (e.g. for piped output or `ARTOO_TUI=false`).

## Current State

`ui/terminal.go` writes directly to stdout using `fmt.Fprint`/`fmt.Fprintf`. The `Terminal` struct implements `agent.Callbacks` and provides `ReadInput()` using a standalone Bubble Tea program for each input line (created and destroyed per prompt). There is no persistent TUI — each output line is printed and scrolls with the terminal buffer.

The existing Bubble Tea dependency (`bubbletea v1.3.10`, `bubbles v0.21.0`, `lipgloss v1.1.0`) is already available but only used for the ephemeral text input and spinner components.

## Changes Required

### 1. New file: `ui/tui.go` — Full TUI application

Create a Bubble Tea model that manages the entire terminal screen.

**Layout:**

```
┌─────────────────────────────────────────┐
│  Artoo Agent                   [scroll] │  ← Header (1 line)
├─────────────────────────────────────────┤
│                                         │
│  > What files are in this directory?    │  ← Conversation viewport
│                                         │     (scrollable)
│  Claude: Here are the files:            │
│                                         │
│  ┌─ Tool: list ───────────────────────┐ │
│  │ path: "."                          │ │
│  │ [OK] 12 files listed               │ │
│  └────────────────────────────────────┘ │
│                                         │
│  Claude: I found 12 files in the ...    │
│                                         │
├─────────────────────────────────────────┤
│  > _                                    │  ← Input area (fixed, 1-2 lines)
└─────────────────────────────────────────┘
```

**Model structure:**

```go
package ui

import (
    "fmt"
    "strings"

    "github.com/aelse/artoo/agent"
    "github.com/charmbracelet/bubbles/spinner"
    "github.com/charmbracelet/bubbles/textinput"
    "github.com/charmbracelet/bubbles/viewport"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

// Ensure TUI implements agent.Callbacks
var _ agent.Callbacks = (*TUI)(nil)

// message types for sending events from callbacks to the Bubble Tea event loop
type textDeltaMsg struct{ text string }
type toolCallMsg struct{ name, input string }
type toolResultMsg struct{ name, output string; isError bool }
type thinkingMsg struct{}
type thinkingDoneMsg struct{}
type textMsg struct{ text string }
type inputSubmittedMsg struct{ text string }

// TUI is the full terminal user interface.
type TUI struct {
    // Bubble Tea components
    viewport  viewport.Model
    input     textinput.Model
    spinner   spinner.Model

    // State
    lines     []string      // All rendered lines in the conversation
    ready     bool          // Whether the viewport has been initialized
    thinking  bool          // Whether we're waiting for a response
    width     int
    height    int

    // Channel for sending input back to the REPL
    inputCh   chan string

    // Program reference for sending messages from callbacks
    program   *tea.Program
}

// NewTUI creates a new TUI instance.
func NewTUI() *TUI {
    ti := textinput.New()
    ti.Placeholder = "Type a message..."
    ti.Focus()
    ti.Prompt = "> "

    sp := spinner.New()
    sp.Spinner = spinner.Points

    return &TUI{
        input:   ti,
        spinner: sp,
        inputCh: make(chan string),
    }
}
```

**Bubble Tea lifecycle methods:**

```go
func (t *TUI) Init() tea.Cmd {
    return tea.Batch(textinput.Blink, tea.WindowSize())
}

func (t *TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd

    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        t.width = msg.Width
        t.height = msg.Height
        headerHeight := 1
        inputHeight := 3 // input line + border
        viewportHeight := t.height - headerHeight - inputHeight

        if !t.ready {
            t.viewport = viewport.New(t.width, viewportHeight)
            t.viewport.SetContent(strings.Join(t.lines, "\n"))
            t.ready = true
        } else {
            t.viewport.Width = t.width
            t.viewport.Height = viewportHeight
        }

    case tea.KeyMsg:
        switch msg.Type {
        case tea.KeyCtrlC:
            return t, tea.Quit
        case tea.KeyEnter:
            if !t.thinking {
                value := t.input.Value()
                if value != "" {
                    t.appendLine(fmt.Sprintf("> %s", value))
                    t.input.Reset()
                    // Send input to the REPL goroutine
                    go func() { t.inputCh <- value }()
                }
            }
        case tea.KeyPgUp, tea.KeyPgDown:
            // Delegate to viewport for scrolling
            t.viewport, _ = t.viewport.Update(msg)
        }

    // Handle callback messages
    case textMsg:
        t.appendLine(fmt.Sprintf("Claude: %s", msg.text))
    case toolCallMsg:
        t.appendLine(fmt.Sprintf("┌─ Tool: %s", msg.name))
        t.appendLine(fmt.Sprintf("│ %s", msg.input))
    case toolResultMsg:
        status := "OK"
        if msg.isError {
            status = "ERROR"
        }
        t.appendLine(fmt.Sprintf("│ [%s]", status))
        t.appendLine("└────")
    case thinkingMsg:
        t.thinking = true
        return t, t.spinner.Tick
    case thinkingDoneMsg:
        t.thinking = false
    case spinner.TickMsg:
        if t.thinking {
            var cmd tea.Cmd
            t.spinner, cmd = t.spinner.Update(msg)
            cmds = append(cmds, cmd)
        }
    }

    // Update text input
    if !t.thinking {
        var cmd tea.Cmd
        t.input, cmd = t.input.Update(msg)
        cmds = append(cmds, cmd)
    }

    return t, tea.Batch(cmds...)
}

func (t *TUI) View() string {
    if !t.ready {
        return "Initializing..."
    }

    // Header
    header := titleStyle.Render("Artoo Agent")

    // Status indicator
    status := ""
    if t.thinking {
        status = t.spinner.View() + " Thinking..."
    }

    // Viewport (conversation)
    conversationView := t.viewport.View()

    // Input area
    inputView := t.input.View()
    if t.thinking {
        inputView = status
    }

    // Compose layout
    return fmt.Sprintf("%s\n%s\n%s", header, conversationView, inputView)
}

// appendLine adds a line to the conversation and updates the viewport.
func (t *TUI) appendLine(line string) {
    t.lines = append(t.lines, line)
    if t.ready {
        t.viewport.SetContent(strings.Join(t.lines, "\n"))
        t.viewport.GotoBottom()
    }
}
```

**Callbacks implementation (sends messages to Bubble Tea):**

```go
func (t *TUI) OnThinking() {
    t.program.Send(thinkingMsg{})
}

func (t *TUI) OnThinkingDone() {
    t.program.Send(thinkingDoneMsg{})
}

func (t *TUI) OnText(text string) {
    t.program.Send(textMsg{text: text})
}

func (t *TUI) OnToolCall(name string, input string) {
    t.program.Send(toolCallMsg{name: name, input: input})
}

func (t *TUI) OnToolResult(name string, output string, isError bool) {
    t.program.Send(toolResultMsg{name: name, output: output, isError: isError})
}

// ReadInput blocks until the user submits input via the TUI.
func (t *TUI) ReadInput() (string, error) {
    return <-t.inputCh, nil
}
```

**Run method to start the TUI:**

```go
// Run starts the TUI application. It blocks until the user quits.
// The provided callback is run in a goroutine and receives input via ReadInput().
func (t *TUI) Run(loop func(tui *TUI) error) error {
    p := tea.NewProgram(t, tea.WithAltScreen())
    t.program = p

    // Run the REPL loop in a background goroutine
    go func() {
        if err := loop(t); err != nil {
            // Signal quit
            p.Send(tea.Quit())
        }
    }()

    _, err := p.Run()
    return err
}
```

### 2. `ui/terminal.go` — Keep as simple mode

Rename nothing — the existing `Terminal` stays as-is for simple mode. Add a comment noting it's the non-TUI fallback.

### 3. `main.go` — Choose between TUI and simple mode

```go
func main() {
    ctx := context.Background()
    cfg := LoadConfig()

    client := anthropic.NewClient(
        option.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
    )

    a := agent.New(client, cfg.Agent)
    a.SetConversationConfig(cfg.Conversation)

    if cfg.UseTUI {
        tui := ui.NewTUI()
        err := tui.Run(func(t *ui.TUI) error {
            for {
                input, err := t.ReadInput()
                if err != nil {
                    return err
                }
                if input == "quit" || input == "exit" {
                    return nil
                }
                _, err = a.SendMessage(ctx, input, t)
                if err != nil {
                    t.OnText(fmt.Sprintf("Error: %v", err))
                }
            }
        })
        if err != nil {
            fmt.Fprintf(os.Stderr, "Error: %v\n", err)
            os.Exit(1)
        }
    } else {
        // Existing simple terminal mode
        term := ui.NewTerminal()
        term.PrintTitle()

        for {
            input, err := term.ReadInput()
            if err != nil {
                term.PrintError(err)
                break
            }
            if input == "" || input == "quit" || input == "exit" {
                break
            }
            _, err = a.SendMessage(ctx, input, term)
            if err != nil {
                term.PrintError(err)
            }
            fmt.Println()
        }
    }
}
```

### 4. `config.go` — Add TUI config

| Env Var | Default | Description |
|---------|---------|-------------|
| `ARTOO_TUI` | `true` | Use full TUI mode (false for simple line-by-line output) |

```go
type AppConfig struct {
    Agent        agent.Config
    Conversation conversation.Config
    Debug        bool
    UseTUI       bool
}
```

```go
func LoadConfig() AppConfig {
    return AppConfig{
        // ...
        UseTUI: getEnvBool("ARTOO_TUI", true),
    }
}
```

Auto-detect: if stdout is not a terminal (piped), force `UseTUI = false`:

```go
if !isTerminal(os.Stdout) {
    cfg.UseTUI = false
}
```

Use `golang.org/x/term` or `os.Stdout.Fd()` with `isatty` to detect.

### 5. Keyboard shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Submit input |
| `Ctrl+C` | Quit |
| `PgUp` / `PgDn` | Scroll conversation |
| `Up` / `Down` | Scroll conversation (when not in input) |
| `Home` / `End` | Jump to top/bottom of conversation |

These are handled in the `Update` method's `tea.KeyMsg` case.

### 6. Styling

Use the existing lipgloss styles from `terminal.go` (`titleStyle`, `claudeStyle`, `userStyle`, `debugStyle`, `errorStyle`). Move these to a shared file or keep in `terminal.go` and import from `tui.go` (both are in the `ui` package so they share access).

Tool call blocks should use a bordered box style:

```go
var toolBoxStyle = lipgloss.NewStyle().
    Border(lipgloss.RoundedBorder()).
    BorderForeground(lipgloss.Color("8")).
    Padding(0, 1)
```

### 7. Tests

**`ui/tui_test.go`:**

- `TestTUI_AppendLine` — append lines, verify they appear in the lines slice.
- `TestTUI_OnText` — simulate OnText callback via program.Send, verify line added.
- `TestTUI_OnToolCall` — simulate tool call, verify formatted output.
- `TestTUI_OnToolResult` — simulate tool result, verify status indicator.
- `TestTUI_View_Thinking` — verify spinner appears when thinking=true.
- `TestTUI_View_Ready` — verify layout includes header, viewport, and input.

Note: Full Bubble Tea integration tests are complex. Focus on unit-testing the state transitions (appendLine, message handling) rather than rendering. Use `teatest` from the Bubble Tea test utilities if needed.

**`ui/terminal_test.go`:**

- Existing tests continue to pass (simple mode unchanged).

## Dependencies

No new external dependencies. All components come from the existing Bubble Tea ecosystem:
- `github.com/charmbracelet/bubbletea` — already in go.mod
- `github.com/charmbracelet/bubbles/viewport` — part of bubbles (already in go.mod)
- `github.com/charmbracelet/bubbles/textinput` — already used
- `github.com/charmbracelet/bubbles/spinner` — already used
- `github.com/charmbracelet/lipgloss` — already used

Optional: `golang.org/x/term` for terminal detection (or use `os.Stdout.Fd()` with a syscall).

## Files Changed

| File | Change |
|------|--------|
| `ui/tui.go` | New file: TUI model, Bubble Tea lifecycle, Callbacks implementation |
| `ui/tui_test.go` | New file: tests for TUI |
| `ui/terminal.go` | No changes (kept as simple mode fallback) |
| `main.go` | Branch on UseTUI: run TUI or simple terminal |
| `config.go` | Add `UseTUI` to AppConfig, `ARTOO_TUI` env var |
| `CONFIG.md` | Document `ARTOO_TUI` and keyboard shortcuts |

## Verification

1. `go build ./...` compiles cleanly.
2. `go test ./...` passes.
3. Manual test: run artoo with default config, verify full TUI appears with scrollable conversation and fixed input.
4. Manual test: have a long conversation, scroll up with PgUp, verify old messages are visible.
5. Manual test: trigger tool calls, verify they appear in bordered boxes.
6. Manual test: set `ARTOO_TUI=false`, verify simple line-by-line output (existing behaviour).
7. Manual test: pipe output (`echo "hello" | artoo`), verify TUI is not used.
8. Manual test: resize the terminal window during a session, verify layout adapts.

## Migration Notes

- The simple `Terminal` mode is preserved as a fallback. No existing behaviour is removed.
- The `agent.Callbacks` interface is unchanged — TUI and Terminal both implement it.
- Users who prefer the simple output can set `ARTOO_TUI=false`.
- The TUI uses Bubble Tea's alt-screen mode, so it does not pollute the terminal scrollback buffer. When artoo exits, the terminal is restored to its previous state.
