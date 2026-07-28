# Backend Development Guidelines

> Executable contracts for the independent `mm-chat` Go/PostgreSQL backend.

## Guidelines Index

| Guide                                               | Scope                                                                                                                 |
| --------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| [RAG retrieval storage](./rag-retrieval-storage.md) | PostgreSQL retrieval, Citation authority/display, diagnostics, and rollback contracts                              |
| [Chat source fusion](./chat-source-fusion.md)       | Conversation-aware external Search query rewriting, Knowledge/Web authority, diagnostics, and fallback contracts      |
| [Planned chat Tool Loop](./chat-tool-loop.md)       | G19 provider-normalized Tool rounds, three-state Search authority, process persistence, approvals, and citation truth |
| [Direct chat attachments](./chat-attachments.md)    | Attachment-only messages, native images, bounded document extraction, provider context, and explicit failures       |
| [Hosted media provider smoke](./provider-live-smoke.md) | Exact live-provider authorization, one-off credentials, explicit TTS voices, artifacts, and sanitized evidence    |
| [Hosted TTS production](./hosted-tts-production.md) | Dedicated SiliconFlow Voice authority, exact activation, server-mode playback, per-user cache, and cleanup |
| [Memory v2 benchmark](./memory-v2-benchmark.md) | Synthetic-only 500-case Golden lifecycle, strict observation/evaluation contracts, immutable reports, and non-promotional boundaries |
| [Memory v2 storage](./memory-v2-storage.md) | Project/scope/settings foundation, Global v1 repository compatibility, ownership constraints, and guarded rollback |
| [Memory v2 worker](./memory-v2-worker.md) | ID-only completed-turn capture, leased PostgreSQL jobs, private Go worker, Redis wake, least privilege, replay, and rollback |
| [Memory v2 provenance/delete](./memory-v2-provenance-deletion.md) | Canonical revisions, ID/hash evidence, visibility epochs, tombstones, manifests, and provider-free purge |
| [Memory v2 candidate/Review](./memory-v2-candidate-review.md) | Strict candidate batches, proposal-only conflict/scope/temporal routing, Review shadow, and provider-free expiry |
| [Memory v2 actions/Activity/Usage](./memory-v2-actions-activity-usage.md) | Current-user typed actions, strict planner authority, immutable Usage, link-only Activity polling, and revision-safe undo |
| [Memory v2 lexical shadow](./memory-v2-lexical-shadow.md) | Transactional L1 exact/CJK BM25 projection, current-authority shadow comparison, ID-only diagnostics, and v1 fail-open |
| [Memory v2 hybrid shadow](./memory-v2-hybrid-shadow.md) | Fixed BGE-M3 vector jobs, Exact/BM25/vector RRF/rerank shadow, budget fallback, old-response fences, and zero prompt injection |
| [Memory v2 governance](./memory-v2-governance.md) | Project/Conversation policy, scoped governance CRUD, Review decisions, current-only detail/Activity hydration, and governed v1 compatibility |

## Pre-Development Checklist

For RAG retrieval, PostgreSQL migration, or indexing changes:

1. Read [`rag-retrieval-storage.md`](./rag-retrieval-storage.md).
2. Confirm the running PostgreSQL major and extension availability before
   adding ordinary migrations.
3. Identify the current-authority hydration boundary and rollback artifact.
4. Define a synthetic Golden/negative proof before changing the reader.

For external Search or Knowledge/Web fusion changes:

1. Read [`chat-source-fusion.md`](./chat-source-fusion.md).
2. Prove the active-branch context and runtime model identity reaching the
   rewrite boundary.
3. Keep exact query text and source bodies out of durable diagnostics.
4. Define rewrite-success, unchanged, and fail-open tests before changing the
   Search request.

For G19 Tool Loop or durable process-trace changes:

1. Read [`chat-tool-loop.md`](./chat-tool-loop.md).
2. Identify the owning G19 group and keep unpromoted behavior behind the
   current source-fusion rollback path.
3. Prove provider-native continuation, cancellation, redaction, and current-
   turn Citation reconciliation before promotion.

For chat upload, attachment parsing, or provider attachment changes:

1. Read [`chat-attachments.md`](./chat-attachments.md).
2. Trace file bytes from upload storage through the current provider request.
3. Preserve native images, explicit document failures, context limits, and the
   untrusted-data boundary.
4. Prove attachment-only acceptance and pre-acceptance draft restoration.

For hosted Voice/Image executor or live-smoke changes:

1. Read [`provider-live-smoke.md`](./provider-live-smoke.md).
2. Identify the exact kind/provider/model and any provider-qualified voice.
3. Preserve default denial, one-off credential isolation, and sanitized
   evidence before making a provider call.
4. Define offline zero-network tests and independent output validation before
   running the authorized live smoke.

For production hosted TTS administration, runtime, playback, or cache changes:

1. Read [`hosted-tts-production.md`](./hosted-tts-production.md).
2. Trace encrypted ingress -> Voice vault -> exact attestation -> executor ->
   File artifact -> authenticated frontend playback.
3. Preserve TTS/STT capability separation and block every server-mode legacy
   Voice route/provider fallback.
4. Prove per-user source ownership, cache reuse, TTL/LRU reclamation, and
   replay-safe object deletion before live activation.

For Memory benchmark, Memory reader, recall-ranking, prompt-injection, or
reader-promotion changes:

1. Read [`memory-v2-benchmark.md`](./memory-v2-benchmark.md).
2. Keep corpus inputs synthetic-only and keep committed drafts explicitly
   ineligible for promotion.
3. Preserve frozen hashes, exact case order, one-shot Holdout, exclusive report
   output, and the separation between evaluation evidence and reader authority.
4. Prove current-fact, false-injection, cross-user, secret, deletion, Provider-
   egress, latency, token, and cost gates before changing a reader.

For Memory Project, scope, settings, repository, or migration changes:

1. Read [`memory-v2-storage.md`](./memory-v2-storage.md).
2. Keep Project/Conversation ownership in composite database foreign keys;
   source provenance is not scope authority.
3. Preserve the Global-only v1 API until its owning reader/API PR changes the
   contract explicitly.
4. Prove additive defaults, backfill, same-scope uniqueness, guarded rollback,
   and disposable PostgreSQL down/re-up before release.

For Memory completed-turn capture, outbox/jobs, worker, Redis wake, Provider
hydration, or worker deployment changes:

1. Read [`memory-v2-worker.md`](./memory-v2-worker.md).
2. Preserve PostgreSQL as authority and keep Redis ID-only/best-effort.
3. Trace finalize transaction -> claim -> hydrate -> apply -> complete/retry,
   including every lease/user/source/generation/profile fence.
4. Prove current/N-1 schema handling, crash reclaim, stale lease denial,
   direct-table denial, Redis-down polling, and guarded down/re-up.

For Memory canonical writes, evidence/revisions, epochs, tombstones, deletion
manifests, or purge changes:

1. Read
   [`memory-v2-provenance-deletion.md`](./memory-v2-provenance-deletion.md).
2. Preserve the v1 Global-only HTTP/reader contract and the one canonical
   plaintext row.
3. Trace delete transaction -> immediate invisibility -> tombstone/manifest ->
   provider-free purge, including the old-response no-resurrection path.
4. Prove backfill, append-only revisions, cross-user denial, manual precedence,
   stale epoch/lease denial, idempotent plaintext wipe, and guarded down/re-up.

For Memory extraction candidates, Review suggestions, conflict/scope/temporal
routing, or Review expiry changes:

1. Read [`memory-v2-candidate-review.md`](./memory-v2-candidate-review.md).
2. Preserve candidate-wide atomic proposal and keep every PR5 outcome out of
   canonical Memory and every active reader.
3. Trace redacted context -> strict extraction -> bounded decision -> SQL
   proposal -> committed replay/30-day provider-free expiry.
4. Prove secret zero-plaintext, evidence/scope/target ownership, exact/manual/
   temporal routing, old-apply denial, and guarded down/re-up.

For direct-user Memory actions, answer Usage links, Activity polling, or undo
changes:

1. Read
   [`memory-v2-actions-activity-usage.md`](./memory-v2-actions-activity-usage.md).
2. Preserve current-completed-user-only intent and strict Provider proposal;
   rebind user, scope, target, revision, epoch, and generation authority.
3. Keep Usage immutable and Activity link-only; hydrate only current visible
   content and never reconstruct deleted content from revision history.
4. Prove secret zero-egress/plaintext, exact NOOP silence, safe/stale undo,
   direct purge, least privilege, and guarded down/re-up.

For Memory exact/CJK BM25 projections or lexical shadow comparison changes:

1. Read [`memory-v2-lexical-shadow.md`](./memory-v2-lexical-shadow.md).
2. Preserve v1 as the only prompt/Usage reader and keep the shadow flag
   default-off; do not pull PR8 vector/RRF/rerank into this slice.
3. Bind every candidate before ranking to current user/scope/Sensitive/epoch/
   generation/revision/hash authority, and keep observations query/content/
   raw-score free.
4. Prove all canonical write/delete/purge paths maintain derived projection in
   the same transaction, shadow failure leaves v1 unchanged, runtime roles lack
   table CRUD, and guarded down/re-up remains clean.

For Memory BGE-M3 projection/jobs, hybrid RRF/rerank, token budget, or hybrid
shadow switch changes:

1. Read [`memory-v2-hybrid-shadow.md`](./memory-v2-hybrid-shadow.md).
2. Keep the fixed BGE tuple and pin revision/hash/epoch/scope/projection/
   Provider authority across claim, hydrate, Provider work, and complete.
3. Keep all three candidate lanes independently authorized, keep query/content/
   raw scores out of durable diagnostics, and reauthorize after rerank.
4. Prove the shared default-off flag makes zero Memory Provider calls, v1
   prompt/Usage remains byte-authoritative, and guarded down/re-up is clean.

For Memory Project/Conversation governance, Review decisions, detail/history,
assistant Activity UI, or post-`060` v1 CRUD changes:

1. Read [`memory-v2-governance.md`](./memory-v2-governance.md).
2. Preserve v1 Global Top 5 as the only prompt/Usage reader and keep scoped
   Memory governance-only.
3. Bind all plaintext hydration to current enabled/lifecycle/epoch/scope-
   generation authority; never reconstruct deleted content from revisions.
4. Prove secret/Sensitive classification in both Go and SQL, legacy write
   capability revocation, Review replay fences, provider-free purge, and clean
   guarded down/re-up.

## Quality Check

- Run the disposable database drill for the changed storage group.
- Run `go vet ./...` and `go test ./...` from `mm-chat/backend`.
- Prove SECURITY DEFINER paths, role grants, deletion invisibility, migration
  manifest stability, and cleanup.
- Update the G18 plan/process records when the retrieval program is affected.
- Run real external-Search hit/follow-up proof and update the G11 source-fusion
  process when query planning is affected.
- For attachment changes, run parser units, image-path regression,
  attachment-only API/UI tests, and one live upload-to-answer replay.
- For hosted provider smoke changes, run focused executor/gate tests, all Go
  tests and vet, a diff secret scan, then the exact authorized live command.
- For production TTS changes, also run migration `051` replay/cache integration,
  frontend server-route regressions, Compose rendering, and the standalone full
  gate. Live activation requires a fresh separately authorized Key.
- For Memory benchmark changes, run focused race tests for `internal/memoryeval`
  and `cmd/memory-eval`, all backend tests, and `go vet ./...`. Reader/runtime
  changes additionally require their owning migration, shadow, and rollback
  gates; PR1 alone does not require Docker or a Live Provider.
- For Memory storage changes, run focused race tests for `internal/usermemory`,
  `internal/migration`, and `cmd/migrate`; prove the exact migration against a
  disposable PostgreSQL database before all backend tests and vet.
- For Memory worker changes, run the focused race, PostgreSQL 17 replay,
  preflight/Compose, and backend-image gates defined in `memory-v2-worker.md`.
- For Memory provenance/delete changes, also run the PostgreSQL 17 deletion
  drill and every gate in `memory-v2-provenance-deletion.md`.
- For Memory candidate/Review changes, also run the PostgreSQL 17 proposal and
  expiry drill plus every gate in `memory-v2-candidate-review.md`.
- For Memory direct action/Activity/Usage changes, also run the PostgreSQL 17
  action/undo/purge/replay drill plus every gate in
  `memory-v2-actions-activity-usage.md`.
- For Memory lexical projection/shadow changes, also run the PostgreSQL 17 CJK
  BM25 projection/authority/replay drill plus every gate in
  `memory-v2-lexical-shadow.md`.
- For Memory hybrid vector shadow changes, also run the PostgreSQL 17 fake
  vector/lease/authority/RRF/replay drill plus every gate in
  `memory-v2-hybrid-shadow.md`.
- For Memory governance changes, also run the PostgreSQL 17 Project/policy/
  Review/Activity/legacy-wrapper/purge/replay drill plus every gate in
  `memory-v2-governance.md`.

**Language**: All documentation should be written in English.
