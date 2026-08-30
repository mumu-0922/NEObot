# Journal - mm (Part 3)

> Continuation from `journal-2.md` (archived at ~2000 lines)
> Started: 2026-08-28

---



## Session 108: Retire built-in Playwright Browser MCP

**Date**: 2026-08-28
**Task**: Retire built-in Playwright Browser MCP
**Branch**: `main`

### Summary

Removed the deployment-managed Browser manifest authority, migrated exact stale selections and auth rows while preserving private Playwright and history, advanced MCP PostgreSQL drills to head 108, and deployed healthy Backend/Runner runtime state.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `c09b54e3` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 109: Rename Skill and MCP product labels

**Date**: 2026-08-28
**Task**: Rename Skill and MCP product labels
**Branch**: `main`

### Summary

Renamed Skill Store and Tools product-area labels to Skill and MCP across supported locales, preserved generic Tool terminology, added focused regression coverage, and deployed the healthy Frontend candidate only.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `220efe3f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 110: Hide internal Skill versions from UI

**Date**: 2026-08-28
**Task**: Hide internal Skill versions from UI
**Branch**: `main`

### Summary

Removed Skill version and fallback fingerprint labels from installed cards, marketplace cards and details, and Conversation pickers while preserving exact-version install identity; added focused coverage and deployed the healthy Frontend only.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ffa1406c` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 111: Repair Memory Worker backlog health

**Date**: 2026-08-29
**Task**: Repair Memory Worker backlog health
**Branch**: `main`

### Summary

Fixed orphaned-assistant dead-letter activity rollback, deployed the shared Backend/Memory Worker image, drained the recoverable queue, and verified a new Sub GPT-5.6 Luna extraction end to end. Historical dead letters remain append-only for separate governance.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `3e466132` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 112: Playwright core E2E foundation

**Date**: 2026-08-29
**Task**: Playwright core E2E foundation
**Branch**: `main`

### Summary

Added deterministic Chromium E2E coverage for auth, per-conversation models, and Agent Harness lifecycle; wired CI failure artifacts and isolated Vitest discovery.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `143b75dd` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 113: Playwright RAG E2E journeys

**Date**: 2026-08-29
**Task**: Playwright RAG E2E journeys
**Branch**: `main`

### Summary

Added deterministic Knowledge upload processing, Conversation-scoped collection selection, cited-answer rendering, and retrieval-degradation Playwright journeys with a reusable server-authoritative Knowledge fixture; all frontend and Chromium gates pass.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `769be70f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 114: Playwright Memory lifecycle journeys

**Date**: 2026-08-29
**Task**: Playwright Memory lifecycle journeys
**Branch**: `main`

### Summary

Added deterministic Server Memory governance persistence, recall trace, direct action Activity and revision-fenced undo browser journeys; all 13 Chromium E2E tests passed.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `54830d49` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 115: Skill and MCP Playwright E2E

**Date**: 2026-08-29
**Task**: Skill and MCP Playwright E2E
**Branch**: `main`

### Summary

Added deterministic Skill installation, per-conversation Skill/MCP selection, durable interleaved Resource transcript, reload persistence, and terminal MCP failure browser journeys; documented the fixture trust boundary and verification contract.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `3279934f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 116: Standalone release verification

**Date**: 2026-08-30
**Task**: Standalone release verification
**Branch**: `main`

### Summary

Ran the isolated full standalone release gate successfully across Compose and source boundaries, Agent/Memory topology contracts, frontend format/lint/typecheck/1012 Vitest tests/production build, all Go tests and vet, and RAG Ruff/mypy/1910 pytest tests; no product repair was required and protected runtime state remained unchanged.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `242dc600` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
