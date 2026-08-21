# Chat Agent Runtime Contract

## Scenario: execute installed Skills through the ordinary Chat Agent

### Scope / trigger

Apply when changing `internal/chat`, `internal/localskills`,
`internal/skillsupply`, `/v1/skills/*`, workspace Files/Jobs/Terminal,
artifact publication, or migration `098`.

### Signatures

```text
Conversation config: toolMode = "chat" | "agent"
Skill API: /v1/skills/*
Agent local Tools: skill, file_read, file_write, file_edit, file_search,
                   terminal, job_list, job_output, job_kill, publish_file
Migration head: 102_host_workspaces

Transcript v2 events: context.injected, assistant.chunk,
                      assistant.block.completed
Transcript v2 start marker: turn.started.payload.transcriptVersion = 2
```

### Contracts

- Agent is a persisted Conversation runtime policy, not a separate control
  application. Chat mode physically omits Agent Tools; Agent mode adds Skills,
  File, Terminal, Job, Browser/MCP, Goal, and `publish_file`.
- Unsupported Tool models downgrade the effective Turn to Chat without changing
  stored intent or returning Skill/MCP admission conflicts.
- Keep `internal/agents` (Assistant library), `internal/localskills`, and
  `internal/skillsupply`. The Skill Store remains server-authoritative through
  `/v1/skills/*` and must not regain Runs, Schedules, Learning, Shadow, Canary,
  Runner, OCI, delegation, or Subagent controls.
- `local_direct` runs as the Backend UID/GID under two explicit roots. Enforce
  timeout, output, call, round, and concurrency bounds plus process-group
  cancellation/reaping. Jobs remain process-local and must never claim restart
  durability.
- `AGENT_LOCAL_WORKSPACE_HOST_ROOT`, when nonempty, must be the clean absolute
  Host path for the one directory already mounted at `/workspace`. Resolve only
  canonical Linux absolute and WSL UNC inputs below it into relative names
  before the existing `os.Root`/CAS/symlink checks. It is never a second root.
- Local execution is not a Sandbox. Never add `sudo`, a container socket,
  host-wide personal/secret binds, privileged execution, automatic OS package
  installation, or per-Skill isolation claims.
- Foreground Terminal success is a synchronous execution boundary and does not
  create a Completion Policy mutation. Do not classify arbitrary Shell text as
  read-only/write. Structured File/MCP writes remain evidence-gated; foreground
  Terminal may check them. Background Terminal remains outstanding by exact Job
  ID until successful `job_output(status=completed)` evidence is explicitly
  recorded. Successful local Tool Results expose their exact Provider-only
  `evidenceToolCallId`. Process events may retain only versioned, Tool-owned,
  allowlisted presentations: bounded/redacted Terminal command, stable cwd,
  final stdout/stderr transcript and execution flags; bounded workspace-relative
  File preview/diff/search summaries; Job lifecycle output; Skill/Goal summaries;
  or safe MCP/Browser fallbacks. Provider-only raw Results, credentials, exact
  retrieval queries, private Server refs and materialized Workspace/Host/Skill
  paths never enter ProcessStep, durable Agent events, or SSE.
- Foreground Terminal may emit transient running ProcessStep snapshots from its
  bounded pipe capture. Keep a 64-byte sanitizer holdback, coalesce updates to
  at most one per 75 ms unless 16 KiB becomes stable, and never block command
  pipes on a slow SSE consumer. Transient snapshots are SSE-only; the final
  bounded/redacted head+tail snapshot is the sole durable transcript authority.
- Every Run SSE frame uses the JSON `sequence` as SSE `id` and enters a
  process-local 1,024-frame/4 MiB ring. Exact-user reconnect uses
  `GET /v1/chat/runs/{runId}/events?after={sequence}` and reauthorizes the
  Conversation. Retain completed streams for 30 seconds, cap finished entries
  and subscribers, close slow subscribers rather than blocking execution, and
  emit `stream.gap/cursor_evicted` when the requested prefix is gone. Never
  write transient chunks to `chat_agent_events`; converge on the terminal
  persisted Message snapshot after a gap.
- For an admitted timeline canary, every event persisted through the active
  recorder is also sent as `agent.event` with its durable `eventId`, durable
  sequence, UTC occurrence time, and already-normalized payload. The outer SSE
  sequence remains the reconnect cursor. Emit `assistant.message` and
  `turn.ended` before the terminal Message frame, and include the same event set
  in that terminal Message so live, reconnect, and reload share one authority.
- Transcript v2 projects one flat durable sequence rather than grouping Tools
  inside a summary panel. Record only sanitized context segments that actually
  reached the Provider and reasoning text actually returned by the Provider.
  Each reasoning block uses `assistant.chunk(block-start|reasoning-delta)` plus
  `assistant.block.completed`, carries a positive block index and Provider
  round, is bounded to 64 KiB per event/1 MiB per Turn, and closes before the
  next Tool or answer-text boundary. Unsupported Providers produce no fake
  `Think` row. DeepSeek native Tool rounds must not force thinking off; preserve
  `reasoning_content` through the existing continuation exchange.
- Migration `101` redefines `chat_agent_start_turn` so every new Turn receives
  the exact numeric `transcriptVersion: 2` start marker. Frontend admission uses
  it for Tool-only Turns; historical unmarked Tool events remain on the legacy
  renderer. Presence of a v2-only Context/assistant event is sufficient for a
  partial replay whose `turn.started` prefix is unavailable.
- `context.injected` accepts only `system-prompt|skill-catalog|skill-instruction|
  runtime-context`, a bounded label/content pair, and explicit truncation.
  Redact credentials and replace the configured Host workspace root before
  persistence/SSE; never expose materialized Skill paths or Provider-internal
  encrypted/redacted thinking.
- Admitted canaries receive no duplicate `process.step.updated` or
  `tool.call.updated` frame for persisted facts. Sanitized, coalesced Terminal
  snapshots use `agent.progress` and remain transient; the next durable event
  replaces the same step. Continue writing the bounded metadata ProcessTrace
  only as flag-off rollback state, but omit it from typed canary DTOs. A
  non-canary/control request retains the legacy frames and metadata projection.
- A background Terminal start is presented as a Job card and persists only its
  sanitized command/cwd plus `jobId`, process-local status, timestamps,
  duration, exit flags, and bounded transcript. Later `job_output`/`job_kill`
  calls remain immutable Tool events; the frontend merges their cards by exact
  `jobId` without mutating history. Startup Turn recovery changes a persisted
  `running`/`stopping` process-local Job to `interrupted`, even when the Tool
  call that launched it already succeeded. Do not add a second Job authority.
- A destructive Terminal call in `smart` mode creates a durable five-minute
  approval before execution and waits on the same Tool call. `Allow once`
  bypasses the smart approval check exactly once; policy-permitted `Allow for
  conversation` creates an exact Conversation + Tool name + risk-class grant.
  Deny, expiry, cancellation and `restart_denied` do not execute. The hard
  blocklist is earlier authority and is never approval-bypassable.
- `publish_file` accepts only workspace-relative regular files, persists through
  the existing user-owned File/object-store path, and attaches only successful
  outputs to the assistant message. Cross-user and stale/deleted access fails.
- Applied migration SQL is byte-immutable. The production checksum for `096`
  is `f7c6227d3dd559cb53b22a28af1d77bc570d45a42288bf1f348b22136ef1b042`;
  runtime corrections belong in forward migration `099`, never in the old
  pair or `schema_migrations.checksum`.
- Migrations `084`–`095` remain immutable history. Migration `098` uses explicit
  object whitelists, locks before counting, permits only two bootstrap
  singleton rows, aborts on any fact row or schema drift, uses no wildcard
  deletion/`CASCADE`, and never names `chat_agent_*`.
- `098.down` is an irreversible no-op. Reapplying after down must succeed only
  when every retired object is already absent.
- `099` idempotently repairs the two Chat event gateways, reasserts hardened
  `search_path` and least-privilege grants, and has a forward-only no-op down.
- `100` stores mutable approval/CAS and exact conversation-grant authority but
  no raw Tool payload. The immutable Agent event remains presentation/replay
  authority. API runtime has no table DML and may execute only create, decide,
  and restart-recovery gateways. First valid decision wins; duplicate/conflict
  tabs return current authority. Startup denies all pending rows.
- Typed Harness timeline exposure requires both
  `AGENT_TIMELINE_ENABLED=true` and an exact authenticated UUID in
  `AGENT_TIMELINE_CANARY_USER_IDS`. Empty, malformed, duplicate, disabled, or
  non-matching configuration fails closed. Continue immutable Agent event
  writes while gated off, omit `agentEvents`/presentations from responses, and
  do not create an invisible approval wait for a non-canary turn.
- Manual Tool retry is Backend-authorized, never inferred from a frontend Tool
  name or risk label. The first admitted Tool is a failed `local_direct`
  `file_read` with `file_not_found` or `execution_failed`: Backend rehydrates
  only its sanitized workspace-relative path/offset from the owner-scoped
  terminal event, creates one deterministic retry Message per source event,
  assigns a new call ID, and persists `retryOf`. Write/execute/MCP,
  `outcome_unknown`, malformed, cross-user, non-canary, and legacy events have
  no retry affordance and the retry route fails closed.
- Canary security acceptance must pass one hostile presentation through the
  real local presenter, ProcessStep sanitizer, Tool-event builder, event payload
  bound, durable JSON shape, and frontend renderer. The fixture must cover
  high-confidence secrets, runtime-owned host-path aliasing, ANSI/control
  filtering, 64 KiB transcript retention, the 256 KiB event cap, unknown MCP
  summary-only fallback, raw argument/result exclusion, and artifact-body
  exclusion. Keep safe status visible when content is hidden. Legacy
  ProcessStep transport remains a control/rollback surface until these gates
  pass one focused exact-user canary acceptance session. Do not widen beyond
  exact users or delete the fallback code before eligible evidence exists.

### Validation matrix

| Condition | Required result |
| --- | --- |
| Chat mode or Tool-incapable model | no local/MCP/Goal Tool preparation |
| Agent enabled, no installed Skills | File/Terminal/Job remain; Skill catalog is empty |
| invalid root/shell/limits | Backend startup fails |
| destructive command in smart mode | durable awaiting-approval event; execute only after exact allow |
| duplicate/conflicting approval decision | return first terminal decision; do not overwrite history |
| pending approval at Backend restart | `denied/restart_denied`; no execution |
| timeline flag false or canary empty/non-matching | legacy ProcessStep projection; no typed cards or approval wait |
| timeline flag true and exact UUID matches | typed live/reload cards and approval controls |
| Provider returns reasoning around a Tool call | separate ordered `Think -> Tool -> Think` rows live and after reload |
| Provider returns no reasoning on a marked v2 Turn | no fabricated Think row; Tool/final answer order remains authoritative |
| historical Tool-only Turn has no v2 marker | retain legacy renderer and legacy reasoning |
| context or reasoning contains a secret/Host root or exceeds bounds | redact/alias/truncate before event append and DOM rendering |
| failed safe `file_read` retried twice | one retry Message/Tool execution; new call ID links to immutable source through `retryOf` |
| write/execute/MCP/outcome-unknown/cross-user retry | no affordance; Backend denies without Tool execution |
| reconnect cursor retained | replay exact suffix in original sequence |
| reconnect cursor evicted | explicit unsequenced gap, retained suffix, final Message convergence |
| slow reconnect subscriber | close subscriber; Run and Provider pipes continue |
| restart after successful background start but before terminal Job result | preserve events; project exact Job as `interrupted` |
| later output/kill events share exact Job ID | one display lifecycle; underlying Tool events remain immutable |
| foreground Terminal-only task | Tool-free final answer; no `verify_completion` loop |
| background Terminal plus foreground check | background remains unverified until exact completed `job_output` |
| Backend shutdown with active Job | entire process group canceled and reaped |
| path traversal/symlink/non-regular publish | reject; no File row/object |
| Host/WSL alias below configured workspace | resolve to the same relative File/workingDir path |
| alias outside Host root, Windows drive, or traversal | reject before filesystem/command access |
| unknown/malformed/version-unsupported presentation | drop presentation; retain the valid ProcessStep |
| hostile/oversized Tool presentation | preserve safe status; redact or reject content before durable JSON/SSE/export/DOM |
| nonempty legacy fact table at 098 | whole migration rolls back |
| already-retired schema re-up | successful no-op |

### Good / base / bad cases

- **Good**: Agent mode loads an admitted Skill, maps an authorized pasted Host
  path to a workspace-relative name, edits that file, verifies it, publishes
  it, and returns an authenticated download card.
- **Base**: Agent mode has no installed Skills; bounded File/Terminal/Job Tools
  still work, while Chat mode exposes none of them.
- **Base**: Agent runs `pwd` and `git status --short` in foreground, observes the
  redacted workspace result, shows the same bounded Terminal card live and
  after reload, and answers without manufacturing verification.
- **Bad**: route local execution through a second control service, claim Sandbox
  isolation, parse arbitrary Shell text as a reliable mutation classifier,
  verify a running background Job with unrelated foreground output, mount a
  Home/parent directory containing unrelated secrets, treat an alias as a
  second root, or let migration `098` delete by wildcard.

### Required tests

```bash
bash mm-chat/scripts/verify-agent-local-runtime.sh
bash mm-chat/scripts/verify-legacy-agent-cleanup-postgres17.sh
bash mm-chat/scripts/verify-chat-artifacts-postgres17.sh
bash mm-chat/scripts/verify-chat-agent-approvals-postgres17.sh

cd mm-chat/backend
GOCACHE=/tmp/neo-chat-go-cache go vet ./...
GOCACHE=/tmp/neo-chat-go-cache go test ./...
```

The local Runtime suite must also prove foreground Terminal-only completion,
`file_write -> terminal -> verify_completion`, exact local
`evidenceToolCallId`, background Job ID/status gating, typed Terminal
presentation redaction/bounds, raw-output absence, and live/reload parity.
Cursor tests must prove after-sequence replay, duplicate suppression, bounded
eviction gap, exact-user authorization, terminal grace replay, browser
auto-resume, and final-snapshot convergence without per-chunk database events.
Job lifecycle tests must prove background Terminal -> Job presentation,
start/output/kill merge by exact ID, bounded/redacted transcript retention,
input-event immutability, and restart reconciliation of unresolved process-
local status.
Transcript tests must prove strict payload unions, DeepSeek Tool-round thinking,
context/reasoning redaction and bounds, flat sequence projection, duplicate
event rejection, marker-gated legacy compatibility, and byte-equivalent
live/reload rendering. The PostgreSQL 17
event-log drill must append all three migration-101 event types through the
hardened runtime gateway, assert the v2 start marker, and replay cleanly to head
102.

Cross-layer changes also require frontend format/lint/typecheck/test/build and
`bash mm-chat/scripts/verify-standalone.sh --full`.

### Wrong vs correct

```text
Wrong: treat execution=true as proof that permission presets are enforced
Correct: route bound Tools only when execution=true; keep permissionModes empty
         until each advertised preset passes its independent enforcement probes

Wrong: inspect Shell command text -> guess mutation -> force verification
Correct: foreground result -> synchronous boundary; background Job -> exact completed output

Wrong: copy stdout/stderr or Terminal arguments into generic process detail
Correct: typed redacted Terminal card -> durable ProcessStep -> same live/replay card

Wrong: append reasoning to message metadata only and render it below a grouped Tool panel
Correct: Provider reasoning -> durable indexed assistant blocks -> flat live/reload transcript

Wrong: overwrite the background start event when job_output arrives
Correct: retain immutable events -> merge display cards by exact jobId

Wrong: DROP ... CASCADE after a broad agent_* match
Correct: lock -> exact manifest/data validation -> explicit drops -> forward repair -> head 102
```

## Scenario: operate the interactive WSL Agent Host control plane

### Scope / trigger

Apply when changing `cmd/agent-host`, `internal/agenthost`, the internal Host
Unix-socket protocol, Host workspace canonicalization, or the pre-routing
Runner capability boundary.

### Signatures

```text
GET  /internal/v1/capabilities
POST /internal/v1/workspaces/resolve
POST /internal/v1/directories/browse
POST /internal/v1/directories/pick-native
POST /internal/v1/tools/execute

Protocol version: 1
Runner id:         [a-z][a-z0-9-]{2,63}
Request limit:     16 KiB
Response limit:    64 KiB
Execution request: 128 KiB
Execution response:72 MiB
Workspace path:    1..4096 valid UTF-8 bytes without controls
```

### Contracts

- Transport is HTTP/1.1 over a private Unix socket; there is no TCP listener.
- Every request uses an independent bearer token. Never reuse an MCP token,
  Provider credential, browser Session, or Backend API token.
- Load the token only from an absolute canonical non-symlink regular file with
  exact mode `0600`, owner equal to the effective UID, and a bounded value.
  Open the final component with `O_NOFOLLOW` and validate the opened file.
- Pin the expected `runnerId` on every successful client response. Unknown
  JSON fields, duplicate keys, trailing documents, oversized bodies, malformed
  capability labels/modes/limits, unknown remote error codes, and invalid
  workspace fingerprints are protocol failures.
- `capabilities` advertises only implemented facts. The interactive control
  plane reports Workspace resolve, WSL directory browse, and the native
  Windows picker when their Host dependencies exist. An active
  `ExecutionManager` reports `execution=true`; `permissionModes` remains empty
  until the separate enforcement probes pass.
- Directory browse starts at the ordinary Host user's home for an empty path,
  returns at most 256 sorted directory-only entries, and never reads file
  contents. Native picker runs a fixed PowerShell/WinForms command with no
  caller interpolation and resolves the selection through the same `wslpath`
  and canonicalization boundary.
- The Host alone converts and probes Host paths. Invoke the fixed absolute
  `wslpath` executable with `-u`, `--`, and the path as a distinct argv value;
  never invoke a shell. Resolve symlinks, require an existing absolute
  directory, and classify original Windows or `/mnt/<drive>` paths as
  `windows-mounted`.
- Workspace identity is
  `sha256(runnerId || NUL || canonicalPath)`. The client recomputes the exact
  lowercase fingerprint. It is a deduplication key, not integrity or inode
  proof; every future execution must re-resolve durable authority.
- A Tool execution request carries the immutable canonical path/fingerprint,
  camel-case user/Conversation Job scope, allowlisted Tool name, strict
  arguments, Backend-owned approval bit, and an optional contained relative
  active Skill root. Re-resolve the Workspace on every call and require the
  returned canonical path and fingerprint to match exactly.
- Reuse `localskills.Executor` at the Host canonical root so File CAS/symlink
  checks, Terminal hard blocks/approval, time/output bounds, process-group
  cancellation, artifacts, and process-local Jobs remain one implementation.
  Emit `mode=host_workspace`; clear Docker Workspace aliases from the adapter
  config and redact Host/Skill paths before Provider or durable presentation.
- Error bodies use allowlisted stable codes and generic messages. Do not return
  submitted Host paths, token material, converter output, or raw OS errors.
- Compose connects through the exact read-only socket directory and independent
  credential secrets. Bound Conversations execute on Host; ungrouped legacy
  Conversations retain `local_direct`. Runner loss makes bound Tools fail
  closed without a Docker-path fallback.

### Validation and error matrix

| Condition | Required result |
| --- | --- |
| missing/wrong bearer token | `401 AGENT_HOST_UNAUTHORIZED`; no token echo |
| query, unknown field, duplicate field, trailing JSON, oversize | generic `400 AGENT_HOST_REQUEST_INVALID` |
| protocol version other than `1` | `409 AGENT_HOST_PROTOCOL_UNSUPPORTED` |
| missing/non-directory/relative/control path | sanitized `WORKSPACE_PATH_INVALID` or `WORKSPACE_PATH_UNAVAILABLE` |
| Windows input without working fixed `wslpath` | `503 WINDOWS_PATH_INTEROP_UNAVAILABLE` |
| response Runner id differs from the pinned id | client `ErrHostProtocol` |
| fingerprint is non-hex, uppercase, wrong length, or does not recompute | client `ErrHostProtocol` |
| socket unavailable/cancelled request | unavailable/context error; no fallback |
| native picker cancellation | successful `cancelled=true`; no Workspace mutation |
| execution canonical path/fingerprint drift | `WORKSPACE_AUTHORITY_INVALID`; no Tool execution |
| active Skill root is absolute, escaping, missing, or symlinked | `ARGUMENTS_INVALID`; no process |
| Host loss after immutable binding | durable failed/interrupted Tool; never Docker `/workspace` |

### Good / base / bad cases

- **Good**: `/home/user/project`, its symlink alias, and the matching canonical
  path resolve to one Runner-bound fingerprint without changing the project.
- **Good**: `D:\\project` is passed as one `wslpath` argv value and returns a
  canonical `/mnt/d/project` descriptor while retaining the Windows display
  path.
- **Base**: the Host is stopped; ungrouped legacy `local_direct` remains, while
  every bound Conversation and Host control operation fails closed.
- **Bad**: advertise `workspace-write` before filesystem and shell sandbox
  probes enforce it on both WSL filesystems and DrvFS.
- **Bad**: trust a path, fingerprint, error message, or Runner identity merely
  because it arrived from the authenticated socket.

### Tests required

```bash
cd mm-chat/backend
go test -race ./internal/agenthost ./internal/hostworkspace ./internal/chat \
  ./internal/httpserver ./cmd/agent-host ./cmd/api
go vet ./internal/agenthost ./internal/hostworkspace ./internal/chat \
  ./internal/httpserver ./cmd/agent-host ./cmd/api
```

Tests must cover bearer failures/non-disclosure, strict JSON including
duplicates and limits, stable error mapping, Runner mismatch, a real
Unix-socket client/server round trip, active/stale/replaced sockets, unsafe
socket/token ownership/modes/symlinks, Windows conversion, symlink alias
deduplication, invalid paths, exact fingerprint recomputation, and Host-path
non-disclosure.
Execution tests additionally prove actual Host cwd, File read/write, Job
lifecycle, cancellation, Skill-root containment, `host_workspace` live/reload
presentation, and loss without local fallback.

### Wrong vs correct

```text
Wrong: exec.Command("sh", "-c", "wslpath -u " + userPath)
Correct: exec.CommandContext(ctx, absoluteWslpath, "-u", "--", userPath)

Wrong: authenticated socket response -> trust runnerId/path/fingerprint/message
Correct: strict bounded response -> pin runnerId -> validate facts -> recompute fingerprint

Wrong: Host unavailable -> canonicalize the submitted path inside Docker
Correct: old conversations remain unchanged; Host binding/browse/pick fail closed

Wrong: bound Host call fails -> retry the same Tool through local_direct
Correct: persist the Host failure -> restore the same pinned Runner -> retry explicitly
```

## Scenario: persist converged Host Workspaces before execution routing

### Scope / trigger

Apply when changing `internal/hostworkspace`, `/v1/workspaces*`, migration
`102_host_workspaces`, `conversations.workspace_id`, or any future call to
`LockConversationExecutionWorkspace`.

### Signatures

```text
GET    /v1/workspaces
GET    /v1/workspaces/{workspaceId}
PUT    /v1/workspaces/{workspaceId}
PATCH  /v1/workspaces/{workspaceId}
DELETE /v1/workspaces/{workspaceId}
POST   /v1/workspaces/{workspaceId}/bind
PUT    /v1/workspaces/{workspaceId}/conversations/{conversationId}

Binding status: unbound | bound
Migration head: 102_host_workspaces
```

### Contracts

- Extend the existing `workspaces` table and visible Workspace identity in
  place. Preserve UUID, name, system prompt, files, color, Search/Reasoning
  settings, and `conversations.workspace_id`; never create a competing Project
  table or infer a path from the name.
- Legacy browser import is current-user, idempotent, and unbound. Equivalent
  replay leaves revision unchanged. Settings updates, bind, and delete use an
  exact positive compare-and-set revision.
- The Host Runner alone canonicalizes/probes an input path. Persist its pinned
  Runner id, canonical/display paths, kind, and recomputed fingerprint as one
  all-or-null tuple. Binding is one-way; aliases conflict through the active
  `(owner_user_id, runner_id, directory_fingerprint)` unique index.
- Authorize the current-user Workspace, expected revision, and unbound state
  before invoking the Host resolver. Missing/cross-user/stale/already-bound
  requests must not become Host filesystem probes.
- Keep visible grouping in `conversations.workspace_id`. On first Host Agent
  execution, transactionally copy the selected bound Workspace into the
  `agent_workspace_*` execution snapshot. Repeating the same lock is
  idempotent; a different Workspace is drift and must fail.
- Scope every query and mutation to the authenticated owner. Use the composite
  Conversation/Workspace ownership FK and column-limited runtime grants. The
  runtime role must not change owner/team identity or physically delete rows.
- Strict JSON rejects unknown/duplicate keys, trailing documents, wrong media
  types, queries, and oversize. Browser errors are generic and never echo a
  submitted Host path, SQL, Runner transport detail, or raw OS failure.
- Until the Backend mounts the Host socket and constructs a pinned resolver,
  bind returns `503 HOST_WORKSPACE_UNAVAILABLE`. Do not canonicalize inside
  Docker or silently bind the static `local_direct` root.
- Soft deletion never mutates the Host directory. Migration down refuses with
  `HOST_WORKSPACE_ROLLBACK_BLOCKED` once imported settings, Host binding, or an
  execution snapshot exists.

### Validation and error matrix

| Condition | Required result |
| --- | --- |
| malformed UUID/settings/body or query | `400 INVALID_WORKSPACE_REQUEST` |
| missing or cross-user Workspace/Conversation | generic `404` |
| stale revision | `409 WORKSPACE_REVISION_CONFLICT` |
| second bind | `409 WORKSPACE_ALREADY_BOUND` |
| canonical directory alias duplicate | `409 WORKSPACE_DIRECTORY_REGISTERED` |
| unbound execution lock | `409 WORKSPACE_UNBOUND` |
| grouping conflicts with execution snapshot | `409 CONVERSATION_WORKSPACE_LOCKED` |
| resolver missing or Host unavailable | `503 HOST_WORKSPACE_UNAVAILABLE` |
| Host stable error/protocol violation | sanitized `502` |
| down after durable state | atomic `HOST_WORKSPACE_ROLLBACK_BLOCKED` |

### Good / base / bad cases

- **Good**: import one legacy browser Workspace, preserve every setting, bind
  it once through the pinned Runner, group a Conversation, and lock the exact
  Runner/canonical-path snapshot before its first Host Tool call.
- **Base**: legacy Workspace remains `unbound`; ordinary chat rendering works,
  while Host binding/execution is unavailable and no Docker fallback occurs.
- **Bad**: resolve a path in Backend, overwrite a bound path, expose raw path
  failures, or use mutable grouping as the execution `cwd` after work starts.

### Tests required

```bash
cd mm-chat/backend
go test -race ./internal/hostworkspace ./internal/agenthost \
  ./internal/migration ./internal/httpserver ./cmd/api
go vet ./internal/hostworkspace ./internal/agenthost \
  ./internal/migration ./internal/httpserver ./cmd/api
```

The disposable PostgreSQL 17 test must apply through `102`, run Repository
mutations as `go_api_runtime`, prove empty down/re-up, idempotent import, alias
uniqueness, cross-user denial, execution lock/no drift, in-use delete denial,
and guarded down after durable state.

### Wrong vs correct

```text
Wrong: frontend Workspace + separate Host Project table + copied conversations
Correct: one existing Workspace id + additive Host binding + execution snapshot

Wrong: GRANT UPDATE ON workspaces -> rely only on Go ownership filters
Correct: owner-scoped SQL + composite FK + only required insert/update columns

Wrong: Host resolver not wired -> accept/clean the path inside Docker
Correct: return 503 until the pinned Host authority is reachable
```

## Scenario: route immutable Workspace Agent Tools through the Host

### Scope / trigger

Apply when changing Chat selection of `localToolExecutor`,
`agenthost.ExecuteTool`, Host File/Terminal/Job execution, execution-mode
presentation, or Runner-loss behavior after a Conversation has a Workspace.

### Signatures

```text
Chat mode:                 no Agent local/Host Tools
Agent + workspace_id:      host_workspace
Agent + no workspace_id:   local_direct (legacy rollback only)
Host capability:           execution=true, permissionModes=[]
Runtime environment:       NEO_CHAT_AGENT_RUNTIME=host_workspace
                           NEO_CHAT_HOST_WORKSPACE=1
```

### Contracts

- Probe Host status before locking the first bound Agent Turn. Atomically lock
  `agent_workspace_*`, require the locked Runner id to match the current pinned
  Host, then construct a Host adapter. Do not select by mutable Workspace state
  after the snapshot exists.
- A bound route returns no Docker executor on any status, binding, transport,
  protocol, or execution error. Never retain `WorkspaceRoot=/workspace` or its
  Host alias in the Host adapter config because prompt path rewriting would map
  the wrong project into the selected Host Workspace.
- Materialized Skills remain Backend-authorized. Convert the Backend absolute
  active Skill directory to one contained relative name; the Host joins it to
  its canonical Skill root, rejects symlinked/escaping directories, and adds
  only the Host path to the child environment.
- Emit `host_workspace` in ProcessStep and typed Tool updates. Backend and
  frontend sanitizers must accept the same Terminal/File/Job/Skill cards live
  and after reload. Manual safe-read retry remains `local_direct`-only until the
  retry route can resolve the immutable Host binding.
- Buffered Host foreground output may feed the existing callback only after
  completion in this slice. The sole durable authority is still the final
  bounded transcript; do not claim per-chunk Host streaming.

### Validation and error matrix

| Condition | Required result |
| --- | --- |
| bound Workspace, ready matching Runner | every local Agent Tool uses Host adapter |
| missing Service or `execution=false` | `503 HOST_EXECUTION_UNAVAILABLE`; no Provider/Tool execution |
| lock error or Runner mismatch | `409 HOST_WORKSPACE_BINDING_FAILED`; no fallback |
| Tool transport loss after lock | failed/cancelled durable Host Tool; no local mutation |
| ungrouped legacy Conversation | existing `local_direct` executor |
| Tool event `mode=host_workspace` | same strict card live and after reload |

### Good / base / bad cases

- **Good**: selected `/mnt/d/project` executes `pwd`, File CAS, and Jobs there;
  refresh retains detailed `host_workspace` cards.
- **Base**: an old ungrouped Conversation continues on the one configured
  Docker project until migration removes the compatibility route.
- **Bad**: inherit `/workspace` path aliases in a Host adapter, label Host work
  `local_direct`, accept an active Skill symlink, or retry on Docker after loss.

### Tests required

```bash
cd mm-chat/backend
go test -race ./internal/localskills ./internal/agenthost \
  ./internal/hostworkspace ./internal/chat ./internal/httpserver \
  ./cmd/agent-host ./cmd/api
go vet ./internal/localskills ./internal/agenthost \
  ./internal/hostworkspace ./internal/chat ./internal/httpserver \
  ./cmd/agent-host ./cmd/api

cd ../frontend
corepack pnpm exec vitest run src/__tests__/mcpTypes.test.ts \
  src/__tests__/processTrace.test.tsx
corepack pnpm typecheck
```

Assert real Host cwd/file mutation, cancellation, greater-than-16-KiB execution
input, control-limit separation, no Skill symlink escape, no Docker fallback,
Runner-identity fencing, environment mode truth, and byte-equivalent live/reload
Host presentations.

### Wrong vs correct

```text
Wrong: conversation.workspace_id -> read mutable Workspace path for every Tool
Correct: first Agent Turn -> immutable execution snapshot -> every Host Tool

Wrong: Host unavailable -> localSkillExecutor.Execute(request)
Correct: Host unavailable -> durable generic failure -> zero local execution
```
