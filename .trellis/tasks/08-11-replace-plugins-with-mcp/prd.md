# Replace the Plugin System with MCP

## Goal

Replace Neo Chat's current OpenAPI/HTTP plugin subsystem with a secure,
server-authoritative Model Context Protocol (MCP) client implementation. The
replacement must execute provider-native multi-round tool calls, support
remote Streamable HTTP servers and an optional hardened stdio runner, expose a
clear conversation-level Tools experience, and remove the old plugin UI, API,
and runtime in one product cutover. Assistants and Skills remain unchanged.

## Requirements

### Product scope

- Rename the top-level product area to **Tools** while identifying MCP as the
  underlying protocol.
- Remove the plugin UI, `/v1/plugins*` routes, OpenAPI plugin execution,
  built-in Weather/Unsplash/Agnes plugins, and frontend plugin planning.
- Do not ship an OpenAPI-to-MCP adapter or a long-lived dual stack.
- Keep `plugin_registry` read-only for one stable rollback release; delete it
  only in a later, independent migration.
- Implement Neo Chat as an MCP client only. Do not expose a public MCP proxy or
  make Neo Chat an MCP server.
- The MVP supports MCP Tools only. Resources, Prompts, Sampling, Elicitation,
  and legacy SSE transport are out of scope.

### MCP server sources and authority

- Ship an audited, rollback-safe catalog with each Neo Chat release.
- Permit users to create private remote MCP definitions only for public HTTPS
  endpoints. Persist them as disabled drafts until validation succeeds.
- Load administrator-shared remote and stdio definitions from a strict,
  versioned manifest. Unknown fields, duplicate IDs, relative commands,
  inline secrets, shell commands, and unsafe environment entries are invalid.
- Shared definitions may be granted globally or to a Team/Workspace.
- Introduce server-authoritative Workspace identity, membership, and MCP grants
  where current browser-only Workspace state cannot enforce authorization.
- PostgreSQL is authoritative for private servers, encrypted credentials,
  grants, OAuth state, conversation selection, run snapshots, calls, results,
  and audit references. The frontend may cache but must not become authority.
- Catalog artifacts and administrator manifests remain their respective
  authorities; database rows must not silently override them.

### Conversation selection and authorization

- A new conversation inherits its Workspace MCP defaults; a conversation with
  no Workspace starts with no MCP enabled.
- Copying a conversation copies its MCP selection but never credentials.
- Store selection using explicit `inherit` and `custom` modes. `custom` with an
  empty list means all MCP servers are explicitly disabled.
- Enabling a server/tool in a conversation is the authorization boundary. Do
  not add per-call approval dialogs.
- Users may disable individual tools from an enabled server.
- Freeze the selected server/tool/schema/grant snapshot for each accepted run.
- If a selected server is unavailable or requires authorization, reject the
  send before accepting it and offer a disable-and-continue path.
- Re-authorize every tool call server-side. Tool output cannot enable tools,
  alter grants, or expand the run snapshot.

### Native tool loop

- Extend the existing backend provider-native `ToolRoundProvider` loop rather
  than adding another frontend planner.
- Models without native tool calling cannot use MCP. Do not synthesize tool
  calls with prompt JSON and do not switch models automatically.
- Support `model -> tool calls -> results -> same model` until completion.
- Apply these hard MCP limits per run: 8 tool rounds, 32 calls, 30 seconds per
  call, and 120 seconds wall clock.
- Execute trusted read-only calls with at most four concurrent calls. Execute
  write/unknown calls serially.
- Never automatically retry write or unknown calls. A trusted idempotent read
  may retry once only when no result was observed and the wall-clock budget
  permits it.
- On dropped write connections, mark the result `outcome_unknown`, stop the
  run, and never let the model guess or retry.
- Ordinary tool/argument errors return as bounded structured tool results so
  the model may correct the call or select another enabled tool.
- Authentication, permission, unavailable-server, cancellation, budget, and
  unknown-write failures stop the affected branch or run as appropriate.
- When a hard budget is reached, disable further tools and, if enough wall time
  remains, allow one final no-tools model response based on completed results.
- Cross-server chaining is allowed only among tools already enabled in the
  frozen run snapshot.

### Tool discovery, schema, and change handling

- Servers with at most 32 compatible tools expose all compatible tools.
- Larger servers use bounded relevance retrieval plus an internal
  `mcp_tool_search` capability; expose no more than 32 tool definitions to a
  provider round.
- Cache tool schemas by content hash. Accept `tools/list_changed`, but refresh
  only between runs; one run retains its frozen snapshot.
- Use stable internal identity composed from server ID and original tool name.
  Generate deterministic, provider-compatible aliases to avoid collisions and
  length/character violations.
- Strictly validate tool input schemas. Disable only incompatible tools with a
  visible `unsupported` reason; do not silently weaken parameter semantics.
- Treat remote annotations as untrusted. User-created remote tools default to
  write/unknown for retry and scheduling unless a trusted catalog/manifest
  classification overrides them.

### Remote transport, SSRF, and OAuth

- The Go backend connects directly to remote Streamable HTTP MCP servers.
- Reuse the established SSRF protections and strengthen them for every DNS
  resolution, redirect, OAuth metadata fetch, token endpoint, and reconnect.
  User-private endpoints must remain public HTTPS. Never forward credentials
  across origin-changing redirects.
- Support unauthenticated servers, bounded static header/API-key credentials,
  and OAuth authorization code with PKCE.
- Store only a hash of a high-entropy, single-use OAuth state bound to user,
  server, redirect target, and a 10-minute expiry.
- The public callback restores identity from state; it must not depend on a Neo
  Chat browser bearer token being present on the callback request.
- Use standards metadata discovery. Catalog/administrator definitions may
  provide pre-registered client metadata; private definitions may provide a
  validated client ID. Dynamic client registration is disabled by default.
- Refresh access tokens lazily with singleflight locking per credential.
  `invalid_grant` revokes the credential and blocks dependent sends.
- Encrypt user credentials with the existing context-bound provider secret
  vault using a new MCP-specific context. Never return stored secret material.
- Administrator credentials may use only bounded `secretFile` or allowlisted
  `envRef` sources.

### Optional stdio runner

- Run administrator-approved stdio servers through one optional MCP runner,
  not one permanently resident container per server.
- The runner itself may remain resident; spawn server processes on demand,
  reap them after 15 idle minutes, and enforce a 24-hour maximum lifetime.
- Allow at most four active stdio server processes. Use non-root execution,
  read-only filesystem, `cap_drop: ALL`, no-new-privileges, bounded CPU/memory/
  PID limits, isolated work directories, allowlisted environment variables,
  argv arrays, and process-group termination.
- Ship a dedicated immutable runner image pinned by digest with audited
  runtimes and server artifacts. Do not use runtime `npx`/`uvx` downloads,
  Docker socket access, `sh -c`, or arbitrary host mounts.
- Put runner control traffic on a private internal network with an independent
  Docker-secret-backed service token. Do not expose a host port or user bearer
  authentication to the runner.
- A runner failure must not make the whole chat backend unready. Only runs that
  selected a local server fail closed.
- Runner health checks validate its own event loop, manifest, secret, and
  process-manager capacity; they do not eagerly start all stdio servers.

### Results, persistence, UI, and cancellation

- Normalize MCP Text, JSON/structured content, Image, Audio, and Resource Link
  results. Treat all content as untrusted tool data, never as system guidance.
- Never auto-fetch Resource Links.
- Persist at most 256 KiB of normalized inline text/JSON per call. Store larger
  content and media in MinIO, capped by default at 25 MiB per item and 50 MiB
  per call. Apply a smaller token/context budget before sending results back to
  a model.
- Results follow the conversation lifecycle and are not indexed by RAG,
  application search, or training paths. Conversation/account deletion
  cascades to calls, credentials, and MinIO artifacts.
- Persist bounded, redacted audit metadata for 90 days by default. Logs and
  metrics must never contain arguments, result bodies, tokens, custom URLs, or
  high-cardinality server/tool/user labels.
- Add a Tools button and enabled server/tool chips to the composer. Show
  authorization and availability continuously.
- Stream and persist a structured call timeline with `queued`, `running`,
  `succeeded`, `failed`, `canceled`, and `outcome_unknown` states. Show server,
  tool, redacted argument summary, duration, and status; collapse results by
  default.
- Only trusted idempotent read tools may expose manual retry. Write tools have
  no one-click retry.
- User cancellation propagates to model requests, remote MCP requests, queued
  calls, and runner process groups. An already-dispatched write may end as
  `outcome_unknown`.

### Configuration, limits, and operations

- Default quotas: 20 private servers per user, 8 enabled servers per
  conversation, 32 calls per run, 4 concurrent calls per user, 32 exposed
  schemas per provider round, and 5 pending OAuth flows per user. Make safe
  deployment-level limits configurable.
- Provide `MCP_ENABLED`, `MCP_REMOTE_ENABLED`, and `MCP_STDIO_ENABLED` kill
  switches. Disabling MCP never revives the old plugin runtime.
- Load manifests atomically on startup. Provide a standalone validate/preflight
  command. Changes take effect after restart; MVP hot reload is out of scope.
- Invalid manifests fail the MCP subsystem closed without taking unrelated
  chat functionality down.
- Carry `request_id`, `chat_run_id`, and `tool_call_id` through backend,
  runner, audit, SSE, and bounded logs. Extend the existing Prometheus metrics
  with low-cardinality transport/outcome/error-class/read-write labels. Do not
  introduce an OpenTelemetry deployment in this task.
- The frontend-facing API is a strict `/v1/mcp/*` product API using existing
  camelCase DTO, bounded JSON decoder, no-store, and stable error-envelope
  conventions. The runner API is internal only.

### Migration and rollout

- Add new tables with additive, replay-safe up/down migrations before removing
  plugin runtime paths.
- Explicitly migrate browser settings, sessions, and Workspaces to remove
  `activePlugins`, `installedPlugins`, and `pluginConfigs` without mapping them
  to MCP.
- Remove `activePlugins` from PostgreSQL conversation metadata and change copy
  operations to a bounded allowlist so retired fields cannot propagate.
- Implement in reviewable slices behind kill switches, then hard-cut the
  product in one release. Do not expose a user-visible dual stack.
- Release in the order backup, image build/pull, explicit migration, backend,
  optional runner, frontend, and smoke verification.
- Roll back by disabling MCP or restoring the prior images while preserving
  additive MCP tables. Do not run destructive down migrations after traffic.
- Update environment examples, Compose, preflight, backup/restore, release,
  security, and operator documentation.

## Acceptance Criteria

- [ ] No plugin UI, frontend orchestration, `/v1/plugins*` route, built-in
      plugin runtime, or active OpenAPI plugin execution remains.
- [ ] A fake remote Streamable HTTP MCP server completes a native multi-round
      tool call through the real backend chat stream and persisted timeline.
- [ ] A fake stdio MCP server completes the same flow through the hardened
      runner and private control API.
- [ ] Models that lack native tool calling cannot send with MCP enabled and do
      not receive prompt-simulated tool instructions.
- [ ] OAuth discovery, PKCE, hashed state, callback, refresh singleflight,
      revoke, expiry, and concurrent refresh cases pass offline tests.
- [ ] User-private endpoint validation rejects HTTP, private/link-local/
      metadata/multicast targets, DNS rebinding, unsafe redirects, and
      cross-origin credential forwarding.
- [ ] Run limits, read concurrency, write serialization, read retry, write
      non-retry, cancellation, and `outcome_unknown` behavior are deterministic.
- [ ] Tool list changes affect only subsequent runs, schema conflicts are
      isolated, aliases are stable, and large servers expose at most 32 tools.
- [ ] Tool results are bounded, untrusted, redacted from diagnostics, stored in
      MinIO when required, and removed with their conversation/account.
- [ ] Workspace grants and conversation selections are authorized by the
      backend, including explicit-empty, inheritance, copy, and cross-user
      denial cases.
- [ ] Runner authentication, crash recovery, process-group cleanup, resource
      limits, no-host-port topology, and optional availability pass tests.
- [ ] Fresh migration, replay, one-step down/re-up on disposable data, plugin
      metadata cleanup, and rollback-image compatibility are proven.
- [ ] Frontend tests cover Tools management, chips, auth/unavailable states,
      timeline rendering, cancellation, migration, and blocked sends.
- [ ] Compose renders with the example and active environment files, and
      backup/restore includes MCP tables and result artifacts.
- [ ] Backend `go vet ./...` and `go test ./...` pass.
- [ ] Frontend format, lint, typecheck, test, and production build pass.
- [ ] `bash mm-chat/scripts/verify-standalone.sh --full` passes.

## Definition of Done

- Product behavior, migrations, security boundaries, runtime topology, tests,
  and operator documentation match this PRD.
- Relevant backend/frontend/operations Trellis specifications are updated to
  record the new MCP contracts and the intentional MCP-specific differences
  from the existing product Tool loop.
- No real secrets, chat data, user files, or live runtime state are modified or
  committed.
- The final diff has completed Trellis quality verification and is organized
  into reviewable conventional commits approved by the user.

## Technical Approach

Use `github.com/modelcontextprotocol/go-sdk` v1.7.0 as the protocol and
transport implementation. Keep OAuth persistence and policy behind Neo Chat
interfaces because client-side OAuth is documented as experimental in that SDK
release. Extend the current backend provider-native ToolRoundProvider seam and
process-step/SSE contracts rather than retaining the frontend plugin planner.

Implement MCP as backend-owned modules for manifests/catalog, storage,
credentials/OAuth, remote transport, tool snapshots/search, execution, and
result persistence. Add a small standalone runner command that shares the MCP
transport/process-management package but owns no user/session authority.

The existing chat Tool Loop specification currently has no product-level round
cap and requires approval before side-effect tools are registered. MCP is an
intentional, separately authorized extension: the explicit conversation
server/tool selection replaces per-call approval, and MCP-specific hard limits
apply whenever MCP participates in a run. Existing Web, Knowledge, and Memory
semantics must not regress.

## Decision (ADR-lite)

**Context**: Neo Chat's plugin implementation is a frontend one-shot planner
over proprietary OpenAPI/HTTP execution. It cannot provide MCP interoperability,
native same-model continuation, secure OAuth lifecycle, authoritative
conversation grants, or hardened local stdio execution.

**Decision**: Hard-cut to a backend-owned MCP Tools client. Remote Streamable
HTTP connects from the Go backend; approved local stdio servers run through one
optional hardened runner. Persist authoritative state in PostgreSQL/MinIO,
reuse the existing provider-native Tool loop and secret vault, and expose a
conversation-level Tools UI without per-call approval.

**Consequences**: The change is intentionally large and cross-layer. It adds a
backend Workspace authority, OAuth storage, runner image/service, normalized
tool-result persistence, and stricter migrations. Old plugin functionality is
not preserved. Resources/Prompts and broader MCP features remain available for
future work through isolated extension points rather than premature MVP scope.

## Implementation Plan

1. Foundation: specifications, SDK dependency, manifest/catalog validation,
   normalized schema, repositories, Workspace grants, and configuration gates.
2. Remote runtime: SSRF-safe Streamable HTTP client, schema normalization,
   snapshots/search, credentials, and OAuth lifecycle.
3. Local runtime: shared process-control package, runner command/image,
   internal authentication, Compose topology, and crash/cancel handling.
4. Chat integration: native MCP Tool rounds, budgets, scheduling, result
   persistence, audit/metrics, SSE events, and recovery semantics.
5. Frontend: Tools management, conversation selections, composer chips,
   timeline states/results, cancellation, and persistence migration.
6. Cutover: delete old plugin UI/API/runtime, retain the rollback table,
   update copy/import/export/backup/restore behavior and documentation.
7. Verification: focused security/integration tests, disposable migration and
   Compose drills, all component gates, and the standalone full gate.

## Out of Scope

- MCP Resources, Prompts, Sampling, Elicitation, and legacy SSE transport.
- Neo Chat acting as an MCP server or public proxy.
- OpenAPI plugin compatibility adapters or long-term plugin/MCP dual runtime.
- User-supplied stdio commands, runtime package downloads, arbitrary host
  mounts, or Docker socket access.
- Automatic third-party MCP marketplace synchronization.
- Prompt-JSON tool simulation or automatic model switching.
- Full OpenTelemetry/collector rollout and manifest hot reload.
- Automatic migration of old plugin definitions or credentials into MCP.
- Deleting `plugin_registry` in the first MCP release.

## Research References

- [`research/current-system.md`](research/current-system.md) — current plugin,
  chat loop, state, OAuth, storage, and deployment facts.
- [`research/mcp-sdk.md`](research/mcp-sdk.md) — official Go SDK version,
  feature support, transport seams, and OAuth stability risk.
- [`research/security-operations.md`](research/security-operations.md) — SSRF,
  secret, runner, Compose, observability, migration, and rollback constraints.

## Technical Notes

- Product source remains under `mm-chat/`; runtime paths under `mm-chat/data/`,
  `mm-chat/secrets/`, `mm-chat/backup/`, and `.env.single-server` are protected.
- Existing process-step and provider continuation contracts are documented in
  `.trellis/spec/backend/chat-tool-loop.md` and
  `mm-chat/docs/contracts/chat-tool-loop.md`.
- Frontend persisted-state migrations must use the established storage-version
  path and runtime normalization rather than component-side deletion.
- Project documentation and Trellis specifications are written in English.

