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


## Session 117: Live deployment smoke and Skill cleanup

**Date**: 2026-08-30
**Task**: Live deployment smoke and Skill cleanup
**Branch**: `main`

### Summary

Verified live Chat, Agent, RAG, Memory, Skill, and MCP paths; fixed owner-bound Skill selection cleanup before Conversation soft delete; passed backend, race, and disposable PostgreSQL 17 gates.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ae221181` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 118: Deploy Skill cleanup with restore rehearsal

**Date**: 2026-08-30
**Task**: Deploy Skill cleanup with restore rehearsal
**Branch**: `main`

### Summary

Built and deployed Backend skill-cleanup-ae221181 after matched PostgreSQL/MinIO backup and isolated restore drills; verified authenticated Conversation Skill-selection cleanup without Provider calls, preserved unrelated services and rollback inputs, and replaced the stale hard-coded restore migration manifest with dynamic live/restored authority comparison.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `f1dc33e5` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 119: Harden five-image production promotion

**Date**: 2026-08-30
**Task**: Harden five-image production promotion
**Branch**: `main`

### Summary

Completed the registry-ready five-image release bundle, added hermetic stale/invalid digest regression coverage, synchronized production deployment docs and operations specs, and left live state and registry untouched.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6c9995f4` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 120: Account security and password recovery

**Date**: 2026-08-31
**Task**: Account security and password recovery
**Branch**: `main`

### Summary

Added authenticated self-service password rotation, all-session revocation, login-page recovery, exact password policy handling, cross-layer tests and Auth E2E; passed full standalone verification and deployed pinned healthy Backend/Frontend images locally.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `fab9da79` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 121: Enforce ASCII password policy

**Date**: 2026-08-31
**Task**: Enforce ASCII password policy
**Branch**: `main`

### Summary

Changed new Server Auth passwords to 8-256 visible ASCII characters, preserved bounded legacy credential verification, synchronized frontend validation/translations/docs, added unit and Auth Playwright coverage, passed full gates, and deployed healthy local images.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `3a24a71a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 122: Restore GitHub CI fresh-checkout parity

**Date**: 2026-08-31
**Task**: Restore GitHub CI fresh-checkout parity
**Branch**: `main`

### Summary

Tracked ignored frontend data modules, restored executable metadata, stabilized the frontend performance gate, aligned standalone checks with GitHub Compose semantics, pinned RAG interop runtimes, and verified final CI plus Docker workflows green.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `2c83d033` | (see git log) |
| `9dfe4e77` | (see git log) |
| `9adec044` | (see git log) |
| `0c1e7eb1` | (see git log) |
| `99e54716` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 123: Rename frontend brand to NeoBot

**Date**: 2026-09-01
**Task**: Rename frontend brand to NeoBot
**Branch**: `main`

### Summary

Renamed the user-facing frontend brand from Neo Chat to NeoBot across the main UI, localized authentication/account/MCP copy, SEO metadata, Open Graph, JSON-LD, and PWA surfaces while preserving internal compatibility identifiers and existing artwork. Added focused brand assertions and documented the external-brand boundary.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `700a23a9` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 124: Replace NeoBot logo with F1-A

**Date**: 2026-09-01
**Task**: Replace NeoBot logo with F1-A
**Branch**: `main`

### Summary

Replaced the frontend logo with the selected F1-A vector mark, added deterministic PNG and multi-size favicon generation, updated metadata and PWA assets, and added focused asset tests and documentation.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `20b5d06a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 125: Redeploy F1-A frontend

**Date**: 2026-09-01
**Task**: Redeploy F1-A frontend
**Branch**: `main`

### Summary

Built and pinned the NeoBot F1-A frontend image, recreated only the frontend service, and verified live branding, health, asset hashes, unrelated container stability, and rollback readiness.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `20b5d06a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 126: Refresh README screenshots

**Date**: 2026-09-02
**Task**: Refresh README screenshots
**Branch**: `main`

### Summary

Replaced stale README desktop and mobile screenshots with deterministic current NeoBot Agent and RAG views; synchronized SEO dimensions and added an asset-dimension regression check.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `0fa55d27` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 127: Bust GitHub README screenshot cache

**Date**: 2026-09-02
**Task**: Bust GitHub README screenshot cache
**Branch**: `main`

### Summary

Renamed the refreshed README screenshot assets, updated README and frontend SEO references, added a cache-busting repository convention, and verified formatting, SEO tests, and the standalone structure gate.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `f79a9585` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 128: Supplemental provider model discovery

**Date**: 2026-09-06
**Task**: Supplemental provider model discovery
**Branch**: `main`

### Summary

Implemented and deployed explicit bounded GPT model discovery using the existing public metadata catalog plus fallback, strict synthetic chat validation, connection-scoped positive/negative caching, preserved selections, and persisted verified additions. Live Sub discovered gpt-6-astra and gpt-5.6 (5.72s first scan, 0.19s cached). Go vet/full tests/race, frontend lint/typecheck/build, focused 76 tests and Playwright discovery-save-reload passed. Full Vitest: 1027 pass, one pre-existing processTrace performance-ratio failure also reproduced on clean baseline 474f9174. Paired backup and isolated DB/MinIO restore verified; only frontend/backend recreated, image pins persisted, temporary smoke session and drill resources removed.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `475ed789` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
