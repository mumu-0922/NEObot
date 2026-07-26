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
