# Chat/Agent Runtime Policy Contract

## Scenario: Server-authoritative Conversation mode

### 1. Scope / Trigger

Apply this contract when changing ordinary Chat Agent Tool admission, Skill/MCP/
Goal initialization, Conversation config, or Tool-capability downgrade behavior.

### 2. Signatures

```go
type chatToolMode string

const (
    chatToolModeChat  chatToolMode = "chat"
    chatToolModeAgent chatToolMode = "agent"
)
```

The public Conversation `config.toolMode` field is `"chat" | "agent"`.
Stream request config is a generation snapshot, not mode authority.

### 3. Contracts

- Read mode from the authenticated persisted Conversation metadata. Missing or
  invalid values mean Agent for compatibility; never trust a conflicting stream
  request value.
- Effective Agent requires native Tool-round capability. Unsupported or unknown
  capability downgrades that run to Chat without MCP/Skill admission conflicts.
- Chat mode physically skips MCP `PrepareRun`, local Skill catalog/runtime, Goal,
  File, Terminal, and Job definitions. It does not merely prompt the model not to call.
- Knowledge, Memory, external/model Web Search, and non-Tool image routing remain
  independent and available in Chat mode.
- Agent mode reuses the existing immutable MCP snapshot, `local_direct` Skill
  Runtime, Goal, scheduler, durable events, and same-model continuation. It does
  not introduce Subagents.
- Assistant metadata records both `requestedToolMode` and effective `toolMode`
  through finalization for replay and diagnosis.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| persisted mode missing/invalid + Tool capable | Agent compatibility behavior |
| persisted Chat + request claims Agent | Chat; no MCP/Skill/Goal preparation |
| persisted Agent + Tool unsupported/unknown | Chat; ordinary answer, no 409 |
| Chat + Knowledge/Memory/Web selected | retain the applicable retrieval path |
| Agent + selected MCP unavailable | existing server-authoritative admission error |

### 5. Good / Base / Bad Cases

- **Good**: stored Chat ignores a forged Agent request while Search/Memory still work.
- **Base**: a legacy Conversation with a Tool-capable provider keeps current Agent Tools.
- **Bad**: use request config as authority, prepare MCP before reading mode, or leave
  Tool definitions present and rely on a system prompt.

### 6. Tests Required

- Unit matrix for missing, valid, invalid, and capability-downgraded mode.
- Handler integration with stored Chat, conflicting request Agent, selected
  unreachable MCP, no Tool round, and persisted effective metadata.
- Existing Knowledge, Memory, Web, Skill, Goal, MCP, cancellation, and durable
  event suites plus `go vet ./...` and `go test ./...`.

### 7. Wrong vs Correct

#### Wrong

```go
mode := requestedChatToolMode(request.Config)
prepared, _ := mcp.PrepareRun(...)
```

#### Correct

```go
conversation, _ := service.GetConversation(ctx, conversationID)
mode := effectiveChatToolMode(conversation.Metadata, toolRoundCapable)
if mode == chatToolModeAgent { prepared, _ = mcp.PrepareRun(...) }
```
