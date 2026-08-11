# MCP Tools Frontend Contract

## Scenario: Conversation-level Tools UI

### 1. Scope / Trigger

Apply this contract when changing `McpToolsControl`, MCP frontend types, the
`mcpApi` client, composer chips/blocked-send recovery, timeline rendering, or
browser persistence migration. The product label is **Tools**; MCP is the
protocol. Assistants and Skills remain unchanged.

### 2. Signatures

```ts
interface NeoChatApiClient { mcp: McpApi }

type McpSelectionMode = "inherit" | "custom";
type McpCallStatus =
  | "queued" | "running" | "succeeded" | "failed"
  | "canceled" | "outcome_unknown";
```

The client maps only to `/v1/mcp/*` routes defined in
`mm-chat/docs/contracts/mcp-tools-api.md`. Server stream `tool.call.updated`
events carry bounded MCP timeline updates.

### 3. Contracts

- Server mode lists server-authorized definitions and selection. Browser
  Workspace state, local cache, server/tool names, and Tool annotations are not
  authorization.
- `inherit` means Workspace defaults. `custom` plus `servers: []` means all
  Tools are explicitly disabled. Preserve backend revision tokens on writes.
- The composer shows one Tools control and enabled server/Tool summary. Status,
  auth requirements, and unavailable state remain visible before send.
- A blocked send may focus the Tools control and offer an explicit
  disable-all-and-continue action. Do not add per-call approval dialogs.
- Credential fields are transient component state, cleared after submission,
  and never persisted/exported. OAuth authorization URLs must parse as HTTPS
  before navigation.
- The process trace maps only backend-redacted call summaries and states.
  Results remain collapsed by default. Manual retry is allowed only if the
  backend exposes a trusted idempotent read retry affordance; never infer it
  from remote annotations.
- Local mode has no Plugin or MCP execution fallback.
- Storage version 5 recursively removes `activePlugins`, `installedPlugins`,
  `pluginConfigs`, `marketPlugins`, and `marketPluginsTimestamp` without
  mapping them to MCP. Entity/import normalization also drops retired
  `activePlugins`.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| API mode is local or MCP config disabled | Tools control disabled/hidden with no legacy call |
| Server list/selection load fails | bounded localized error; no stale authority expansion |
| Server needs auth/unavailable | visible state; send remains blocked until explicit change |
| OAuth URL is not valid HTTPS | localized error; do not navigate |
| Selection revision is stale | show save failure and reload authoritative state |
| `outcome_unknown` timeline event | terminal warning state; no one-click retry |
| Legacy Plugin fields load/import | strip recursively; persist no Plugin or inferred MCP state |

### 5. Good / Base / Bad Cases

- **Good**: a user opens Tools, enables one granted server, disables one Tool,
  saves the returned revision, sends, and sees queued-to-succeeded timeline.
- **Base**: a conversation without Workspace or selection shows zero enabled
  Tools and sends ordinary chat.
- **Bad**: hydrate MCP selection from `activePlugins`, retain a credential in
  Zustand, or execute a Tool from the browser.

### 6. Tests Required

- API-client URL/body/response mapping and local-mode fail-closed behavior.
- Tools control load, inherited/custom/explicit-empty selection, Tool disable,
  private draft validation, credential submission, OAuth URL rejection,
  unavailable/auth states, and disable-all recovery.
- Timeline mapping for every state including `outcome_unknown`, redacted
  summaries, and cancellation.
- Storage/entity/import tests that remove all retired Plugin keys without
  modifying Assistants, Skills, security metadata, or unrelated settings.
- Run focused Vitest files plus frontend `typecheck`; add full frontend gates
  only when the broader release gate is requested.

### 7. Wrong vs Correct

#### Wrong

```ts
const enabled = persisted.activePlugins.map((id) => ({
  ref: { source: "private", id },
}));
```

#### Correct

```ts
const selection = await api.mcp.getConversationSelection(conversationId);
// Render and mutate only this server-authoritative DTO and its revision.
```
