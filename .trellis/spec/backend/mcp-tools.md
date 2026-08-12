# MCP Tools Backend Contract

## Scenario: Server-authoritative MCP Tools

### 1. Scope / Trigger

Apply this contract when changing `/v1/mcp/*`, MCP catalog/manifest/private
servers, Workspace grants, conversation selection, credentials/OAuth, provider
Tool rounds, result persistence, deletion/retention, or the stdio Runner
connector. Assistants and Skills are separate product concepts and must not be
changed as part of MCP work.

Neo Chat is an MCP client only. Do not add a public MCP proxy/server, browser
execution, OpenAPI adapter, legacy Plugin fallback, or MCP Resources/Prompts/
Sampling/Elicitation in the Tools MVP.

### 2. Signatures

Public routes:

```text
GET|POST /v1/mcp/servers
POST       /v1/mcp/servers/private/{id}/validate
DELETE     /v1/mcp/servers/private/{id}
PUT|DELETE /v1/mcp/servers/{source}/{id}/credential
POST       /v1/mcp/oauth/start
GET        /v1/mcp/oauth/callback
POST       /v1/mcp/oauth/revoke
GET|PUT    /v1/mcp/conversations/{id}/selection
GET|PUT    /v1/mcp/workspaces/{id}/selection
GET        /v1/mcp/conversations/{id}/calls?runId=
GET        /v1/mcp/marketplace/search?q=&category=&page=&pageSize=
GET        /v1/mcp/marketplace/items/{identifier}?version=
POST       /v1/mcp/marketplace/items/{identifier}/install
```

Execution seams:

```go
PrepareRun(ctx, userID, conversationID, messageID, runID string) (PreparedRun, error)
ExecuteRound(ctx, PreparedRun, []ExecuteInput, EventSink) ([]CallResult, error)
PruneExpiredData(ctx context.Context, limit int) (int, error)
RunRetention(ctx context.Context, onError func(error))
```

Database authority starts with migration `074_mcp_tools_foundation`: Workspaces
and memberships plus MCP servers, grants, credentials, OAuth states,
selections, run snapshots, calls, results, and
`mcp_artifact_cleanup_queue`. Migration `075_mcp_runtime_role_grants` grants
the Go API role the required repository/cleanup capabilities and revokes
public execution from the account-artifact trigger. Migration
`076_mcp_private_runner_artifacts` extends `mcp_servers.transport` to
`streamable_http|stdio`, constrains private stdio endpoints to
`runner://[approved-id]`, and refuses down while any stdio row exists.

### 3. Contracts

- Public API is authenticated except the state-bound OAuth callback, strict
  camelCase JSON, 1 MiB body maximum, unknown-field rejection, stable error
  envelopes, and `Cache-Control: no-store`.
- Collection fields in public DTOs are always JSON arrays. In particular, a
  newly created draft Server returns `tools: []`, never `tools: null`, so a
  successful write cannot be mistaken for an invalid response by the client.
- Sources are `catalog|manifest|private`; transports are
  `streamable_http|stdio`; selection is `inherit|custom`. `custom` with no
  servers explicitly disables all Tools.
- Before accepting a send, resolve current user/Workspace grant, selection,
  credential, status, transport switch, compatible schema, and native model
  Tool capability. Persist one immutable run snapshot. Reauthorize every call
  against that snapshot.
- MCP is an intentional extension of the existing `ToolRoundProvider` loop.
  Use `model -> calls -> results -> same model`; never prompt-simulate calls or
  switch models.
- Preserve each frozen third-party MCP input schema without advertising the
  Provider-specific OpenAI `strict` extension. The MCP runtime remains the
  argument-validation authority before connector execution.
- Map only an explicit Tool-protocol incompatibility to
  `MCP_MODEL_UNSUPPORTED`; a generic first-round Provider rejection is
  `MCP_PROVIDER_FAILED`.
- Default hard limits are 8 rounds, 32 calls, 30 seconds/call, 120 seconds/run,
  32 exposed schemas/round, and 4 concurrent calls/user. Same-user writes and
  unknowns serialize across runs; trusted reads may run at concurrency four.
- A trusted idempotent read may retry once only when no result was observed and
  budget remains. Write/unknown never auto-retry. A dropped write returns
  `outcome_unknown` and stops the run.
- Private remote endpoints remain public HTTPS across every resolution/
  redirect. Private stdio rows are backend-created `runner://` references only;
  users cannot submit them through the private-server API. Credentials never
  cross an origin change. Ordinary private Server annotations cannot grant
  read/idempotent authority and normalize to `unknown`. Only a private stdio
  Server whose exact Marketplace provenance is rebound to the current reviewed
  manifest artifact may inherit that artifact's local `toolPolicy`; an absent
  or unlisted policy remains `unknown`.
- OAuth state is random, stored only as a hash, user/server/return-bound,
  single-use, and ten-minute limited. Refresh is singleflight per credential;
  `invalid_grant` revokes authority.
- Normalize Text/JSON/Image/Audio/Resource Link as untrusted data. Never fetch
  Resource Links. Keep at most 256 KiB inline, 25 MiB/item, and 50 MiB/call;
  larger/media bytes use validated `mcp-results/<conversation>/<call>/...` keys.
- Call/audit retention defaults to 90 days. Delete object bytes before call
  rows. Conversation deletion is synchronous; account deletion queues object
  coordinates before FK cascade. Cleanup remains active when MCP is disabled;
  startup and periodic sweep failures emit a fixed redacted diagnostic event
  and do not stop later retries.
- Keep migration `011` and `plugin_registry` read-only for one rollback
  release. No handler, runtime config, health check, or execution path may read
  it.
- LobeHub Marketplace is a backend-only, optional discovery adapter. Search and
  detail requests are authenticated to Neo Chat, bounded by timeout/body/page
  limits, use a short server cache, and never expose Marketplace machine
  credentials. Marketplace failure must not affect installed Servers or chat.
- Marketplace search returns bounded `categories: [{category,count}]` facets
  and accepts one bounded exact category key. Item icons are reduced to a
  bounded HTTPS URL or short text/emoji; all remain untrusted display metadata.
- A Marketplace install accepts only `identifier`, exact `version`, optional
  `conversationId`, `selectionRevision`, and `enableForConversation`. The
  backend re-fetches the authoritative detail and never accepts a URL, command,
  or deployment object from the browser.
- LobeHub detail fetches use identifier plus locale only. Do not forward the
  selected version as an upstream query parameter: the current API rejects that
  shape. Compare the version returned by the fresh detail to the client-selected
  exact version and return `MCP_MARKETPLACE_CHANGED` on drift.
- An explicit public HTTPS `http` deployment is installable through the remote
  path. A `stdio` option is installable only when the current manifest contains
  an unauthenticated hidden artifact matching provider, identifier, exact
  Marketplace version, connection/install method, command, arguments, package
  name, and derived deployment hash. `sse` and unmatched package/command paths
  remain display-only.
- A private stdio install persists only `runner://<artifact-id>`, the artifact
  ID, and bounded `provider + identifier + version + deploymentHash`
  provenance. Public Server DTOs expose neither the internal endpoint nor
  metadata. Validation, selection, and execution re-bind all provenance fields
  to the current manifest and reapply its current local Tool policy before
  scheduling. Artifact or provenance drift fails closed instead of retaining a
  stale read classification. The Runner executes only the manifest's separate
  absolute argv and never the Marketplace command.
- Marketplace installation reuses private Server quota/deduplication, SSRF-safe
  validation, Tool discovery, credentials/OAuth, and revision-checked
  Conversation selection. Third-party official/validated/rating metadata never
  upgrades Tool classification or execution trust.

Environment:

```text
MCP_ENABLED MCP_REMOTE_ENABLED MCP_STDIO_ENABLED
MCP_MANIFEST_FILE MCP_RUNNER_URL MCP_RUNNER_TOKEN_FILE
MCP_OAUTH_CALLBACK_URL
MCP_PRIVATE_SERVER_LIMIT MCP_CONVERSATION_SERVER_LIMIT
MCP_MAX_EXPOSED_TOOLS MCP_MAX_CALLS_PER_RUN MCP_MAX_ROUNDS_PER_RUN
MCP_MAX_CONCURRENT_PER_USER MCP_MAX_PENDING_OAUTH_FLOWS
MCP_CALL_TIMEOUT MCP_RUN_TIMEOUT
MCP_AUDIT_RETENTION MCP_CLEANUP_INTERVAL
MCP_MARKETPLACE_ENABLED MCP_MARKETPLACE_BASE_URL
MCP_MARKETPLACE_CLIENT_ID MCP_MARKETPLACE_CLIENT_SECRET_FILE
MCP_MARKETPLACE_TIMEOUT MCP_MARKETPLACE_CACHE_TTL
```

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| MCP or selected transport disabled | `503 MCP_DISABLED` or `MCP_TRANSPORT_DISABLED`; no execution |
| Private HTTP/private-network/rebinding/unsafe redirect | `400 MCP_INVALID`; no credential egress |
| Duplicate active private endpoint for one user | `409 MCP_CONFLICT`; preserve the existing draft/Server |
| Missing/stale grant, selection, credential, or server status | reject before send or `MCP_AUTH_REQUIRED`/`MCP_SERVER_UNAVAILABLE` |
| Model lacks native Tool calls | reject MCP-enabled send; no prompt planner or model switch |
| Unknown/duplicate/unsupported schema | disable only that Tool with a visible reason |
| Arguments fail frozen schema | bounded Tool error; no connector call |
| Trusted read loses connection before a result | at most one retry when budget allows |
| Write/unknown loses connection | persist `outcome_unknown`; no retry or guessed answer |
| Object deletion partially fails | retain PostgreSQL row/queue entry for idempotent retry |
| `plugin_registry` is present | retain for rollback compatibility; active access is forbidden |
| Marketplace disabled/missing credentials/upstream failure | stable Marketplace-specific failure; installed Servers and chat remain available |
| LobeHub rejects an upstream `version` query | do not send that unsupported query; fetch current detail and compare the returned exact version locally |
| Marketplace stdio option exactly matches a manifest artifact and stdio is enabled | detail returns `installable`; install creates/validates one private Runner reference |
| Marketplace item has only SSE or unmatched command/package options | detail remains visible and incompatible/Runner-required; install is rejected |
| Private stdio provenance/artifact ID is stale or tampered | validation/selection/execution fails closed with server unavailable |
| Reviewed private stdio Tool is listed as `read` by the current artifact policy | expose `read`; permit bounded read concurrency |
| Private remote annotation claims read-only or a reviewed artifact omits a Tool policy | normalize the Tool to `unknown`; serialize and never read-retry |
| Client submits stale version/revision | no authority substitution; reject the install/selection mutation without hiding an already-created Server |

### 5. Good / Base / Bad Cases

- **Good**: current grants and an explicit selection freeze one snapshot; four
  compatible reads execute concurrently and the same model continues.
- **Good**: an exact reviewed Marketplace stdio fingerprint resolves to one
  image-bundled absolute executable without running `npx` or downloading code.
- **Base**: no Workspace and no custom selection exposes no MCP Tools and chat
  behaves normally.
- **Good failure**: a write connection drops, the timeline records
  `outcome_unknown`, the run stops, and no automatic/manual one-click retry is
  offered.
- **Bad**: trust a server annotation as read-only, retry a write, let Tool output
  add another server, or execute a client-supplied alias not in the snapshot.

### 6. Tests Required

- Unit: manifest strictness, aliases/schema isolation, SSRF/DNS/redirect origin,
  OAuth PKCE/state/refresh/revoke, result caps, scheduling/retry/cancellation,
  write serialization, retention, and account/conversation artifact deletion.
- HTTP: auth, methods, strict JSON, no-store, stable errors, cross-user/
  cross-Workspace denial, explicit-empty and revision conflict.
- Chat: fake Streamable HTTP and fake Runner complete native multi-round same-
  model continuation and persist the structured timeline; unsupported model
  rejects before acceptance.
- PostgreSQL 17: fresh `001..076`, replay, guarded `076` down with/without a
  stdio row, `075` grant down/up plus `074` schema down/up, retired metadata
  purge without security-field loss, 12 MCP tables, runtime-role denial/grants,
  stdio repository lifecycle, account cascade queue, and final replay via
  `scripts/verify-mcp-postgres17.sh`.
- Security: logs/metrics/errors contain no argument, result, token, custom URL,
  or high-cardinality user/server/tool label.
- Marketplace: M2M token singleflight/expiry, URL construction, response caps,
  query/category/page bounds, category facets, icon sanitization, strict
  identifier/version handling including no upstream detail-version query,
  local drift rejection, deployment selection,
  no-command execution, SSRF/deduplication reuse, exact stdio artifact match,
  provenance reauthorization, current-artifact Tool-policy rebinding, ordinary
  private-annotation denial, hidden Runner endpoint, and install-plus-selection
  revision conflict/recovery.

### 7. Wrong vs Correct

#### Wrong

```go
// Browser or Tool output chooses arbitrary execution authority.
connector.CallTool(ctx, clientServerURL, clientToolName, clientArgs)
```

#### Correct

```go
prepared, err := service.PrepareRun(ctx, userID, conversationID, messageID, runID)
// ExecuteRound resolves only aliases and schemas frozen in prepared.
results, err := service.ExecuteRound(ctx, prepared, calls, emit)
```

#### Wrong

```go
// A private Tool's remote annotation becomes scheduling/retry authority.
tool.Classification = annotationClassification(remoteTool)
```

#### Correct

```go
// This binding is reached only after exact current-artifact provenance passes.
artifact, err := service.privateRunnerArtifact(privateServer)
tools = bindPrivateRunnerToolPolicy(tools, artifact)
```
