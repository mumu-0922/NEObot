# Chat/Agent Mode Frontend Contract

## Scenario: Persisted composer runtime mode

### 1. Scope / Trigger

Apply this contract when changing the composer mode selector, Conversation
config normalization, model Tool capability display, or server-stream config.

### 2. Signatures

```ts
type ChatToolMode = "chat" | "agent";

interface SessionConfig { toolMode?: ChatToolMode }
interface ChatConfig { toolMode: ChatToolMode }
```

`MessageInput` receives requested `toolMode`, resolved `effectiveToolMode`,
`canSelectAgentMode`, and `onToolModeChange`.

### 3. Contracts

- Missing legacy values normalize to `agent`; invalid stored/server values are
  dropped at the Session boundary and the application default remains Agent.
- Requested mode is stored per Conversation. The browser default is updated so
  a newly created Conversation starts with the current choice.
- Model capability precedence is model override, provider default, custom/base
  metadata, then unknown. Unknown retains the requested Agent selection.
- Explicit unsupported capability keeps the requested value but renders Chat as
  effective and disables selecting Agent until the model changes.
- The composer exposes one Chat/Agent capsule. It does not expose MCP or run an
  MCP preflight before send; Connector management stays in the Sidebar Tools page.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| `toolMode` missing | requested/effective Agent when the model supports Tools |
| invalid `toolMode` from storage/API | ignore it; never widen another field |
| Agent plus explicit Tool unsupported | effective Chat with visible reason |
| capability metadata unresolved | keep Agent selectable; Backend remains authoritative |
| Conversation config save fails | show bounded composer error; do not claim persistence |

### 5. Good / Base / Bad Cases

- **Good**: select Chat, reload, see Chat, switch to a Tool-capable model and
  explicitly select Agent.
- **Base**: open a legacy Conversation and retain existing Agent behavior.
- **Bad**: clear installed Tools to simulate Chat, expose MCP in the composer,
  or mutate effective downgrade into the stored requested choice.

### 6. Tests Required

- Pure normalization/capability/effective-mode precedence tests.
- Store selection and per-Conversation persistence tests.
- Server DTO normalization and composer/ChatApp composition tests proving MCP
  control/preflight removal.
- Frontend format, lint, type-check, full Vitest, and production build.

### 7. Wrong vs Correct

#### Wrong

```ts
if (chatMode) selectedServers = [];
await api.chat.preflightMcp(...);
```

#### Correct

```ts
const effectiveToolMode = resolveEffectiveChatToolMode(
  conversation.toolMode ?? "agent",
  modelToolCapability,
);
```

Persist intent once; let Backend physically construct the allowed Runtime Tools.

## Scenario: Render authenticated Agent artifacts

### Contracts

- A server attachment is an Agent artifact only when `source=server`, `fileId`
  is present, and `purpose=output`. Ordinary user input/image/Knowledge
  attachments retain their existing rendering.
- Render the file name, MIME type, bounded size label, download state, and a
  keyboard-accessible Download button. Download through the authenticated File
  API with `disposition=attachment`; never navigate to the DTO URL directly.
- Save the returned Blob through the shared short-lived object-URL helper,
  abort an in-flight component-owned request on unmount, and expose progress or
  failure through `aria-live`. A 404/deleted File is a visible failure, never a
  successful download.
- DTO mapping must retain `source`, `fileId`, `size`, `sha256`, and `purpose`
  across the stream completion and later Conversation reload paths.

### Tests Required

- DTO reload preserves output metadata and the card remains selected only for
  `purpose=output`.
- The download helper calls authenticated `downloadFileContent`, saves only a
  successful Blob, propagates 404/failure, and never saves ordinary input.
