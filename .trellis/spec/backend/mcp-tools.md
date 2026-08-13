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

MCP execution events crossing into chat process trace carry both identities:

```go
type ExecutionEvent struct {
    ServerRef  ServerRef // internal authorization/diagnostic identity
    ServerName string    // trimmed, UTF-8-safe, at most 256 bytes
    ToolName   string
    Status     string
}
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
Migration `077_mcp_marketplace_install_credentials` adds the `env` Server and
credential kinds used only for declared Runner environment fields. Migration
`078_mcp_legacy_tavily_runner_repair` narrowly converts the exact failed legacy
Tavily remote row into its reviewed Runner reference and marks it for
credentials; rollback recognizes only its repair marker. Migration
`079_mcp_tavily_credential_revalidation` moves an exact legacy ready Tavily
Runner row back to `needs_auth` so protocol initialization cannot stand in for
credential validation.
Migration `080_mcp_legacy_deepwiki_icon` adds only bounded display metadata to
the exact legacy DeepWiki endpoint when it has no icon; rollback removes only
the value carrying that migration's repair marker.
Migration `081_mcp_legacy_context7_artifact_rebind` updates only the exact
legacy Context7 Runner provenance hash to the current reviewed manifest hash;
rollback restores only the value carrying that migration's repair marker.

### 3. Contracts

- Public API is authenticated except the state-bound OAuth callback, strict
  camelCase JSON, 1 MiB body maximum, unknown-field rejection, stable error
  envelopes, and `Cache-Control: no-store`.
- `AUTH_BOOTSTRAP_USER_ID` is the MCP deployment administrator. Only that exact
  user may create, install, delete, validate, configure credentials, or start
  and revoke OAuth for managed definitions. Administrator-installed definitions
  are globally visible; ordinary users may select ready definitions and
  enable/disable their tools for their Conversations but cannot mutate shared
  definition or credential state. Enforcement is server-side and rejection is
  the stable `403 MCP_ADMIN_REQUIRED`; UI hiding is not authorization.
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
- Public Server DTOs expose optional `icon` only after the same backend
  normalization. New Marketplace private Servers persist that bounded display
  value in existing metadata. Reviewed private stdio Servers rebind their icon
  to the current exact manifest artifact on listing/validation, so a stale or
  Marketplace-supplied icon never outranks current local review authority.
- Authenticated Marketplace detail derives `installed` from the deployment
  administrator's globally visible definitions using exact Marketplace
  `provider + identifier + version` provenance. It never trusts browser state,
  display name, or endpoint alone. Detail exposes only a bounded `canInstall`
  capability and never returns command metadata.
- A Marketplace install accepts only `identifier`, exact `version`, optional
  exact backend-derived `deploymentHash`, transient backend-declared `secrets`,
  `conversationId`, `selectionRevision`, and `enableForConversation`. The
  backend re-fetches the authoritative detail and never accepts a URL, command,
  header name, or deployment object from the browser.
- LobeHub search/category fetches use the M2M bearer token, but current item
  detail fetches are public and must send no Marketplace credential (the
  upstream rejects that token on the detail route). Detail fetches use
  identifier plus locale only. Do not forward the selected version as an
  upstream query parameter: the current API rejects that shape. Compare the
  version returned by the fresh detail to the client-selected exact version
  and return `MCP_MARKETPLACE_CHANGED` on drift.
- An explicit public HTTPS `http` deployment is installable through the remote
  path. An npm `stdio` option is administrator-installable when current
  Marketplace detail declares `npx`, a valid registry package name, an exact
  version, bounded arguments and environment fields, and the derived
  deployment hash. Shell, Docker, Git, URL/file package specs, and non-npx
  command paths remain display-only.
- Marketplace catalog versions are not assumed to equal npm package versions.
  For npm `stdio`, resolve the Marketplace-declared package selector against
  the fixed HTTPS npm registry and persist only the returned exact SemVer
  package version. Missing/mismatched registry metadata makes the deployment
  incompatible instead of allowing a broken install or a floating Runner tag.
- A dynamic stdio install persists only `runner://<artifact-id>`, a bounded
  Backend-owned artifact definition, and `provider + identifier + version +
  deploymentHash` provenance. Public Server DTOs expose neither the internal
  endpoint nor command metadata. Validation, selection, and execution rebind
  the definition from administrator-owned state and keep every Tool
  classification `unknown`; Marketplace annotations cannot grant read/retry
  authority. Backend seals artifact definition and environment separately for
  each authenticated internal Runner request.
- Marketplace authentication routes server-authoritatively as anonymous HTTP,
  approved header HTTP, OAuth HTTP, or reviewed Runner environment. Query
  secret templates are removed from URLs; approved headers are allowlisted;
  Runner environment names exact-match the current artifact. Submitted secrets
  enter only the encrypted Backend credential vault.
- Marketplace installation may explicitly branch to a user-owned custom remote
  relay without changing the authoritative Marketplace deployment. The request
  carries separate `customEndpointUrl`, `customAuthType`, optional approved
  Header shape/credential, or optional OAuth client ID. Backend re-fetches the
  exact Marketplace item for identity/display provenance, creates an ordinary
  private Streamable HTTP Server marked `connectionMode=custom_remote`, applies
  public-HTTPS/SSRF and credential validation, and selects it only after
  `ready`. The custom URL is not represented as an official Marketplace URL.
  The branch is eligible only when the refreshed detail contains an HTTP
  deployment, Header/OAuth install mode, or required secret fields; pure
  credential-free stdio entries reject custom-relay requests.
- OAuth remote installs discover protected-resource and authorization-server
  metadata, require PKCE S256, and may use Dynamic Client Registration only
  when the live server advertises a validated registration endpoint. The
  generated client identity remains inside encrypted OAuth flow/token state.
  Callback URLs are public HTTPS or exact loopback HTTP only.
- One shared constrained Runner starts multiple stdio children on demand rather
  than one permanent container per MCP. Dynamic children get isolated
  work/HOME/npm-cache directories and bounded environment. The `/work` tmpfs is
  `exec,nosuid,nodev` because npm materializes package bins there; `/tmp`
  remains `noexec`, and the container remains non-root/read-only/capability-free.
  Process reuse is keyed by installed Server ID plus a non-reversible credential
  fingerprint. Updating a credential replaces the child process; cancel/failure/
  idle/lifetime expiry kills its process group and removes its workspace.
  Cold dynamic npm download plus MCP initialize has a separate bounded two-minute
  window; the internal HTTP server envelope is slightly longer, while
  steady-state Tool calls keep the normal shorter call timeout.
- Browser-backed stdio packages use the exact Chromium binary baked into the
  pinned official Playwright MCP base image, carried into the read-only Runner,
  and exposed at Playwright's expected Chrome path.
  Runtime `playwright install` or other browser downloads are not the repair
  path because they would be ephemeral, version-drifting mutations.
- Every MCP call event resolves `ServerName` from the same current-authorized
  `Server` selected for execution. Process trace may retain the internal
  `ServerRef.Key()` for Backend diagnostics and compatibility, but the bounded
  display name is the only Server identity intended for product UI. Never ask
  the browser to recover a display name from `private:<uuid>` or a stale local
  registry.
- For a reviewed private stdio install, rebind its display name as well as its
  icon, command, and Tool policy to the current exact local artifact before
  listing, validation responses, selection snapshots, or execution events. A
  verbose/stale Marketplace title never outranks current artifact display
  authority. Dynamic npm artifacts retain their bounded installed name because
  their artifact definition is reconstructed from that same Server.
- MCP initialize and tools/list prove only protocol availability, not provider
  credential validity. A reviewed Runner artifact that requires a live
  credential check declares one operator-owned HTTPS probe in the local
  manifest. Marketplace/browser metadata cannot supply its URL, method,
  secret binding, or success rule. `401`/`403` records `needs_auth` with
  `credential_invalid`; transport/upstream failure records `unavailable`; only
  a successful probe may produce `ready`.
- Marketplace installation reuses private Server quota/deduplication, SSRF-safe
  validation, Tool discovery, credentials/OAuth, and revision-checked
  Conversation selection. Third-party official/validated/rating metadata never
  upgrades Tool classification or execution trust.
- Reinstalling the exact same Marketplace deployment is idempotent across a
  recoverable draft: match the current user plus exact backend-revalidated
  `provider + identifier + version + deploymentHash`, transport, endpoint,
  auth shape, and Runner artifact binding. Reuse that private Server, overwrite
  a newly submitted credential in the vault, and validate again. Never reuse by
  display name or endpoint alone; an unrelated private Server that owns the
  endpoint remains `MCP_CONFLICT`.

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
| Marketplace custom relay is non-HTTPS/private, mixes official deployment fields, or has inconsistent auth fields | reject before Server creation; persist/log no credential |
| Duplicate active private endpoint for one user | Reuse only an exact current Marketplace-provenance deployment; otherwise `409 MCP_CONFLICT` and preserve the existing Server |
| Missing/stale grant, selection, credential, or server status | reject before send or `MCP_AUTH_REQUIRED`/`MCP_SERVER_UNAVAILABLE` |
| Model lacks native Tool calls | reject MCP-enabled send; no prompt planner or model switch |
| Unknown/duplicate/unsupported schema | disable only that Tool with a visible reason |
| Arguments fail frozen schema | bounded Tool error; no connector call |
| Trusted read loses connection before a result | at most one retry when budget allows |
| Write/unknown loses connection | persist `outcome_unknown`; no retry or guessed answer |
| Object deletion partially fails | retain PostgreSQL row/queue entry for idempotent retry |
| `plugin_registry` is present | retain for rollback compatibility; active access is forbidden |
| Marketplace disabled/missing credentials/upstream failure | stable Marketplace-specific failure; installed Servers and chat remain available |
| LobeHub detail route rejects the search bearer token or an upstream `version` query | send no Marketplace credential and no version query; fetch current detail and compare the returned exact version locally |
| Marketplace stdio option is exact manifest-approved or valid bounded npm/npx and stdio is enabled | detail returns `installable`; install creates/validates one administrator-owned Runner reference |
| Marketplace item has only SSE, non-npx, Shell/Docker/Git/URL/file, or floating package options | detail remains visible and incompatible/Runner-required; install is rejected |
| Private stdio provenance/artifact ID is stale or tampered | validation/selection/execution fails closed with server unavailable |
| Reviewed private stdio Tool is listed as `read` by the current artifact policy | expose `read`; permit bounded read concurrency |
| Private remote annotation claims read-only or a reviewed artifact omits a Tool policy | normalize the Tool to `unknown`; serialize and never read-retry |
| Installed Server has a normalized icon | expose it as display-only `icon`; never treat it as trust/execution authority |
| Server display name enters an execution event | trim and UTF-8-bound it to 256 bytes; preserve the internal ref separately |
| Icon is unsafe, oversized, or missing | omit it from the DTO; frontend uses a local fallback |
| Client submits stale version/revision | no authority substitution; reject the install/selection mutation without hiding an already-created Server |

### 5. Good / Base / Bad Cases

- **Good**: current grants and an explicit selection freeze one snapshot; four
  compatible reads execute concurrently and the same model continues.
- **Good**: a refreshed Marketplace stdio deployment resolves either to one
  exact image-bundled manifest executable or one sealed exact-version npm/npx
  artifact downloaded only inside the isolated Runner.
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
  model continuation, emit/persist the bounded Server display name beside the
  internal ref, and persist the structured timeline; unsupported model rejects
  before acceptance. Any method added to the shared `Repository`
  interface must also be added to Chat and cross-package test fakes; compile the
  owning Chat package in the focused gate so an MCP-only unit run cannot hide
  interface drift.
- PostgreSQL 17: fresh `001..081`, replay, clean `081..077` down, guarded `076`
  down with/without a stdio row, `075` grant down/up plus `074` schema down/up,
  retired metadata purge without security-field loss, 12 MCP tables,
  runtime-role denial/grants, stdio repository lifecycle, targeted legacy
  repair/rollback assertions, account cascade queue, and final replay via
  `scripts/verify-mcp-postgres17.sh` and
  `scripts/verify-mcp-install-credentials-postgres17.sh`. When a new tail
  migration is added, both drills must advance their fresh, down, re-up, and
  final replay expectations in the same change.
- Security: logs/metrics/errors contain no argument, result, token, custom URL,
  or high-cardinality user/server/tool label.
- Marketplace: M2M token singleflight/expiry, URL construction, response caps,
  query/category/page bounds, category facets, icon sanitization, strict
  identifier/version handling including no upstream detail-version query,
  local drift rejection, deployment selection,
  no-command execution, SSRF/deduplication reuse, exact stdio artifact match,
  provenance reauthorization, current-artifact Tool-policy/icon rebinding,
  ordinary private-annotation denial, bounded icon persistence/DTO projection,
  hidden Runner endpoint, and install-plus-selection revision conflict/recovery.

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
