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

const PROVIDER_STREAM_INTERRUPTED_CODE = "PROVIDER_STREAM_INTERRUPTED";
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
- The composer shows one icon-only Tools wrench without adjacent inherited or
  server-name chips. Its enabled styling and status tooltip retain the current
  server/Tool summary; full status, auth requirements, and unavailable state
  remain visible in the Tools dialog before send.
- The Sidebar exposes a first-class **Tools** entry backed by
  `?panel=tools`. Its page lists authorized MCP Server definitions, supports
  private Server lifecycle/authorization, and edits the active Conversation
  selection when one exists. Listing and Server management must still work
  when no Conversation exists; selection controls then remain disabled.
- Reuse the same server-authoritative MCP client and management behavior for
  the top-level page and composer control. Do not create a browser-owned MCP
  registry or a second selection store merely to support panel navigation.
- The top-level page exposes **Installed | MCP Marketplace** tabs. Marketplace
  search/detail always uses the typed `/v1/mcp/marketplace/*` API and never
  calls or scrapes LobeHub from the browser.
- Marketplace cards/details show source, exact version, connection type,
  install compatibility, Tool preview, and display-only trust signals. The UI
  enables install only when the backend returns `compatibility: "installable"`;
  it must not infer authority from connection type. This permits public HTTPS
  Streamable HTTP and exact approved stdio artifacts while keeping SSE and
  unmatched package/Docker/Git options blocked.
- Marketplace navigation exposes a curated primary-category rail backed by
  server-returned category counts and backend category filtering. Cards and
  details render only normalized HTTPS or short emoji/text icons, use
  `no-referrer` for remote images, and retain a local fallback.
- Installed Server cards use the same shared icon renderer as Marketplace
  cards. They render only the optional normalized Server DTO `icon`; missing or
  failed remote images fall back to the local generic MCP glyph without
  changing selection or trust state.
- Marketplace search uses a monotonic request ID in addition to AbortSignal.
  Only the latest request may replace items, totals, loading, or error state;
  an older failure must never leave a false unavailable banner over newer
  successful results.
- `Install and enable` sends only identifier/version plus the current
  Conversation/revision. After install it reloads authoritative Servers and
  selection. API-key and OAuth items continue through the existing credential
  and OAuth UI rather than collecting secrets in Marketplace state.
- A blocked send may focus the Tools control and offer an explicit
  disable-all-and-continue action. Do not add per-call approval dialogs.
- Credential fields are transient component state, cleared after submission,
  and never persisted/exported. OAuth authorization URLs must parse as HTTPS
  before navigation.
- The process trace maps only backend-redacted call summaries and states.
  Results remain collapsed by default. Manual retry is allowed only if the
  backend exposes a trusted idempotent read retry affordance; never infer it
  from remote annotations.
- A failed assistant with `PROVIDER_STREAM_INTERRUPTED` renders a localized
  Provider-interruption notice while retaining the partial answer. It is not
  presented as an MCP Tool failure. For persisted rows created before this
  stable code existed, non-empty failed `PROVIDER_ERROR` content receives the
  same notice; an empty generic Provider failure remains generic.
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
| Create succeeds but response normalization or validation fails | reload the authoritative Server list so the persisted draft remains visible; show the bounded error |
| Server needs auth/unavailable | visible state; send remains blocked until explicit change |
| OAuth URL is not valid HTTPS | localized error; do not navigate |
| Selection revision is stale | show save failure and reload authoritative state |
| `outcome_unknown` timeline event | terminal warning state; no one-click retry |
| Legacy Plugin fields load/import | strip recursively; persist no Plugin or inferred MCP state |
| Marketplace disabled or unconfigured | show a bounded configuration state; Installed remains fully usable |
| Search/detail upstream failure | show a retryable Marketplace-only error; keep installed/selection state unchanged |
| Older search fails after a newer search succeeds | ignore the stale completion; keep the newer results with no false error banner |
| Item is SSE or unmatched stdio/command-only | show compatibility reason; no install request is emitted |
| Backend marks exact approved stdio deployment installable | send only identifier/version; never receive or execute command metadata |
| Install validation/selection step fails after draft creation | reload installed Servers so the recoverable draft remains visible |
| Installed Server icon is missing or its HTTPS image fails | show the local generic MCP fallback; keep the card usable |
| Provider stream interrupts after partial answer content | keep the content and show the localized Provider-interruption notice; do not blame or retry MCP Tools |

### 5. Good / Base / Bad Cases

- **Good**: a user opens Tools, enables one granted server, disables one Tool,
  saves the returned revision, sees only the active wrench in the composer,
  sends, and sees queued-to-succeeded timeline.
- **Base**: a conversation without Workspace or selection shows zero enabled
  Tools and sends ordinary chat.
- **Bad**: hydrate MCP selection from `activePlugins`, retain a credential in
  Zustand, or execute a Tool from the browser.

### 6. Tests Required

- API-client URL/body/response mapping and local-mode fail-closed behavior.
- Tools control load, inherited/custom/explicit-empty selection, Tool disable,
  private draft validation, credential submission, OAuth URL rejection,
  unavailable/auth states, and disable-all recovery.
- Sidebar Tools entry, `?panel=tools` URL round-trip, top-level page
  composition, and Server listing without a current Conversation.
- Marketplace tab/search/detail, category filtering/counts, remote-icon
  fallback, Installed shared-icon/fallback rendering, compatibility labels,
  disabled/unconfigured behavior,
  backend-approved remote/stdio installability, authoritative install payload,
  latest-request race fencing, no command fields, install recovery, and optional
  Conversation enablement with revision.
- Timeline mapping for every state including `outcome_unknown`, redacted
  summaries, and cancellation.
- Generation-error wiring for current `PROVIDER_STREAM_INTERRUPTED` plus the
  non-empty legacy `PROVIDER_ERROR` compatibility path in every locale.
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
