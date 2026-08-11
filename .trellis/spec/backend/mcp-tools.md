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
public execution from the account-artifact trigger.

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
- Private endpoints remain public HTTPS across every resolution/redirect.
  Credentials never cross an origin change. Private annotations cannot grant
  read/idempotent authority.
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

### 5. Good / Base / Bad Cases

- **Good**: current grants and an explicit selection freeze one snapshot; four
  compatible reads execute concurrently and the same model continues.
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
- PostgreSQL 17: fresh `001..075`, replay, `075` grant down/up plus `074` schema
  down/up, retired metadata purge without security-field loss, 12 MCP tables,
  runtime-role denial/grants, repository lifecycle, account cascade queue, and
  final replay via `scripts/verify-mcp-postgres17.sh`.
- Security: logs/metrics/errors contain no argument, result, token, custom URL,
  or high-cardinality user/server/tool label.

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
