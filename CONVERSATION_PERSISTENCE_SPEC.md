# Conversation Persistence / History

## Purpose

Allow conversations to be saved to disk and resumed later. Currently all conversation state is lost when the process exits. Persistence lets the user pick up where they left off, review past conversations, and maintain long-running working contexts across sessions.

## Scope

- Save conversation messages to a JSON file after each exchange.
- Restore a conversation from a file at startup.
- List and select from past conversations.
- Each conversation gets a unique ID and a human-readable title.
- Storage format is plain JSON files in a directory — no database.

## Current State

`conversation/conversation.go` holds messages in `[]anthropic.MessageParam` — an in-memory slice with no persistence. The `Conversation` struct has no concept of identity, timestamps, or storage.

`main.go` creates a fresh agent (and therefore a fresh conversation) on every startup.

## Changes Required

### 1. New file: `conversation/store.go` — Persistence layer

Create a `Store` that manages conversation files on disk.

**Storage directory:** `~/.artoo/conversations/` (created on first use).

**File format:** One JSON file per conversation, named `{id}.json`:

```json
{
  "id": "20260228-143052-a1b2",
  "title": "Debugging grep tool",
  "created_at": "2026-02-28T14:30:52Z",
  "updated_at": "2026-02-28T14:45:10Z",
  "messages": [ ... ]
}
```

The `messages` field holds the raw `[]anthropic.MessageParam` serialized as JSON. The anthropic SDK types implement `json.Marshaler`/`json.Unmarshaler`.

**Store interface:**

```go
type Store struct {
    dir string // e.g. ~/.artoo/conversations/
}

func NewStore(dir string) (*Store, error)
// Creates dir if it doesn't exist.

func (s *Store) Save(conv *Conversation) error
// Marshal conversation to JSON and write to {dir}/{id}.json.
// Uses atomic write (write to temp file, then rename) to avoid corruption.

func (s *Store) Load(id string) (*Conversation, error)
// Read {dir}/{id}.json and unmarshal into a Conversation.

func (s *Store) List() ([]ConversationSummary, error)
// Read all JSON files in dir, parse only id/title/updated_at (not messages),
// return sorted by updated_at descending.

func (s *Store) Delete(id string) error
// Remove {dir}/{id}.json.
```

**ConversationSummary:**

```go
type ConversationSummary struct {
    ID        string
    Title     string
    UpdatedAt time.Time
}
```

### 2. `conversation/conversation.go` — Add identity fields

Add fields to `Conversation` for persistence:

```go
type Conversation struct {
    ID               string                     `json:"id"`
    Title            string                     `json:"title"`
    CreatedAt        time.Time                  `json:"created_at"`
    UpdatedAt        time.Time                  `json:"updated_at"`
    messages         []anthropic.MessageParam   `json:"messages"`
    config           Config                     `json:"-"`
    totalInputTokens int                        `json:"-"`
}
```

Add a method to generate a conversation ID:

```go
func generateID() string {
    now := time.Now()
    // Format: YYYYMMDD-HHMMSS-XXXX where XXXX is random hex
    b := make([]byte, 2)
    rand.Read(b)
    return fmt.Sprintf("%s-%s", now.Format("20060102-150405"), hex.EncodeToString(b))
}
```

Update `New()` and `NewWithConfig()` to set `ID` and `CreatedAt`. Update `Append()` to set `UpdatedAt`.

Add a method to set the title:

```go
func (c *Conversation) SetTitle(title string) {
    c.Title = title
}
```

The title can be auto-generated from the first user message (first 60 characters, truncated at a word boundary).

### 3. `config.go` — Add storage directory config

Add to `AppConfig`:

```go
type AppConfig struct {
    Agent        agent.Config
    Conversation conversation.Config
    Debug        bool
    StorageDir   string // directory for conversation files
}
```

Load from environment:

| Env Var | Default | Description |
|---------|---------|-------------|
| `ARTOO_STORAGE_DIR` | `~/.artoo/conversations` | Directory for saved conversations |

### 4. `agent/agent.go` — Auto-save after each exchange

Add a `Store` field to `Agent`:

```go
type Agent struct {
    // ... existing fields ...
    store *conversation.Store // nil if persistence disabled
}
```

Add a `SetStore(store *conversation.Store)` method.

At the end of `SendMessage`, after the tool-use loop completes, auto-save:

```go
if a.store != nil {
    // Auto-title from first user message if no title set
    if a.conversation.Title == "" {
        a.conversation.SetTitle(autoTitle(text))
    }
    if err := a.store.Save(a.conversation); err != nil {
        // Log but don't fail the conversation
        cb.OnToolResult("_system", "Failed to save conversation: "+err.Error(), true)
    }
}
```

Add `autoTitle(text string) string` helper that takes the first 60 chars of text, truncated at a word boundary.

### 5. `agent/config.go` — Expose conversation for loading

Add method to `Agent`:

```go
func (a *Agent) LoadConversation(conv *conversation.Conversation) {
    a.conversation = conv
}

func (a *Agent) ConversationID() string {
    return a.conversation.ID
}
```

### 6. `main.go` — Resume and list commands

Update the REPL to support resume flow:

**At startup:**
- If `ARTOO_RESUME` env var is set to a conversation ID, load that conversation and continue.
- Otherwise, start a fresh conversation.

**In the REPL loop, add special commands:**
- `/history` — list recent conversations (ID, title, date). Use `store.List()`.
- `/resume <id>` — load a conversation and continue from where it left off.
- `/new` — start a fresh conversation (abandon current).

These commands are handled before passing input to `SendMessage`:

```go
switch {
case input == "/history":
    summaries, _ := store.List()
    for _, s := range summaries {
        fmt.Fprintf(os.Stdout, "  %s  %s  %s\n", s.ID, s.UpdatedAt.Format("2006-01-02 15:04"), s.Title)
    }
    continue
case strings.HasPrefix(input, "/resume "):
    id := strings.TrimPrefix(input, "/resume ")
    conv, err := store.Load(id)
    // ... load into agent ...
    continue
case input == "/new":
    a = agent.New(client, cfg.Agent)
    // ... reinitialize ...
    continue
}
```

### 7. `ui/terminal.go` — Format history output

Add methods for displaying conversation history in the terminal:

```go
func (t *Terminal) PrintHistory(summaries []conversation.ConversationSummary)
func (t *Terminal) PrintResumed(id string, messageCount int)
```

### 8. Tests

**`conversation/store_test.go`:**
- `TestStore_SaveAndLoad` — save a conversation, load it back, verify messages match.
- `TestStore_List` — save multiple conversations, verify list returns them sorted by date.
- `TestStore_Delete` — save, delete, verify gone.
- `TestStore_AtomicWrite` — verify temp file + rename pattern (no partial writes).
- `TestStore_LoadMissing` — load a non-existent ID, verify error.
- `TestGenerateID` — verify format and uniqueness.
- `TestAutoTitle` — verify truncation at word boundary.

**`conversation/conversation_test.go`:**
- `TestConversation_IDGenerated` — new conversation has a non-empty ID.
- `TestConversation_UpdatedAtOnAppend` — verify UpdatedAt changes on Append.

## Files Changed

| File | Change |
|------|--------|
| `conversation/store.go` | New file: Store, ConversationSummary, persistence logic |
| `conversation/store_test.go` | New file: tests for Store |
| `conversation/conversation.go` | Add ID, Title, CreatedAt, UpdatedAt fields; generateID(); SetTitle() |
| `conversation/conversation_test.go` | Add tests for new fields |
| `agent/agent.go` | Add store field, auto-save in SendMessage, autoTitle helper |
| `agent/config.go` | Add LoadConversation, ConversationID methods |
| `config.go` | Add StorageDir to AppConfig, ARTOO_STORAGE_DIR env var |
| `main.go` | Add /history, /resume, /new commands; initialize store; support ARTOO_RESUME |
| `ui/terminal.go` | Add PrintHistory, PrintResumed methods |
| `CONFIG.md` | Document new env vars and commands |

## Verification

1. `go build ./...` compiles cleanly.
2. `go test ./...` passes all new and existing tests.
3. Manual test: start artoo, have a conversation, exit. Check `~/.artoo/conversations/` has a JSON file. Start artoo again, type `/history`, see the conversation listed. Type `/resume <id>`, verify conversation continues.
4. Manual test: verify auto-save doesn't crash if `~/.artoo/conversations/` is read-only (graceful degradation).
5. Manual test: verify a fresh start with no prior conversations works as before.
