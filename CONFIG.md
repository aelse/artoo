# Configuration

Artoo is configured via environment variables. No configuration files or command-line flags are needed.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ARTOO_MODEL` | `claude-sonnet-4-20250514` | Claude model to use for API calls |
| `ARTOO_MAX_TOKENS` | `8192` | Maximum tokens per API response |
| `ARTOO_MAX_CONTEXT_TOKENS` | `180000` | Maximum conversation context window (Sonnet's 200k limit with headroom) |
| `ARTOO_TOOL_RESULT_MAX_CHARS` | `10000` | Maximum characters for tool outputs before truncation |
| `ARTOO_DEBUG` | `false` | Enable debug output |

## Examples

### Use Opus model with higher token limits

```bash
export ARTOO_MODEL=claude-opus-4-20250805
export ARTOO_MAX_TOKENS=16384
export ARTOO_MAX_CONTEXT_TOKENS=200000
./artoo
```

### Enable debug output

```bash
export ARTOO_DEBUG=true
./artoo
```

### Use custom context window for long sessions

```bash
export ARTOO_MAX_CONTEXT_TOKENS=250000
export ARTOO_TOOL_RESULT_MAX_CHARS=5000  # More aggressive truncation
./artoo
```

### Set all options

```bash
export ANTHROPIC_API_KEY=sk-...
export ARTOO_MODEL=claude-opus-4-20250805
export ARTOO_MAX_TOKENS=16384
export ARTOO_MAX_CONTEXT_TOKENS=200000
export ARTOO_TOOL_RESULT_MAX_CHARS=20000
export ARTOO_DEBUG=true
./artoo
```

## Boolean Values

The `ARTOO_DEBUG` variable accepts these true values:
- `1`, `true`, `True`, `TRUE`
- `yes`, `Yes`, `YES`
- `on`, `On`, `ON`

And these false values:
- `0`, `false`, `False`, `FALSE`
- `no`, `No`, `NO`
- `off`, `Off`, `OFF`

Case-insensitive. Invalid values default to `false`.

## Configuration Behavior

- **Unset variables** use their default values
- **Invalid values** for integers are logged and default values are used
- **Configuration is loaded once** at startup
- **No config file** support (environment variables only)
- **No command-line flags** (environment variables only)

## Required Environment Variable

- `ANTHROPIC_API_KEY` - Your Claude API key (not managed by artoo)

## How Configuration Works

When artoo starts, it:

1. Loads all configuration from environment variables
2. Uses defaults for any unset variables
3. Passes agent config to the `agent.Agent` constructor
4. Passes conversation config to the `conversation.Conversation`
5. Prints debug info if `ARTOO_DEBUG=true`

All configuration is read at startup and cannot be changed without restarting artoo.

## Configuration in Code

Configuration is defined in `config.go` and loaded via `LoadConfig()`:

```go
cfg := LoadConfig()

// Access agent config
model := cfg.Agent.Model
maxTokens := cfg.Agent.MaxTokens

// Access conversation config
maxContext := cfg.Conversation.MaxContextTokens
toolMaxChars := cfg.Conversation.ToolResultMaxChars

// Access debug flag
debug := cfg.Debug
```

The configuration is applied in `main.go`:

```go
a := agent.New(client, cfg.Agent)
a.SetConversationConfig(cfg.Conversation)
```
