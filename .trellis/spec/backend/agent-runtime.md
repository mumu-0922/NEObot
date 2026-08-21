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
Migration head: 101_chat_agent_transcript_blocks

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
101.

Cross-layer changes also require frontend format/lint/typecheck/test/build and
`bash mm-chat/scripts/verify-standalone.sh --full`.

### Wrong vs correct

```text
Wrong: Agent Center -> OCI Runner -> Broker -> downloadable host path
Correct: Chat Agent -> bounded local Tool -> user-owned File -> message artifact

Wrong: inspect Shell command text -> guess mutation -> force verification
Correct: foreground result -> synchronous boundary; background Job -> exact completed output

Wrong: copy stdout/stderr or Terminal arguments into generic process detail
Correct: typed redacted Terminal card -> durable ProcessStep -> same live/replay card

Wrong: append reasoning to message metadata only and render it below a grouped Tool panel
Correct: Provider reasoning -> durable indexed assistant blocks -> flat live/reload transcript

Wrong: overwrite the background start event when job_output arrives
Correct: retain immutable events -> merge display cards by exact jobId

Wrong: DROP ... CASCADE after a broad agent_* match
Correct: lock -> exact manifest/data validation -> explicit drops -> forward repair -> head 101
```
