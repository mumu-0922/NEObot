# Harness Transcript v2

## Goal

Bring neo-chat's Agent execution experience materially in line with DeepSeek
Harness without replacing the existing Agent runtime: every new Agent turn must
render one flat, durable, replay-safe transcript in true execution order,
including sanitized context injection, Provider-returned reasoning blocks,
Tool calls/results, approvals, and the final assistant response.

## Requirements

### Authority and protocol

- Reuse `chat_agent_events` as the sole durable authority for new Agent turns.
- Use a stable relational event envelope plus a tagged, versioned
  `presentation` union. The backend assigns monotonic sequence numbers and
  idempotent event identifiers; timestamps are UTC and durations come from the
  execution-side monotonic clock.
- Produce a sanitized presentation exactly once in the backend and use that
  same value for live SSE, durable replay, and conversation export.
- Preserve old messages through the legacy renderer. Do not synthesize missing
  historical events. Project the new event stream to the old SSE contract for
  one stable release, but never dual-write two durable authorities.
- Support current and previous presentation schema versions. Unknown cards or
  versions render a safe summary without raw payload data.

### Execution state and recovery

- Normalize execution states as `pending`, `waiting_approval`, `running`,
  `succeeded`, `failed`, `timed_out`, `cancelled`, `interrupted`, and `unknown`.
- Preserve provider-round grouping and actual call order; never imply serial
  execution for concurrent calls.
- Stream ordered stdout/stderr chunks with source labels and chunk sequence
  numbers. Keep a bounded active-turn ring buffer for cursor reconnect; persist
  only the final bounded snapshot.
- A disconnect may replay retained chunks. If chunks were evicted, show an
  explicit gap and converge on the final snapshot.
- Merge background Job operations into a lifecycle view keyed by job ID while
  retaining the underlying immutable events. Persist minimal Job metadata and
  reconcile after restart; use `unknown` or `interrupted` when the outcome
  cannot be proved.
- Cancel stops the current Agent turn and propagates cancellation to active
  Tools. Retry creates a new call ID linked through `retry_of` and is available
  only for backend-authorized safe read-only or idempotent Tools.

### Approval

- Backend risk classification is authoritative for approval requirements.
- Support `Allow once`, policy-permitted `Allow for conversation`, and `Deny`.
- Approval waits at most five minutes, survives browser refresh, and defaults
  to denial after backend restart.
- Use revision/CAS semantics: the first valid decision wins across tabs and
  duplicate requests return the current decision.

### Presentation and UX

- Replace the typed-canary aggregate panel with a Harness-like flat transcript:
  `Context injection -> Think -> Skill/Search/Terminal/... -> Think -> final
  answer`. Each event is an independent row on one vertical rail and keeps its
  authoritative event order; provider rounds are secondary metadata, not an
  outer accordion.
- Context and Think rows expose a concise collapsed label and an explicit
  expandable detail region. Context detail contains only the exact sanitized
  text actually supplied at the recorded injection boundary. Think detail
  contains only reasoning text actually returned by the Provider; unsupported
  Providers produce no fabricated reasoning.
- Persist and stream a block protocol with `context.injected`,
  `assistant.chunk`, and `assistant.block.completed`. Assistant chunks are a
  tagged union (`block-start`, `reasoning-delta`, `text-delta`, `block-end`,
  `usage`, `finish`) and carry Turn, Provider round/step, block index/type, and
  durable sequence identity.
- OpenAI-compatible Providers display only the reasoning content or summary
  actually returned by their API. DeepSeek `reasoning_content` is retained in
  full subject to the transcript bounds and redaction rules, including native
  Tool rounds and same-model continuation.
- Expand the active call, approval, failed, and unknown cards by default;
  collapse successful historical cards. Preserve explicit user expansion.
- Auto-follow only while the user remains near the bottom. Otherwise show a
  “return to latest execution” affordance.
- Support expand, copy, cancel, and backend-authorized retry. Copied/downloaded
  content and exports contain only sanitized presentation data.
- Desktop gets full cards; mobile gets compact summaries and drawer expansion.
  Keyboard navigation, ARIA state, reduced motion, and safe ANSI rendering are
  required.

### Typed presenters

- Web Search: use one card with an `OpenAI Built-in` or `Tavily` badge, bounded
  query summary, result count, sources, citations, status, and duration.
- Knowledge/Memory: show scope, hit count, locators, and bounded sanitized
  summaries. Memory content is collapsed and must not expose embeddings,
  internal scores, or other-conversation identifiers.
- File: variants for read/write/edit/search/publish. Use workspace-relative
  paths; read previews and unified diffs are bounded to 64 KiB; publish exposes
  safe artifact metadata and SHA-256.
- Terminal: sanitized command/cwd header, ordered stdout/stderr transcript, and
  exit/duration/timeout/truncation/background footer.
- Job: merge start/output/status/kill into a job lifecycle card.
- Skill: show safe name/package/stage/status. Skill-catalog and explicit
  user-authorized Skill injections may also appear as separate sanitized
  Context injection rows, but materialized host paths, credentials, evaluator
  prompts, and unrelated private source remain forbidden.
- Goal: show a sanitized objective, state transition, explicit budget usage,
  and safe verification summary; never expose evaluator prompts.
- Browser: show action, sanitized URL, target summary, status, and authorized
  screenshot/artifact references; never expose secrets, cookies, storage,
  profile paths, raw DOM, injected code, or sensitive form values.
- MCP: each supported Tool uses an allowlisted presenter. Unknown Tools render
  server/tool/status/duration plus a safe “details hidden” message. Raw JSON is
  forbidden.
- Binary/image/audio/resource results use authenticated artifacts; event rows
  contain only safe references.

### Bounds and security

- Per call, retain at most 64 KiB of combined stdout/stderr using head 32 KiB
  plus tail 32 KiB. Per turn, retain at most 1 MiB of Tool output and 2 MiB of
  total presentation data. Artifact bodies do not count toward the
  presentation bound and follow conversation authorization/lifecycle.
- Coalesce live chunks every 50–100 ms with a maximum chunk around 16 KiB and
  bounded queues. Slow clients may lose intermediate refreshes but must receive
  the final snapshot.
- Reasoning persistence is bounded to 1 MiB per Turn and 64 KiB per durable
  chunk. Preserve the Provider text after secret/control/path sanitization;
  never invent, infer, summarize, or reconstruct hidden reasoning.
- Sanitize before streaming, persistence, logging, DOM rendering, copying, and
  export. Combine configured-secret exact matching, high-confidence token
  patterns, path aliasing, dangerous-control filtering, and per-Tool allowlists.
- A presentation redaction failure must not change Tool execution. Hide the
  affected content, retain the safe status, and display a structured reason.
- Logs and metrics contain correlation IDs, Tool type, status, duration, byte
  counts, truncation, and redaction counters only—never commands, arguments,
  output, or user content.
- Quarantine the unused legacy private Playwright snapshot containing unsafe
  Tools; after backup and audit, remove it without changing the selected
  manifest Playwright 17-Tool allowlist.

### Rollout

- Deliver in four reviewable stages: Event Core, Timeline UI, Approval/Control,
  then full Tool Presenters.
- Keep the server feature flag off by default during each incomplete stage and
  enable only for exact canary users after the stage gate passes.
- Migrations are additive. Roll back by disabling the new UI while retaining
  event writes and the legacy transport projection; never delete event data or
  rewrite applied migration files.
- Automatically create focused conventional commits for completed coherent
  stages. Do not push.

## Acceptance Criteria

- [ ] Every currently callable Tool has a typed presenter or fail-closed safe
      fallback, with no raw JSON path.
- [ ] A new turn durably reproduces `turn.started -> tool.called -> streaming ->
      tool.result -> assistant.message -> turn.ended` in authoritative order.
- [ ] A Tool-using reasoning-capable turn durably reproduces multiple distinct
      `Think` blocks interleaved with independent Tool rows, and renders the same
      flat order live, after reload, and after reconnect.
- [ ] System-prompt and Skill-catalog context injections are independently
      expandable, bounded, sanitized, and absent when no such context reached
      the Provider.
- [ ] Live display, reload, and reconnect converge to the same order, status,
      bounded output, and presentation.
- [ ] Concurrent duplicate events do not duplicate cards or overwrite terminal
      states.
- [ ] Approval allow/deny/expiry/refresh/restart and multi-tab CAS behavior are
      covered by focused tests.
- [ ] Cancel propagates to the active turn; safe retry creates a linked new call
      without mutating history.
- [ ] Terminal transcript, File preview/diff, Browser artifacts, Search
      citations, Job lifecycle, Skill, Goal, and MCP fallback render correctly.
- [ ] Secret, host-path, malicious ANSI/control, oversized payload, unknown MCP,
      and artifact authorization probes leak no raw sensitive content through
      DB, SSE, logs, export, or DOM.
- [ ] A 500-event timeline remains usable; visible live update latency targets
      p95 <= 300 ms and reload does not regress by more than 20% from the
      existing path under the same fixture.
- [ ] Old messages retain legacy display without fabricated execution history.
- [ ] Focused backend, frontend, migration, and critical live/reload/control E2E
      checks pass. Full standalone verification runs only for release or when a
      discovered cross-domain risk justifies it.

## Definition of Done

- Code, migrations, focused tests, localization, security review, operational
  notes, rollout/rollback instructions, and relevant Trellis specs are updated.
- Each stage has a reversible, focused commit; the final workspace is clean and
  no commit is pushed automatically.
- The feature remains behind a server flag until exact-user canary acceptance
  proves live/reload parity and the security gates.

## Technical Approach

Extend the existing `ChatAgentEvent` authority rather than route new server
Tools into the legacy `ToolCallBlock`. Add backend-owned assistant/context block
events beside the existing typed Tool presentations. Persist immutable,
sanitized facts, then project one flat frontend transcript by durable sequence.
The legacy `ProcessTracePanel` remains only for messages without authoritative
Transcript v2 events.

## Decision (ADR-lite)

**Context:** neo-chat already persists Agent events, but most server Tools lose
their call/result detail before the frontend; only Terminal currently has a
specialized presentation. DeepSeek Harness demonstrates that durable call/result
events plus Tool-owned presenters can serve both live UI and replay.

**Decision:** adapt the Harness block protocol and flat information architecture
to neo-chat's existing runtime. Keep the backend as authority, persist only
bounded sanitized context/reasoning/Tool presentations, and render them from
the same authoritative event stream used by live SSE and reload.

**Consequences:** this is a cross-layer change touching event contracts,
storage, execution loops, SSE normalization, state projection, UI, security,
tests, and rollout. It intentionally does not provide raw debugging payloads or
replace the runtime.

## Out of Scope

- Replacing the existing Agent runtime or importing the DeepSeek Harness
  framework.
- Inferring hidden chain-of-thought or displaying reasoning not returned by the
  selected Provider API.
- Displaying credentials, host paths, evaluator prompts, provider-internal
  encrypted/redacted thinking payloads, or unsanitized prompt/tool data.
- Persisting unlimited raw Tool output.
- Building a complete durable background-job orchestration system in this
  release.
- Rebuilding Playwright as a separate backend Tool family.

## Technical Notes

- DeepSeek Harness reference commit:
  `141eb6fef83422698aef7a981029e843e8161534`.
- Existing event authority: `mm-chat/backend/internal/chat/chat_agent_events.go`.
- Existing process projection: `mm-chat/backend/internal/chat/process_trace.go`
  and `process_trace_runtime.go`.
- Existing frontend timeline: `mm-chat/frontend/src/components/content/ProcessTracePanel.tsx`.
- Existing server Tool updates are normalized in
  `frontend/src/services/api/client/server/chatApi.ts`, but the server chat
  store currently does not consume `onToolCall`; the new path must not create a
  second client-side authority.
- Research references:
  [`research/harness-architecture.md`](research/harness-architecture.md) and
  [`research/current-tool-inventory.md`](research/current-tool-inventory.md).

## Implementation Progress

### Slice 1 — versioned presentation and typed timeline

- Implemented the backend-owned schema-v1 presentation union and strict
  Tool/mode allowlists.
- Added safe presenters for Search, File, Terminal, Job, Skill, Goal, Browser,
  and generic MCP. Exact retrieval and workspace search queries remain omitted
  under the existing diagnostics security contract.
- Added bounded/redacted Terminal final transcripts with 32 KiB head + 32 KiB
  tail retention, File preview/diff/search summaries, and summary-only dynamic
  MCP/Browser fallback.
- Added frontend runtime normalization, provider-round grouping, typed cards,
  transcript/diff rendering, copy controls, and unknown-version fail-closed
  behavior.
- Focused Backend chat tests, Go vet, frontend ESLint/typecheck, and focused
  ProcessTrace Vitest pass.

### Remaining slices

- Implement authoritative assistant/context blocks and the flat Transcript v2
  projector, then run one pinned focused canary acceptance session. Widening
  scope or deleting the legacy control/rollback fallback remains a separate
  reversible slice.

### Slice 8 — performance and security acceptance

- Replaced repeated step-list scans during durable projection with one
  step-ID index while preserving first-seen order and last-event state.
- Added a 500-event warm p95 gate covering visible update latency at 300 ms
  and durable reload within 20% of the legacy path under the same render
  workload.
- Added a hostile end-to-end presentation fixture covering secret redaction,
  host-path aliases, ANSI/control filtering, transcript and payload bounds,
  unknown MCP fallback, raw payload exclusion, and DOM non-disclosure.
- Kept the legacy ProcessStep transport intact. Removal remains blocked until
  one focused exact-user canary session passes these gates.

### Slice 9 — focused-canary evidence gate

- Defines acceptance as one positive-duration focused session on one unchanged
  Git commit and immutable Backend/Frontend digests with exactly one canary
  plus a disjoint control; no arbitrary soak time is required.
- Added a strict content-free evidence verifier for minimum synthetic Tool and
  event coverage, live/reload/reconnect/approval/cancel/retry behavior,
  performance limits, security probes, and rollback preservation.
- Added a synthetic example and focused mutation tests for invalid windows,
  control leakage, latency/ratio regression, missing security probes, and raw
  content fields.
- The verifier performs no deployment, Provider call, flag mutation, Push, or
  transport deletion. Legacy removal remains blocked until real external
  focused-session evidence returns `eligible`.

### Slice 10 — authoritative live Agent events

- Added exact-canary `agent.event` SSE frames containing the normalized event
  returned by durable persistence. Durable `eventId`/sequence own frontend
  ordering and deduplication while the outer SSE sequence remains the reconnect
  cursor.
- Final `assistant.message` and `turn.ended` facts precede the terminal Message,
  which carries the same recorded event set for immediate convergence.
- The frontend validates live Agent events at the stream boundary and projects
  them with the same durable-event projector used after reload. The legacy
  ProcessStep/Tool compatibility frames remain for the next reversible removal
  slice.

### Slice 11 — remove duplicate transport from the typed canary path

- Admitted canaries now receive persisted facts only through `agent.event`;
  duplicate `process.step.updated` and `tool.call.updated` frames are not sent.
- Coalesced Terminal snapshots use transient `agent.progress`. The frontend
  overlays them without adding them to the immutable event accumulator and
  removes them on the next matching durable event or terminal Turn state.
- Backend retains bounded ProcessTrace metadata solely for flag-off rollback
  but omits it from typed canary DTOs. Non-canary controls retain legacy frames
  and metadata, so rollback does not fabricate events or erase historical UI.
- Focused eligible evidence still gates widening beyond exact users and final
  deletion of the control/rollback fallback. No flag widening, deployment,
  Push, or evidence fabrication is part of this code slice.

### Slice 12 — Harness Transcript v2

- Added forward migration `101` and strict authoritative
  `context.injected`, `assistant.chunk`, and `assistant.block.completed`
  payloads. Context and Provider reasoning are sanitized before durable append,
  SSE, reload, or DOM projection; reasoning is bounded to 64 KiB per event and
  1 MiB per Turn.
- Removed the official DeepSeek Tool-round thinking override. Native Tool
  requests and continuation now preserve the configured reasoning mode and the
  existing `AssistantReasoning -> reasoning_content` exchange.
- Added distinct reasoning blocks around Tool boundaries. Each block carries a
  positive index and Provider round; unsupported Providers produce no fake
  Think row.
- Replaced the typed-canary aggregate panel with a flat `AgentTranscript`:
  independent expandable Context/Think rows and existing typed Tool presenters
  share one vertical sequence. Messages without v2 events retain the legacy
  `ProcessTracePanel`.
- Focused Go event/Provider/handler tests, strict frontend projection/render
  tests, typecheck/lint, and the PostgreSQL 17 migration replay pass. Production
  build, commit, and exact-user Canary deployment complete the release slice.

### Slice 2 — live Terminal transcript

- Added ordered stdout/stderr pipe callbacks under the existing executor output
  budget.
- Added a 64-byte sanitizer holdback, 75 ms/16 KiB coalescing, path redaction,
  and nonblocking transient Provider events.
- Running ProcessSteps update the existing Terminal card in place. Transient
  progress is SSE-only; completion persists the final bounded/redacted snapshot
  so reload does not create or replay one database event per chunk.

### Slice 3 — durable Tool approval

- Added migration `100` with least-privilege approval/CAS gateways and exact
  Conversation + Tool + risk grants. Raw Tool arguments/results never enter
  approval storage.
- Destructive Terminal calls now persist an `awaiting_approval` presentation,
  wait at most five minutes, and resume the same call only after an exact allow.
  Hard-blocked commands remain non-bypassable.
- Added first-decision-wins `Allow once`, policy-permitted `Allow for
  conversation`, `Deny`, expiry, and startup `restart_denied` behavior.
- Added runtime-validated frontend approval controls with safe malformed-data
  fallback, plus focused Go/Vitest and PostgreSQL 17 approval coverage.

### Slice 4 — cursor reconnect

- Added a process-local bounded Run ring keyed by the existing monotonic SSE
  sequence. Sequenced frames now also carry the matching SSE `id` cursor.
- Added exact-user/current-Conversation `GET /v1/chat/runs/{runId}/events`
  replay with atomic suffix subscription, bounded slow-client queues, heartbeat,
  terminal grace retention, and explicit `cursor_evicted` gaps.
- Frontend automatically resumes from the last accepted sequence, ignores
  duplicates, shows a localized partial-view warning after eviction, and
  converges on the terminal Message snapshot without fabricating missing data.
- Focused Backend ring/endpoint and Frontend reconnect/gap tests pass; transient
  chunks remain SSE-only and never become one database event per chunk.

### Slice 5 — process-local Job lifecycle

- Background Terminal now emits a Job start presentation with sanitized
  command/cwd and minimal process-local lifecycle metadata. Later output/kill
  calls remain separate immutable Agent Tool events.
- The frontend joins exact matching `jobId` cards into one lifecycle while
  preserving the first call position and leaving normalized source steps
  untouched.
- Startup Turn recovery projects unresolved `running`/`stopping` Jobs as
  `interrupted`; it never fabricates process survival, output, or completion.
- Focused Backend race tests and Frontend normalization/render tests cover
  metadata bounds/redaction, lifecycle merge, immutability, and restart state.

### Slice 6 — exact-user canary gate

- Added API-only `AGENT_TIMELINE_ENABLED` and
  `AGENT_TIMELINE_CANARY_USER_IDS`; both default off/empty and both must admit
  the exact authenticated UUID.
- Durable Agent event writes continue for rollback safety. Non-canary responses
  omit Agent events and typed presentations, preserve legacy ProcessSteps, and
  never start an invisible durable approval wait.
- Added config/admission/immutability tests, Compose/preflight defaults, and a
  Backend-only flag rollback contract. No migration or event deletion is part
  of rollout or rollback.

### Slice 7 — Backend-authorized safe Retry

- Added a fail-closed Retry capability carried only by a versioned sanitized
  presentation. The first allowlist admits failed `local_direct file_read`
  calls with `file_not_found` or `execution_failed`; no browser arguments or
  risk inference cross the API.
- The owner-scoped source event is immutable. A retry creates one deterministic
  sibling Agent Message per source event, a new call ID, and durable `retryOf`
  linkage. Duplicate requests return the existing attempt instead of executing
  twice.
- Write/execute/MCP, `outcome_unknown`, cross-user, non-canary, malformed, and
  legacy events expose no control and fail closed. Focused Backend race tests
  and Frontend normalization/render/store tests cover the admitted and denied
  paths.
