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

resolveToolRoundCapabilityForMode(
    ctx, provider, resolution, modelRef, requestedMode,
) ToolCapabilityStatus
```

The public Conversation `config.toolMode` field is `"chat" | "agent"`.
Stream request config is a generation snapshot, not mode authority.

### 3. Contracts

- Read mode from the authenticated persisted Conversation metadata. Missing or
  invalid values mean Agent for compatibility; never trust a conflicting stream
  request value.
- Resolve the persisted mode before capability admission. Chat uses the existing
  non-blocking cache/probe path. Agent reuses the same singleflight probe state
  and waits only for the bounded provider probe, never for its best-effort cache
  write.
- Only an explicit provider/model override, a non-Tool adapter, a cached
  `unsupported`, or an explicit probe incompatibility may downgrade Agent to
  Chat. A cached or freshly probed transient/inconclusive `unknown` preserves
  the adapter-native Agent path so the real first Tool round remains authority.
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
| persisted Agent + cache miss, supported probe | wait once; same request enters Agent Tool Registry |
| two Agent requests miss the same provider/model key | one shared probe; both consume its result |
| persisted Agent + transient/cached unknown | retain native Agent admission; do not write `toolMode=chat` |
| persisted Agent + confirmed unsupported | Chat; ordinary answer, no 409 |
| persisted Chat + cache miss | return immediately; background probe only |
| Chat + Knowledge/Memory/Web selected | retain the applicable retrieval path |
| Agent + selected MCP unavailable | existing server-authoritative admission error |

### 5. Good / Base / Bad Cases

- **Good**: a first Agent turn waits for one supported probe, then receives File,
  Terminal, Goal, Skill, and applicable MCP Tools in that same request.
- **Base**: a legacy Conversation with a Tool-capable provider keeps current Agent Tools.
- **Bad**: use request config as authority, turn `unknown` into `unsupported`,
  prepare MCP before reading mode, or leave Tool definitions present and rely on
  a system prompt.

### 6. Tests Required

- Unit matrix for missing, valid, invalid, confirmed capability downgrade,
  supported first probe, shared Agent waiters, transient unknown, and Chat
  non-blocking behavior.
- Handler integration with stored Chat, conflicting request Agent, selected
  unreachable MCP, no Tool round, and persisted effective metadata. The first
  Agent cache-miss regression must execute `file_write -> file_read ->
  publish_file` and assert that the first real Agent round contains the complete
  workspace Tool set.
- Existing Knowledge, Memory, Web, Skill, Goal, MCP, cancellation, and durable
  event suites plus `go vet ./...` and `go test ./...`.

### 7. Wrong vs Correct

#### Wrong

```go
mode := requestedChatToolMode(request.Config)
capability := resolveToolRoundCapability(ctx, provider, resolution, modelRef)
mode = effectiveChatToolMode(conversation.Metadata, capability == ToolCapabilitySupported)
prepared, _ := mcp.PrepareRun(...)
```

#### Correct

```go
conversation, _ := service.GetConversation(ctx, conversationID)
requested := requestedChatToolMode(conversation.Metadata)
capability := resolveToolRoundCapabilityForMode(
    ctx, provider, resolution, modelRef, requested,
)
mode := effectiveChatToolMode(
    conversation.Metadata,
    capability == ToolCapabilitySupported,
)
if mode == chatToolModeAgent { prepared, _ = mcp.PrepareRun(...) }
```
