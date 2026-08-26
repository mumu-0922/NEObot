# MCP Tools Frontend Contract

## Scenario: Sidebar Tools and Connector management UI

### 1. Scope / Trigger

Apply this contract when changing `McpToolsControl`, MCP frontend types, the
`mcpApi` client, Sidebar Tools management, timeline rendering, or browser
persistence migration. The product label is **Tools/Connectors**; MCP is the
protocol. Assistants and Skills remain unchanged.

### 2. Signatures

```ts
interface NeoChatApiClient { mcp: McpApi }

type McpSelectionMode = "inherit" | "custom";
type McpCallStatus =
  | "queued" | "running" | "succeeded" | "failed"
  | "canceled" | "outcome_unknown";
type ToolCallMode = "mcp" | "local_direct";
type ToolCallClassification =
  | "read" | "write" | "unknown" | "execute";

const PROVIDER_STREAM_INTERRUPTED_CODE = "PROVIDER_STREAM_INTERRUPTED";
```

The MCP client maps only to `/v1/mcp/*` routes defined in
`mm-chat/docs/contracts/mcp-tools-api.md`. For an admitted timeline canary, the
Chat stream carries persisted timeline facts through `agent.event`; the
compatibility `tool.call.updated` transport is accepted only for the
non-canary/control rollback path.

### 3. Contracts

- Server mode lists server-authorized definitions and Conversation selection. Browser
  Workspace state, local cache, server/tool names, and Tool annotations are not
  authorization.
- `inherit` means Workspace defaults. `custom` plus `servers: []` means all
  Tools are explicitly disabled. Preserve backend revision tokens on writes.
- The composer renders one compact MCP picker beside the other Chat/Agent
  controls. It must not render the full `McpToolsControl` management surface or
  preflight MCP before every send. The picker loads only the exact current
  Conversation selection, lists authorized inventory, and performs
  revision-bound selection writes. Full status, credentials, installation,
  diagnostics, and lifecycle controls remain in Sidebar Tools; Backend prepares
  selected Connectors only in Agent mode.
- The Sidebar exposes a first-class **Tools** entry backed by
  `?panel=tools`. Its page lists authorized MCP Server definitions, supports
  private Server lifecycle/authorization, and never edits the active
  Conversation selection. Per-Conversation selection belongs exclusively to
  the compact composer picker. Listing and Server management must still work
  when no Conversation exists.
- Reuse the same server-authoritative MCP client and management behavior for
  top-level and embedded management surfaces. Do not create a browser-owned MCP
  registry or a second selection store merely to support panel navigation.
- The top-level page exposes **Installed | MCP Marketplace** tabs. Marketplace
  search/detail always uses the typed `/v1/mcp/marketplace/*` API and never
  calls or scrapes LobeHub from the browser.
- Server-list/detail DTOs expose bounded `canManage`/`canInstall` capabilities
  derived by Backend from the authenticated user. Only the deployment
  administrator sees create/delete/validate/credential/OAuth/install controls.
  Ordinary users may inspect Installed inventory while selection remains in
  the composer picker; frontend capability checks shape UX but never replace
  Backend authorization.
- Marketplace cards/details show source, exact version, connection type,
  install compatibility, Tool preview, and display-only trust signals. The UI
  enables `installable`, plus `needs_configuration` only when the backend also
  declares `installMode: header|runner_env` and bounded secret fields. It must
  not infer authority from connection type. This permits public HTTPS
  Streamable HTTP and exact administrator-installable npm stdio artifacts while
  keeping SSE and Shell/Docker/Git/URL-package options blocked.
- Marketplace detail consumes the authenticated Backend `installed` flag for
  the exact current `provider + identifier + version`. Installed items disable
  the install action and direct management to the Installed tab. When at least
  one deployment is installable, hide incompatible duplicate alternatives from
  the connection picker; compatibility details are not an execution tutorial.
- Marketplace navigation exposes a curated primary-category rail backed by
  server-returned category counts and backend category filtering. Cards and
  details render only normalized HTTPS or short emoji/text icons, use
  `no-referrer` for remote images, and retain a local fallback.
- The local icon fallback renders underneath a remote image from the first
  paint; the remote image overlays it only when usable. Do not wait for an
  image error before rendering the fallback, because slow/transparent remote
  responses otherwise appear as an empty colored tile.
- Installed Server cards use the same shared icon renderer as Marketplace
  cards. They render only the optional normalized Server DTO `icon`; missing or
  failed remote images fall back to the local generic MCP glyph without
  changing inventory or trust state.
- Installed Server cards stay compact by default: the Tool count is an
  accessible expand/collapse button in the Server summary row, Tool details and
  the description render only while expanded. Hide the non-informative
  `unknown` classification badge while preserving meaningful `read`/`write`
  badges. Tool rows are inventory diagnostics, not selection controls.
- Marketplace search uses a monotonic request ID in addition to AbortSignal.
  Only the latest request may replace items, totals, loading, or error state;
  an older failure must never leave a false unavailable banner over newer
  successful results.
- Marketplace errors remain scoped to the operation that failed. Initial
  search errors may use the Marketplace-level banner; item-detail errors keep
  the loaded result list and expose a detail retry, while install errors stay
  inside the detail surface.
- Marketplace browsing uses server-authoritative pages of 20. A reset search
  replaces page 1; a near-bottom sentinel appends exactly the next page with
  identifier deduplication and at most one request in flight. The UI reports
  loaded/total counts and retains a manual load/retry action. Next-page failure
  preserves current cards, while search/category changes abort and generation-
  fence older pagination work.
- `Install` sends identifier/version, an exact backend-issued deployment hash,
  and transient values for backend-declared secret fields. It always sets
  `enableForConversation: false` and does not carry a Conversation selection
  revision. Secret values exist only in component state, clear after submission,
  and never enter a URL or browser persistence. OAuth installs create a
  recoverable `needs_auth` Server and then open the validated authorization URL
  without a Conversation ID; Runner environment drafts can be reconfigured from
  the Installed tab using only backend-returned non-secret field names.
- Required secret inputs render inside the currently selected deployment card,
  with the first field focused and a visible completion hint beside the
  disabled install action. A required configuration field must never be hidden
  below an independently scrolling deployment list.
- Agent-mode MCP admission errors remain bounded chat errors and direct the user
  to Sidebar Tools when configuration is required. Chat mode skips MCP admission
  entirely. Do not invent browser-owned MCP approval dialogs; only render a
  backend-issued approval presentation for a Tool/risk combination whose
  runtime policy explicitly permits it.
- Credential fields are transient component state, cleared after submission,
  and never persisted/exported. OAuth authorization URLs must parse as HTTPS
  before navigation.
- Private remote creation collects a custom HTTPS MCP endpoint and an optional
  Header/API key as visibly separate fields in one form. Creation stores the
  endpoint first, then submits the transient credential through the credential
  route before validation; the secret never enters the endpoint URL.
- Marketplace detail also offers an explicit **custom relay URL** branch. It
  must be visually distinct from the backend-locked official/Runner deployment,
  collect HTTPS URL plus `none|header|oauth` authentication separately, and
  submit those fields only through the typed Marketplace install API. The
  browser never rewrites the official deployment or inserts a secret into the
  URL; Backend creates a provenance-marked private remote Server and performs
  the same SSRF, credential-vault, and MCP readiness validation. The user may
  select the ready installation later from the composer.
  Show this branch only when the authoritative detail exposes an HTTP option,
  Header/OAuth mode, or required secret fields. Credential-free local stdio
  artifacts such as Context7 do not render a relay option.
- The process trace maps only backend-redacted call summaries and states. An
  MCP row renders `<serverName> · <humanized toolName>`, the authoritative
  outer process status, and duration. Humanize third-party identifiers
  generically (`resolve-library-id` -> `Resolve library id`) rather than
  maintaining a product-specific Tool-name dictionary. Do not render the
  internal `server` reference, `classification`, `callStatus`, or
  `argumentSummary`; operators use Backend diagnostics instead. Results remain
  collapsed by default. Manual retry is allowed only if the backend exposes a
  trusted idempotent read retry affordance; never infer it from remote
  annotations.
- A ProcessStep presentation is untrusted input. Accept only schema version 1
  cards whose `kind`, exact Tool name, and mode authorize that card. Strictly
  bound every string, list, transcript and numeric field. Unknown/malformed or
  cross-mode cards are dropped while the enclosing legacy step remains. Typed
  cards cover Terminal, Search, File, Job, Skill, Goal, Browser and generic MCP;
  an optional approval union accepts only a UUID, positive revision, bounded
  enum state/decision, parseable expiry, and Boolean conversation capability.
  Malformed approval data is dropped without destroying its safe Tool card.
- A pending Terminal approval expands by default and offers `Allow once`,
  policy-permitted `Allow for conversation`, and `Deny`. Submit the exact
  displayed revision to the typed decision API and runtime-validate the full
  response. First-decision-wins responses replace only the approval metadata;
  browser state never becomes approval authority.
  generic MCP/Browser cards remain summary-only. Never reuse `argumentSummary`
  or render raw JSON. Terminal transcript is backend-redacted and bounded to
  64 KiB; File content/diff is bounded to 64 KiB and paths remain workspace-relative.
- Repeated running ProcessSteps with the same ID update the existing Terminal
  card in place. They are transient live projections; reload must converge on
  the final durable snapshot and must not create one row per output chunk.
- An admitted canary receives those coalesced snapshots as `agent.progress`,
  never as `process.step.updated`. Keep transient steps separate from the
  immutable event accumulator, overlay them for display, and remove the exact
  step when its next durable event arrives or the Turn ends.
- Normalize each `agent.event` at the untrusted stream boundary. Deduplicate by
  immutable `eventId`, order by the inner durable sequence, and project the
  accumulated events through the same function used for reload. The outer
  stream sequence remains only the reconnect cursor; a duplicate reconnect
  frame must not replace an already accepted durable event.
- On the legacy control path, normalize `tool.call.updated` with its
  mode/classification pair. MCP accepts
  only `read|write|unknown`; `local_direct` accepts `read|write|execute`.
  `execute` must not widen MCP Server definition or Tool classification
  validation. Reject unknown modes and mismatched pairs, but do not label a
  valid local Skill event as an invalid MCP update.
- The redacted local event may omit Provider `callId`. Only when the wire field
  is absent, keep `executionId` mandatory and bounded and use it as the
  normalized UI `callId` fallback. A present empty/wrong-type identity remains
  an invalid response; optionality must not hide malformed data.
- A completed trace with generic MCP Tool steps but no specialized Knowledge or
  Web steps summarizes the number of Tool calls. It must not label Tool-backed
  work as a Direct answer.
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
| API mode is local or MCP config disabled | Sidebar Tools shows unavailable/disabled state; composer omits the MCP picker |
| Tools inventory load fails | bounded localized error; no stale inventory is shown as current |
| Composer selection load fails | bounded localized error; no stale authority expansion |
| Create succeeds but response normalization or validation fails | reload the authoritative Server list so the persisted draft remains visible; show the bounded error |
| Server needs auth/unavailable | visible state; send remains blocked until explicit change |
| OAuth URL is not valid HTTPS | localized error; do not navigate |
| Selection revision is stale | show save failure and reload authoritative state |
| `outcome_unknown` timeline event | terminal warning state; no one-click retry |
| `local_direct` update with `read|write|execute` and optional `callId` | accept, derive missing `callId` from `executionId`, and render through the shared redacted Tool timeline |
| unknown Tool mode or a mode/classification mismatch | reject as `INVALID_SERVER_RESPONSE`; do not widen MCP definition trust |
| MCP trace has only legacy detail without `serverName` | show a humanized Tool action or generic Tool label; never fall back to the internal Server reference |
| Legacy Plugin fields load/import | strip recursively; persist no Plugin or inferred MCP state |
| Marketplace disabled or unconfigured | show a bounded configuration state; Installed remains fully usable |
| Search/detail upstream failure | show a retryable Marketplace-only error; keep installed/selection state unchanged |
| Older search fails after a newer search succeeds | ignore the stale completion; keep the newer results with no false error banner |
| Repeated bottom-sentinel callbacks | issue at most one request for the next page; append no duplicate identifiers |
| Next Marketplace page fails | preserve loaded cards and expose a manual retry; do not loop automatically |
| Item is SSE or unmatched stdio/command-only | show compatibility reason; no install request is emitted |
| Backend marks exact npm stdio deployment installable | administrator sends only identifier/version/hash; browser never receives or executes command metadata |
| Current user is not MCP administrator | hide management/install/configuration actions; keep inventory inspectable and composer selection separately usable |
| Install validation/configuration step fails after draft creation | reload installed Servers so the recoverable draft remains visible |
| Installed Server icon is missing or its HTTPS image fails | show the local generic MCP fallback; keep the card usable |
| Installed Server has many Tools | keep read-only Tool rows folded by default and expose the count with `aria-expanded`/`aria-controls` |
| Selected deployment requires a missing secret | keep install disabled and render the secret input plus completion hint in the selected card |
| Custom relay URL is incomplete or non-HTTPS | keep install disabled client-side; Backend still rejects unsafe/private resolution before create |
| Provider stream interrupts after partial answer content | keep the content and show the localized Provider-interruption notice; do not blame or retry MCP Tools |

### 5. Good / Base / Bad Cases

- **Good**: a user installs/configures one granted server in Sidebar Tools,
  selects it for the exact Conversation through the compact composer picker,
  sends in Agent mode, and sees the queued-to-succeeded timeline.
- **Base**: a conversation without Workspace or selection shows zero enabled
  Tools and sends ordinary chat.
- **Bad**: hydrate MCP selection from `activePlugins`, embed full MCP management
  in the composer, retain a credential in Zustand, or execute a Tool from the
  browser.

### 6. Tests Required

- API-client URL/body/response mapping and local-mode fail-closed behavior.
- Tools inventory load, private draft validation, credential submission, OAuth
  URL rejection, unavailable/auth states, default-folded read-only Tool
  disclosure, and hidden `unknown` badges. Composer tests own
  inherited/custom/explicit-empty selection and must prove one lightweight
  picker, exact Conversation authority, and full management/preflight absence.
- Sidebar Tools entry, `?panel=tools` URL round-trip, top-level page
  composition, and Server listing without a current Conversation.
- Marketplace tab/search/detail, category filtering/counts, remote-icon
  fallback, bounded infinite pagination/reset/append/retry/race fencing,
  loaded/total reporting, Installed shared-icon/fallback rendering,
  compatibility labels, disabled/unconfigured behavior,
  backend-approved remote/stdio installability, authoritative install payload,
  latest-request race fencing, no command fields, install recovery, and an
  inventory-only payload with `enableForConversation: false`.
- Timeline mapping for every state including `outcome_unknown`, redacted
  summaries, cancellation, generic Tool-name humanization, readable
  `serverName`, and non-rendering of internal Server/call/schema detail.
- Stream normalization covers valid MCP and `local_direct` updates, local
  `write|execute`, absent-`callId` fallback, malformed explicit identity, and
  rejects unknown modes plus MCP/`execute` cross-mode drift. Exercise the live
  SSE normalization path directly; a reload-only durable timeline replay is
  not evidence that the live wire contract works.
- Stream reconnect tests split the primary response after a known sequence,
  assert `GET /v1/chat/runs/{runId}/events?after=...` plus `Last-Event-ID`, and
  prove replayed Tool/transcript updates deduplicate. An explicit valid gap
  renders the localized partial-view reason and terminal Message replacement;
  malformed gap bounds/run identity fail closed.
- A background Terminal presentation normalizes as a Job `start`. Display
  projection merges later `job_output`/`job_kill` cards by exact `jobId`, keeps
  the first card position/round, applies the latest status/output, and never
  mutates the normalized source steps. A recovered interrupted Turn changes an
  unresolved process-local Job presentation to `interrupted`.
- The frontend never self-enables the typed timeline. A non-canary Backend
  response omits Agent events/presentations and must continue through the
  legacy ProcessStep renderer without synthesizing cards or approval controls.
- A Retry control renders only from a normalized Backend-issued `retry`
  presentation containing a valid event UUID and bounded `retryOf`, and only
  on `local_direct file_read/read`. It submits no Tool name, path, arguments,
  classification, or user-selected policy. The Backend response becomes a new
  sibling Agent Message; duplicate clicks reuse the source-event idempotency
  key. Unknown, write, execute, MCP, and malformed retry shapes are dropped.
- Terminal presentation tests cover running/completed, exit 0/nonzero,
  timeout/truncation/background pills, malformed-card fail-closed behavior,
  legacy steps, pending/allowed/denied/expired approval controls, malformed
  approval fail-closed behavior, decision API validation, and byte-equivalent
  live `agent.event` versus durable Agent-event replay, with transient Terminal
  progress tested separately.
- Timeline acceptance uses one 500-event fixture for both paths. Measure full
  projection plus render after warm-up: visible live-update p95 must remain at
  or below 300 ms, while durable reload p95 must remain within 20% of the
  legacy projection under the same workload. Keep durable projection linear
  with a per-pass step-ID index; do not repeatedly scan the accumulated list.
- A hostile durable-event fixture must retain safe status and the generic MCP
  fallback while proving that raw arguments/results, artifact bodies, secrets,
  runtime-owned host paths, ANSI/control bytes, and oversized presentation
  content do not reach rendered markup. Keep the legacy ProcessStep fallback
  for disjoint controls and flag-off rollback until these gates have passed one
  focused exact-user canary acceptance session; do not widen or delete it
  earlier.
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

#### Wrong

```ts
if (event.toolCall.mode !== "mcp") throw invalidServerResponse();
```

#### Correct

```ts
const update = normalizeMcpToolCallUpdate(event.toolCall);
if (!update) throw invalidServerResponse();
// The normalizer validates the exact mode/classification pair.
```
