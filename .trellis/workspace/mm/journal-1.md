# Journal - mm (Part 1)

> AI development session journal
> Started: 2026-07-07

---



## Session 1: Complete G18 PG17 production cutover

**Date**: 2026-07-22
**Task**: Complete G18 PG17 production cutover
**Branch**: `main`

### Summary

Cut production Compose and PostgreSQL authority to the fresh PG17 data path, hardened preflight and rollback contracts, verified runtime roles/readiness/restart/backups, updated G18 records, and retained the PG16 rollback anchors.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `c1678de` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 2: Reconcile mm-chat migration ledger

**Date**: 2026-07-23
**Task**: Reconcile mm-chat migration ledger
**Branch**: `main`

### Summary

Closed stale G7/G18/G19 and Phase 9/15 plan states, preserved only Voice, owner-authorized root deletion, and optional Phase 16 as unresolved work, verified documentation consistency, and archived the completed server-refactor task.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `0b43497` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 3: Complete structure-aware BGE RAG candidate

**Date**: 2026-07-25
**Task**: Complete structure-aware BGE RAG candidate
**Branch**: `main`

### Summary

Implemented structure-first token-aware and semantic chunking, SiliconFlow BGE retrieval, generation promotion gates, Jina runtime retirement, and frozen evaluation evidence; Candidate 8 remains verified/ready without Activation, with Python coverage at 90.14%.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `185e040` | (see git log) |
| `5b962c2` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 4: Add knowledge document download

**Date**: 2026-07-26
**Task**: Add knowledge document download
**Branch**: `main`

### Summary

Added an accessible per-document download action for active server Knowledge documents using the existing authorized binary API, sanitized source filenames, bounded Blob URL cleanup, localized errors, and regression coverage; full frontend checks and backend Knowledge tests passed.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `f02eccb` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 5: Improve Knowledge citation display

**Date**: 2026-07-26
**Task**: Improve Knowledge citation display
**Branch**: `main`

### Summary

Projected authorized source names and typed display locators into Knowledge citations; localized multi-format citation cards without exposing UUIDs or raw locator JSON; added regression tests and the cross-layer display contract.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `72feeec` | (see git log) |
| `9bf434f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 6: Remove former root application

**Date**: 2026-07-26
**Task**: Remove former root application
**Branch**: `main`

### Summary

Backed up and restore-drilled the former root, removed its duplicate application, retained mm-chat as the sole product root, and retargeted repository automation and documentation.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `35100ac` | (see git log) |
| `3f133db` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 7: Delete retired PostgreSQL 16 data

**Date**: 2026-07-26
**Task**: Delete retired PostgreSQL 16 data
**Branch**: `main`

### Summary

Verified and externally copied the PG16 logical rollback artifacts, deleted only the retired physical PG16 data directory, and confirmed PostgreSQL 17 plus all application health checks remained green.

### Main Changes

(Add details)

### Git Commits

(No commits - planning session)

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 8: Prune obsolete runtime backups

**Date**: 2026-07-26
**Task**: Prune obsolete runtime backups
**Branch**: `main`

### Summary

Archived and removed superseded mm-chat backup milestones behind a fixed allowlist, retained the PG17 production-cutover package and latest PostgreSQL dump, verified checksums and temporary PostgreSQL/MinIO restores, and passed full standalone plus live health checks.

### Main Changes

(Add details)

### Git Commits

(No commits - planning session)

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 9: Patch frontend dependency vulnerabilities

**Date**: 2026-07-27
**Task**: Patch frontend dependency vulnerabilities
**Branch**: `main`

### Summary

Patched Next/OpenNext and vulnerable transitive frontend dependencies, documented bounded dev-tool audit exceptions, hardened standalone source-copy exclusions, and passed full-stack verification.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `23f7e85` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 10: Reconcile completed chat tasks

**Date**: 2026-07-27
**Task**: Reconcile completed chat tasks
**Branch**: `main`

### Summary

Reconciled four stale chat tasks against their committed implementations, refreshed targeted Go/frontend gates, completed bounded live Knowledge-routing and remote-file smokes with cleanup, wrote result records, and archived all four tasks.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `d9c0d88` | (see git log) |
| `2433754` | (see git log) |
| `b63bd7a` | (see git log) |
| `630fb80` | (see git log) |
| `337dd19` | (see git log) |
| `1c41b47` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 11: Bootstrap frontend guidelines and isolate journal commits

**Date**: 2026-07-27
**Task**: Bootstrap frontend guidelines and isolate journal commits
**Branch**: `main`

### Summary

Completed the frontend Trellis specs and restricted session auto-commits to the journal/index pathspec with isolated Git regression coverage.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `e5c8cce` | (see git log) |
| `1cfa491` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 12: Review and track shared Trellis scaffold

**Date**: 2026-07-27
**Task**: Review and track shared Trellis scaffold
**Branch**: `main`

### Summary

Audited and tracked the shared Trellis/Codex scaffold while isolating developer, runtime, update, cache, worktree, and editor state.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `f8bed19` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 13: Verify SiliconFlow hosted TTS smoke

**Date**: 2026-07-27
**Task**: Verify SiliconFlow hosted TTS smoke
**Branch**: `main`

### Summary

Selected SiliconFlow CosyVoice2 with the claire voice, made the live harness require an explicit provider-qualified voice, passed offline/full quality gates and an authorized real MP3 synthesis, recorded the owner's accepted playback verdict, and kept production Voice wiring and ASR out of scope.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `c0e2c17` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 14: SiliconFlow TTS production rollout

**Date**: 2026-07-27
**Task**: SiliconFlow TTS production rollout
**Branch**: `main`

### Summary

Deployed the dedicated encrypted SiliconFlow CosyVoice2 TTS path, repaired runtime cache grants with migration 052, proved first synthesis and exact cache reuse through actor-owned File playback, reclaimed disposable artifacts, and passed the full standalone gate.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ccad2a3` | (see git log) |
| `39672e1` | (see git log) |
| `baf0c03` | (see git log) |
| `8bc0a56` | (see git log) |
| `5debc62` | (see git log) |
| `8922d2c` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 15: Stabilize read-aloud playback lifecycle

**Date**: 2026-07-27
**Task**: Stabilize read-aloud playback lifecycle
**Branch**: `main`

### Summary

Centralized tab-scoped TTS playback ownership, propagated cancellation, attached blob audio before playback, added lifecycle regressions, verified cached artifacts, passed the full standalone gate, and redeployed the frontend stack.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `cb32866` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 16: Server voice read-aloud visible text

**Date**: 2026-07-27
**Task**: Server voice read-aloud visible text
**Branch**: `main`

### Summary

Projected hosted TTS from authoritative visible text, made Browser speech read rendered innerText, hid legacy Local voice controls in Server mode, passed the full standalone gate, and rebuilt the healthy frontend deployment.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `8779d4a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 17: Implement Memory v2 durable capture worker

**Date**: 2026-07-28
**Task**: Research memory system architecture (PR3)
**Branch**: `main`

### Summary

Implemented PR3's ID-only PostgreSQL Memory outbox/jobs, atomic assistant
finalize capture, private lease-fenced Go Memory Worker, optional Redis wake,
Server provider/vault reuse, least-privilege role boundary, Compose runtime,
and guarded rollback while retaining the Global v1 reader and CRUD contract.

### Main Changes

- Added migration `054`, current/N-1 event handling, bounded retry/reclaim,
  dead-letter, source/profile/generation fences, and restricted SQL functions.
- Removed request-local extraction goroutines and added the standalone
  `mm-chat-memory-worker` command to the shared backend image.
- Added private Compose/preflight/runtime documentation and executable Trellis
  contracts for the Memory worker boundary.

### Testing

- [OK] Focused Go race tests, full `go test ./...`, and `go vet ./...`.
- [OK] Disposable PostgreSQL 17.10 `001 -> 054`, guarded down, and re-up drill.
- [OK] Preflight tests, Compose rendering, module/security/quality scans, and
  backend image build with the worker binary.
- [OK] Full standalone gate: frontend 954 tests/build, backend tests/vet, and
  RAG 1,906 passed / 7 skipped with Ruff and mypy clean.

### Status

[OK] **PR3 completed; parent Memory v2 task remains in progress**

### Next Steps

- Begin PR4 evidence/revision/epoch/tombstone/delete-manifest correctness only
  after this PR3 batch is committed.


## Session 18: Implement Memory v2 provenance and deletion safeguards

**Date**: 2026-07-28
**Task**: Research memory system architecture (PR4)
**Branch**: `main`

### Summary

Implemented PR4's canonical provenance, append-only revision history, per-user
visibility epoch, authority precedence, targeted tombstones, immediate logical
deletion, deletion manifests, and provider-free purge while retaining the v1
reader and Global-only API contract.

### Main Changes

- Added migration `055` with provenance/evidence/revision/state/tombstone and
  deletion-manifest storage plus narrow manual, worker-apply, delete, and purge
  SQL capabilities.
- Enforced surviving user evidence for automatic Memory, manual-authority
  precedence, stale epoch/source/generation fences, and no automatic
  resurrection across tombstones.
- Routed `purge` jobs before Provider hydration, made purge idempotent and
  lease-fenced, revoked runtime table-delete privilege, and documented the new
  executable contracts.

### Git Commits

| Hash | Message |
|------|---------|
| `754240c` | `feat: add Memory provenance and deletion safeguards` |
| `4808e99` | `docs(task): record Memory v2 PR4 progress` |

### Testing

- [OK] Focused Go race tests, full `go test ./...`, and `go vet ./...`.
- [OK] Disposable PostgreSQL 17.10 `054 -> 055`, backfill, authority/evidence,
  deletion/purge, guarded down, clean down, and re-up drills.
- [OK] Preflight tests, Compose rendering, backend image build, and security
  scan with no new findings.
- [OK] Full standalone gate: frontend 954 tests/build, backend tests/vet, and
  RAG 1,906 passed / 7 skipped.

### Status

[OK] **PR4 completed; parent Memory v2 task remains in progress**

### Next Steps

- Begin PR5 capture-candidate proposal, Review shadow, and
  temporal/conflict/scope routing without switching the v2 reader.


## Session 19: Implement Memory v2 candidate and Review shadow

**Date**: 2026-07-28
**Task**: Research memory system architecture (PR5)
**Branch**: `main`

### Summary

Implemented PR5 strict candidate-wide Memory proposals, Review shadow routing,
canonical auto-apply revocation, secret/Sensitive privacy guards, and
provider-free 30-day expiry while retaining the v1 reader and API contract.

### Main Changes

- Added migration `056` canonical temporal/conflict metadata, candidate batch
  authority, normalized Review target/evidence tables, and guarded rollback.
- Split extraction and decision prompts, enforced exact strict JSON fields,
  bounded context/targets, and deterministic secret/Sensitive redaction.
- Routed exact, manual-conflict, temporal, secret, and unrelated candidates
  without any canonical mutation; expiry wipes proposal plaintext without a
  Provider call.

### Git Commits

| Hash | Message |
|------|---------|
| `4a5fbf2` | `feat: add Memory candidate and Review shadow` |
| `290a5cb` | `docs(task): record Memory v2 PR5 progress` |

### Testing

- [OK] Focused race tests, full backend tests, and `go vet ./...`.
- [OK] Disposable PostgreSQL 17.10 proposal/expiry, guarded down, clean down,
  and re-up drill.
- [OK] Preflight, Compose rendering, backend image worker binary, and related
  quality/security scans.
- [OK] Full standalone gate: frontend 954 tests/build, backend tests/vet, and
  RAG 1,906 passed / 7 skipped.

### Status

[OK] **PR5 completed; parent Memory v2 task remains in progress**

### Next Steps

- Begin PR6 direct-user typed Memory actions, Activity/Usage links, and
  revision-safe undo without switching the v2 reader early.


## Session 20: Implement Memory v2 direct actions and Activity/Usage

**Date**: 2026-07-28
**Task**: Implement Memory v2 direct actions and Activity/Usage
**Branch**: `main`

### Summary

Implemented PR6 direct-user typed Memory actions, immutable answer Usage,
link-only Activity polling, and revision-safe undo while retaining the v1
reader and API contract.

### Main Changes

- Added migration `057` with direct-user action/target authority, immutable
  answer Usage links, link-only Activity, complete typed revision snapshots,
  narrow runtime capabilities, and guarded rollback.
- Added current-completed-user-only lexical gating, strict versioned planning,
  task-model preference with chat fallback, secret zero-egress, and Go/SQL
  target, revision, scope, epoch, and generation rebinding.
- Added direct remember/correct/forget, exact-NOOP silence, Activity/Usage APIs,
  atomic assistant Usage finalization, revision-safe created/corrected undo,
  provider-free purge, shared strict JSON/privacy primitives, and executable
  contracts.


### Git Commits

| Hash | Message |
|------|---------|
| `1415b74` | `feat: add Memory direct actions and Activity/Usage tracking` |
| `081b92e` | `docs(task): record Memory v2 PR6 progress` |

### Testing

- [OK] Focused race tests, full backend tests, and `go vet ./...`.
- [OK] Disposable PostgreSQL 17.10 action/undo/purge/replay, runtime-role
  denial, guarded down, clean down, and re-up drill.
- [OK] Preflight, Compose rendering, backend image build, diff/format/secret,
  module, quality, change, and security checks.
- [OK] Full standalone gate: frontend 954 tests/build, backend tests/vet, and
  RAG 1,906 passed / 7 skipped.

### Status

[OK] **PR6 completed; parent Memory v2 task remains in progress**

### Next Steps

- Begin PR7 L1 exact/CJK BM25 projection shadow without switching the v2
  reader or enabling dense retrieval early.


## Session 21: Implement Memory v2 lexical projection shadow

**Date**: 2026-07-28
**Task**: Implement Memory v2 L1 exact/CJK BM25 projection shadow
**Branch**: `main`

### Summary

Implemented PR7 transactional exact/CJK BM25 projections and default-off,
zero-prompt-injection comparison against the v1 reader while keeping v1 Top 5,
prompt, Usage, and chat success as the only production authority.

### Main Changes

- Added migration `058` with canonical-owned derived projections, exact and
  `pg_textsearch` BM25 lanes, normalized ID-only observations/results, narrow
  runtime capability, idempotent replay, and guarded rollback.
- Added optional Go lexical shadow comparison, bounded metadata sanitization,
  fail-open chat behavior, and `MEMORY_LEXICAL_SHADOW_ENABLED=false` wiring.
- Added executable Trellis/runtime/deployment/persistence contracts and static,
  Go, and disposable PostgreSQL coverage for projection correctness, authority,
  lifecycle, replay, permissions, and rollback.

### Git Commits

| Hash | Message |
|------|---------|
| `2b85c35` | `feat: add Memory lexical projection and shadow comparison` |
| `058d08d` | `docs(task): record Memory v2 PR7 progress` |

### Testing

- [OK] Focused race tests, full backend `go test ./...`, and `go vet ./...`.
- [OK] Disposable PostgreSQL 17 backfill, CJK/Latin/punctuation lanes,
  transaction maintenance, authority/lifecycle/staleness filtering,
  replay/conflict, runtime-role denial, guarded down, clean down, and re-up.
- [OK] Preflight, Compose rendering, backend image build, and related
  quality/security scans.
- [OK] Full standalone gate: frontend 954 tests/build, backend tests/vet, and
  RAG 1,906 passed / 7 skipped.

### Status

[OK] **PR7 completed; parent Memory v2 task remains in progress**

### Next Steps

- Begin PR8 BGE-M3/vector, RRF, rerank, and budget fallback as a separate 0%
  prompt-injection shadow batch without switching the v2 reader early.


## Session 22: Implement Memory v2 hybrid vector shadow

**Date**: 2026-07-28
**Task**: Implement Memory v2 BGE-M3/vector hybrid shadow
**Branch**: `main`

### Summary

Implemented PR8 default-off, zero-prompt-injection hybrid Memory shadow with
BGE-M3 vector projections, independent Exact/BM25/Vector lanes, deterministic
RRF, bounded BGE reranking, and fail-open token-budget fallback while retaining
the v1 reader as the only production authority.

### Main Changes

- Added migration `059` with fixed 1024d embedding binding, partial HNSW cosine
  index, lease-fenced embedding jobs, normalized score-free observations/results,
  narrow runtime capabilities, and guarded rollback/replay.
- Added worker-side fake-testable embedding orchestration and API-side hybrid
  comparison with `RRF(k=60)`, rerank fallback, 600-token target, 900-token hard
  cap, 2-second cutoff, and no impact on v1 Top 5, prompt, Usage, or chat success.
- Reused the canonical Memory secret detector before embedding and rerank
  Provider egress, split embedding orchestration out of `worker.go`, and hardened
  the RAG parser transport test against the UDS bind/listen readiness race.

### Git Commits

| Hash | Message |
|------|---------|
| `dc15869` | `feat: add Memory hybrid vector shadow` |
| `2bc9517` | `fix: wait for parser sidecar readiness` |
| `b1e4889` | `docs(task): record Memory v2 PR8 progress` |

### Testing

- [OK] Focused race tests, full backend `go test ./...`, and `go vet ./...`.
- [OK] Disposable PostgreSQL 17 backfill/invalidation/lease/fence, lane/fusion,
  role denial, guarded down, clean down, and `058 -> 059 -> 058 -> 059` replay.
- [OK] Preflight tests, Compose render with API/worker, backend image build, and
  security scan across 17 changed production files with zero findings.
- [OK] RAG UDS readiness reproduced and fixed; 50 replays and 15 focused parser
  transport tests passed.
- [OK] Full standalone gate: frontend 954 tests/build, backend tests/vet, and
  RAG 1,906 passed / 7 skipped.

### Status

[OK] **PR8 completed; parent Memory v2 task remains in progress**

### Next Steps

- Begin PR9 Project/Conversation policy and governance UI without switching the
  v2 reader before its frozen promotion gates pass.


## Session 23: Implement Memory v2 portability, restore replay, and retention

**Date**: 2026-07-28
**Task**: Implement Memory v2 PR10 encrypted portability and backup deletion closure
**Branch**: `main`

### Summary

Implemented PR10 authenticated `age` Export/Import, deterministic dry-run and
ADD-only confirm fencing, encrypted off-host deletion replay, verified backup-
set retention, and Server mode governance UI while keeping the v1 Global Top 5
as the only prompt and Usage reader.

### Main Changes

- Added migration `061` with ID/hash/status-only import and deletion-replay
  authority, imported revision history, narrow SECURITY DEFINER capabilities,
  runtime table-CRUD denial, and guarded rollback.
- Added canonical bounded JSONL inside pinned `filippo.io/age v1.3.1` scrypt
  streams, current-user scope mapping, secret-before-persistence controls,
  HMAC package/plan/state fencing, confirm idempotency, and imported
  supersession mapping.
- Added operator deletion export/replay and default-dry-run backup retention
  bound to a deterministic plan hash, with checksum, traversal, and symlink
  fail-closed behavior.
- Added Server mode Export/Import UI and typed client boundaries; Local mode
  remains unsupported and imported settings remain suggestion-only.
- Hardened final review findings: normalized duplicate mapping rejection,
  final-chunk authentication before Project secret errors, and dry-run handling
  for unavailable imported-history scopes.

### Git Commits

| Hash | Message |
|------|---------|
| `19eaafd` | `feat: add encrypted Memory portability` |
| `3d51e2e` | `feat: add deletion replay and backup retention` |
| `236646d` | `feat: add Memory portability governance UI` |
| `203f35c` | `docs: document Memory portability and restore operations` |

The final task-record commit contains this PRD/info/research/journal update.
Nothing was pushed or archived.

### Testing

- [OK] Focused portability, retention, and migration contract tests.
- [OK] `go test -race ./internal/usermemory ./internal/migration ./cmd/migrate ./cmd/admin`,
  full backend tests, and `go vet ./...`.
- [OK] PostgreSQL 17 portability round-trip, runtime-role denial, deletion
  replay/projection rebuild, empty apply, guarded `060 -> 061 -> 060 -> 061`,
  and final disposable database restoration to migration `059`.
- [OK] Frontend format/lint/typecheck, 198 files / 961 tests, and production
  build.
- [OK] Single-server preflight and full standalone gate: frontend 961 tests,
  backend tests/vet, and RAG 1,906 passed / 7 skipped.
- [OK] Change, quality, and targeted security gates passed. The whole-frontend
  scanner reported only pre-existing non-diff detector fixtures/sanitized HTML
  paths; PR10 production diffs added no Critical/High finding.

### Status

[OK] **PR10 completed; parent Memory v2 task remains in progress**

### Next Steps

- After the approved batched commits, begin PR11 L2 Scene shadow/promotion as a
  separate rollback domain without promoting the v2 reader early.


## Session 24: Implement Memory v2 L2 Scene shadow and promotion

**Date**: 2026-07-28
**Task**: Implement Memory v2 PR11 L2 Scene shadow/promotion
**Branch**: `main`

### Summary

Implemented default-off, same-scope L2 Scenes as rebuildable PostgreSQL-derived
data with independent generation, leased synthesis/embedding/purge, hybrid
retrieval, Server governance, and migration-owner-only promotion/rollback while
retaining v1 L1 as the default prompt and Usage authority.

### Main Changes

- Added migration `062` with Scene/member authority tuples, derived hybrid
  projection, content-free observations, refresh/embedding/purge jobs,
  immediate stale plus 24-hour provider-free purge, least-privilege runtime
  capabilities, promotion evidence gates, and guarded rollback/replay.
- Added fake-testable Go worker synthesis and derived embedding orchestration,
  secret/Sensitive zero-egress controls, deadline/lease/profile/watermark
  fences, exact/CJK BM25/vector RRF/rerank Scene recall, 2-second cutoff, and a
  hard two-Scene/500-token budget.
- Wired shadow and active lanes into Chat with independent default-off flags;
  shadow remains zero-injection, active requires database promotion plus current
  L1/user/Scene authority, and every failure falls back to unchanged L1.
- Added typed Server-only governance API/UI for Scene profile, detail,
  enable/disable, per-Scene/all rebuild, and correction through canonical L1;
  no Scene plaintext PATCH or Local adapter was introduced.
- Fixed promotion evidence key ordering and a synthetic canary timestamp
  precision flake; added static ordering and PostgreSQL microsecond-boundary
  regression coverage plus the executable L2 code-spec.

### Testing

- [OK] PostgreSQL 17 Scene apply/search/stale/provider-free purge/promotion/
  active injection/rollback and guarded `061 -> 062 -> 061 -> 062`; promotion
  lifecycle passed 10 consecutive replays after the precision fix.
- [OK] Focused race for Memory worker/user memory/migration, full backend tests,
  and `go vet ./...`.
- [OK] Frontend format/lint/typecheck, 198 files / 961 tests, and production
  build.
- [OK] RAG Ruff/mypy/pytest: 1,906 passed / 7 skipped.
- [OK] Single-server preflight, Compose clean-copy render, backend image build,
  full standalone gate, diff check, change/quality gates, and targeted security
  review. Whole-tree scanner findings were pre-existing test fixtures and
  sanitized HTML/PEM parser patterns; PR11 diffs added no Critical/High issue.
- [OK] No Live Provider call and no Live Memory read or mutation.

### Status

[OK] **PR11 completed; parent Memory v2 task remains in progress**

### Next Steps

- After the approved batched commits, begin PR12 L3 Persona shadow/promotion as
  a separate generation and rollback domain without enabling it before formal
  evidence passes.


## Session 25: Implement Memory v2 L3 Persona shadow and promotion

**Date**: 2026-07-29
**Task**: Implement Memory v2 PR12 L3 Persona shadow/promotion
**Branch**: `main`

### Summary

Implemented default-off, Global stable-L1 L3 Persona as rebuildable PostgreSQL-
derived data with independent generation, leased synthesis/embedding/purge,
hybrid retrieval, Server governance, and migration-owner-only promotion/
rollback while retaining v1 L1 as the default prompt and Usage authority.

### Main Changes

- Added migration `063` with Persona/member authority tuples, derived hybrid
  projection, content-free observations, refresh/embedding/purge jobs,
  immediate stale plus 24-hour provider-free purge, least-privilege runtime
  capabilities, account-cascade safety, and guarded rollback/replay.
- Added fake-testable Go synthesis and embedding orchestration with strict
  single-Persona JSON, stable Global L1 filtering, secret/Sensitive zero-egress,
  deadline/lease/profile/watermark fences, and token/member recomputation.
- Added independent Exact/CJK BM25/vector RRF/rerank retrieval, lexical/RRF
  fallback, a one-Persona/300-token bound, and a lower-priority active prompt
  block that never contains Persona/member IDs.
- Added typed Server-only governance API/UI for Persona profile, detail,
  enable/disable, rebuild, current source/evidence hydration, and correction
  through canonical L1; no derived plaintext PATCH was introduced.
- Bound the fixed SiliconFlow hybrid provider whenever L3 shadow is enabled,
  even when L1/L2 provider-backed flags remain disabled.

### Testing

- [OK] Disposable PostgreSQL 17 refresh/search/Sensitive fencing/lease reclaim/
  token spoof/lexical fallback/disabled preservation/promotion denial/runtime
  role denial/account cascade and guarded `062 -> 063 -> 062 -> 063`.
- [OK] Focused race for Memory worker/user memory/chat/http/migration, full
  backend tests, and `go vet ./...`.
- [OK] Frontend format/lint/typecheck, 198 files / 961 tests, and production
  build.
- [OK] RAG Ruff/source-tree mypy/pytest: 1,906 passed / 7 skipped.
- [OK] Example and active Compose rendering, preflight regression suite,
  backend image/four-binary gate, and full standalone gate.
- [OK] Diff check, released migration immutability, changed-file security scan,
  and change/quality gates; no changed production-file security finding.
- [OK] No Live Provider call and no Live Memory read or mutation.

### Operational Note

The protected live `.env.single-server` remains untouched. It renders with
Compose but predates the current strict preflight contract and must be migrated
separately before the next live deployment.

### Git Commits

Pending the required one-shot approval of the exact batched commit plan. No
commit was created, amended, or pushed during verification.

### Status

[OK] **PR12 completed; parent Memory v2 task remains in progress**

### Next Steps

- After approved batched commits, begin PR13 as an isolated fixture-only
  Hindsight adapter/profile. Any real trial remains separately authorized.


## Session 21: Complete Memory v2 PR13 Hindsight fixture evaluation

**Date**: 2026-07-29
**Task**: Complete Memory v2 PR13 Hindsight fixture evaluation
**Branch**: `main`

### Summary

Implemented and verified the isolated synthetic Hindsight dual-profile adapter, recorded 8/10 content-free results for both profiles, proved failure and signal teardown, removed all Hindsight runtime/image state, and completed the PR1-PR13 Memory v2 implementation plan without enabling a production reader or real-data trial.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `33a0a35` | (see git log) |
| `5b0c6c2` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 22: Memory benchmark authoring foundation

**Date**: 2026-07-29
**Task**: Memory benchmark authoring foundation
**Branch**: `main`

### Summary

Delivered the deterministic 650-case Memory benchmark authoring toolchain, protected human-review ledger and loopback UI, exact freeze and one-shot Holdout enforcement, content-free status evidence, documentation, and full verification gates. Formal 500-case human review remains a separate operational task.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `5436bf6` | (see git log) |
| `a7e53f3` | (see git log) |
| `ba7d509` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 23: Memory v2 regression benchmark delivery

**Date**: 2026-07-29
**Task**: Memory v2 regression benchmark delivery
**Branch**: `main`

### Summary

Rebuilt the compromised Memory review corpus as an isolated 500-case machine-reviewed regression benchmark, preserved the v1 evidence chain, kept formal Golden admission unchanged, documented the non-promotion contract, and passed focused, backend, security, quality, replay, and full standalone verification.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `59c11e7` | (see git log) |
| `7e41f27` | (see git log) |
| `5a7471f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 24: Native Memory regression runner

**Date**: 2026-07-29
**Task**: Native Memory regression runner
**Branch**: `main`

### Summary

Implemented and verified the isolated production-reader Memory regression capture lane, deterministic fake protocol, strict live/cost gates, exclusive evidence publication, Compose teardown, and non-promotional documentation; live SiliconFlow comparison remains separately authorized.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ce1544f` | (see git log) |
| `52af80e` | (see git log) |
| `9ca627b` | (see git log) |
| `78c9249` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 25: Complete L1 Memory relevance and validation tooling

**Date**: 2026-08-06
**Task**: Complete L1 Memory relevance and validation tooling
**Branch**: `main`

### Summary

Completed the L1 Memory product path: transport-stable fixed-Luna recall, safe auto-capture, cross-model explicit-read routing, server-owned background lifecycle, fail-closed health, and isolated schema-v15 production-policy Validation tooling. Full standalone, focused race, shell lifecycle, security, and fake PostgreSQL/Compose checks passed. No live schema-v15 Provider request or Release claim was made; production remains Beta pending separately authorized live Validation.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `3976f659` | (see git log) |
| `cb0f1c2e` | (see git log) |
| `7f4ecca3` | (see git log) |
| `0dbb24cc` | (see git log) |
| `59343b67` | (see git log) |
| `5d421e3b` | (see git log) |
| `c79d248b` | (see git log) |
| `20d5f57d` | (see git log) |
| `6ff18816` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 26: Vault-backed Memory production validation

**Date**: 2026-08-06
**Task**: Vault-backed Memory production validation
**Branch**: `main`

### Summary

Added owner-authorized one-shot Vault credential export and cleanup-safe schema-v15 Memory production validation workflow; completed the live run, which returned Orange due to false-injection rate 0.09 and requires disabling recall while preserving data, without changing runtime flags.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `b4b6e05a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 27: Disable Memory recall while preserving data

**Date**: 2026-08-06
**Task**: Disable Memory recall while preserving data
**Branch**: `main`

### Summary

Applied the schema-v15 Orange containment action: disabled Memory Tool recall and hybrid shadow Provider work, preserved all Memory data with a protected logical dump, pinned a schema-069-compatible backend image after detecting mutable local-tag drift, verified both services healthy and 43 Memory relations unchanged, and documented immutable image pinning for live recreation.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `beb4cb74` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 28: Remediate Memory false injection

**Date**: 2026-08-06
**Task**: Remediate Memory false injection
**Branch**: `main`

### Summary

Added a hash-bound Development-only bilingual negative meta-policy guard, preserved the production policy hash and disabled runtime, verified zero candidate Provider egress on abstention, recorded the consumed-Validation offline audit, and passed focused race, backend, and full standalone gates.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `022a1168` | (see git log) |
| `6b755d3f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
