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
