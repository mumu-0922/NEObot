# Journal - mm (Part 2)

> Continuation from `journal-1.md` (archived at ~2000 lines)
> Started: 2026-08-14

---



## Session 49: Agent Runtime G20.9 legacy Skill retirement

**Date**: 2026-08-14
**Task**: Agent Runtime G20.9 legacy Skill retirement
**Branch**: `main`

### Summary

Hard-deleted legacy browser text-Skill state, UI, catalogs and prompt execution; added marker-last browser retirement, history fact projection, backend anti-resurrection, backup/count-gated PostgreSQL cutover, focused gates, synchronized docs/specs, and verified the Runtime remains held at ISOLATION_UNAVAILABLE.

### Main Changes

- Added persistent Chat/Agent modes with physical Agent Tool admission and
  model-capability downgrade.
- Moved MCP behind the Connector layer, repaired Browser execution, and added
  authenticated chat artifact publication/download.
- Split Skill Store from Agent Center and retired the unused legacy Runner,
  Orchestrator, Broker, Delegation, Canary, Schedule, and Learning control
  plane through migration `098`.
- Repaired the Memory regression taxonomy-v2 validator by admitting the
  canonical `PROVIDER_CONTEXT_OVERFLOW` category.

### Git Commits

| Hash | Message |
|------|---------|
| `f33a3db5` | (see git log) |

### Testing

- [OK] `bash scripts/test-memory-regression.sh`
- [OK] `bash scripts/verify-standalone.sh --full`
- [OK] Frontend: 190 Vitest files / 922 tests and production build
- [OK] Backend: `go vet ./...` and `go test ./...`
- [OK] RAG: Ruff, mypy, and 1906 passed / 7 skipped pytest cases
- [OK] Migration `098` PostgreSQL 17 cleanup and preservation drill

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 50: Agent Runtime G20.10 production closure

**Date**: 2026-08-14
**Task**: Agent Runtime G20.10 production closure
**Branch**: `main`

### Summary

Closed the offline production operations contract with conservative policy defaults, strict release-bound closure evidence, a fail-closed evaluator/self-test, incident/retention/backup/rotation runbooks, and full Agent/PostgreSQL/standalone verification; exact-host promotion remains honestly held at ISOLATION_UNAVAILABLE.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `14b6222f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 51: Agent Runtime G21.0 production control activation

**Date**: 2026-08-14
**Task**: Agent Runtime G21.0 production control activation
**Branch**: `main`

### Summary

Added the exact-host Runner bundle, strict control-plane activation evidence, dedicated least-privilege maintenance worker, default-off Compose/preflight wiring, documentation, and full verification while keeping execution disabled and this host ISOLATION_UNAVAILABLE.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `b14ac9cb` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 52: Agent Runtime G21.1 Root Run canary

**Date**: 2026-08-14
**Task**: Agent Runtime G21.1 Root Run canary
**Branch**: `main`

### Summary

Added the separately activated Root Run canary, caller-scoped Runner RPC policy, immutable no-network/no-Tool plan, signed launch/heartbeat/cancel flow, atomic PostgreSQL terminalization, restart recovery, default-off production wiring, focused PostgreSQL/preflight gates, and synchronized architecture/contract/deployment/Trellis documentation. Final full standalone verification passed while the development host remained ISOLATION_UNAVAILABLE.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `437faf27` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 53: Agent Runtime G21.2 Broker and Artifact canary

**Date**: 2026-08-14
**Task**: Agent Runtime G21.2 Broker and Artifact canary
**Branch**: `main`

### Summary

Added the separately activated Broker/Artifact canary, private Runner-to-Broker mTLS relay, strict five-action read-only/Artifact plan, migration 091 function-only Artifact authority, object-before-row cleanup, outcome_unknown no-retry handling, default-off production wiring, enabled synthetic preflight, and synchronized contracts/specs. Backend, PostgreSQL 17, G21.0/G21.1 regressions, Phase 0, and full standalone verification passed while this development host remained ISOLATION_UNAVAILABLE and no exact-host/live proof was produced.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `962c2647` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 54: Agent Runtime G21.3 bounded Project mutation canary

**Date**: 2026-08-15
**Task**: Agent Runtime G21.3 bounded Project mutation canary
**Branch**: `main`

### Summary

Added the independently activated, offline-approved synthetic Project CAS canary, caller-specific private relay, migration 092 function-only CAS/status/cleanup authority, durable acknowledgement-loss no-retry recovery, default-off production wiring, and synchronized contracts/specs. PostgreSQL 17, G21.0-G21.2 regressions, Phase 0, security/quality gates, and full standalone verification passed while this development host remained ISOLATION_UNAVAILABLE and no exact-host/live promotion evidence was produced.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `0a62b406` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 55: Complete Agent Runtime G21.4 depth-one Child canary

**Date**: 2026-08-15
**Task**: Complete Agent Runtime G21.4 depth-one Child canary
**Branch**: `main`

### Summary

Added the default-off G21.4 one-Parent/one-Child production-path canary, migration-093 atomic reap transport, restart recovery, exact activation/preflight/Compose wiring, migrated every PostgreSQL tail drill through head 093, and passed focused, Phase 0, Backend, PostgreSQL 17, security, and full standalone verification while retaining ISOLATION_UNAVAILABLE.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `3f5a366b` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 56: Complete Agent Runtime G21.5 exact workers

**Date**: 2026-08-15
**Task**: Complete Agent Runtime G21.5 exact workers
**Branch**: `main`

### Summary

Added independent exact-target Cron and quarantined Draft-learning workers, migration 094 least-privilege authority, lifecycle-only Runner result transport, production wiring, PostgreSQL 17 recovery gates, and full documentation. Full standalone passed while this host remained ISOLATION_UNAVAILABLE.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `9013926f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 57: Complete Agent Runtime G21.6 product canary

**Date**: 2026-08-15
**Task**: Complete Agent Runtime G21.6 product canary
**Branch**: `main`

### Summary

Implemented and verified migration 095, the authenticated bounded product-canary queue and worker, exact Runner caller, final closure/promotion contracts, read-only health, canonical replay bindings, deployment wiring, frontend action, PostgreSQL drills, and full standalone closure while retaining ISOLATION_UNAVAILABLE on this host.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `a9ca4a57` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 58: Agent Runtime WSL local-test host tooling

**Date**: 2026-08-16
**Task**: Agent Runtime WSL local-test host tooling
**Branch**: `main`

### Summary

Added a fail-closed Ubuntu 22.04 WSL2 local-test Runner bootstrap, exact pinned rootless Podman toolchain installer, real bounded synthetic Skill smoke, local-only report contract, guarded rollback, docs, tests, and full verification while production remained ISOLATION_UNAVAILABLE.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `3546b990` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 59: Agent Skill local direct execution

**Date**: 2026-08-16
**Task**: Agent Skill local direct execution
**Branch**: `main`

### Summary

Replaced the mandatory WSL/Podman Skill activation path with Hermes-style local_direct execution: owner-bound Skill materialization, native Chat Skill/terminal Tools, bounded Backend-user commands, single-server wiring, local-ready UI, docs/specs, and full verification.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `8a2eca7e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 60: Restore local Skill browser execution

**Date**: 2026-08-16
**Task**: Restore local Skill browser execution
**Branch**: `main`

### Summary

Fixed OpenAI strict local Tool schemas, canonical PostgreSQL UUID validation, and frontend local_direct Tool event normalization; rebuilt the stack and verified Agent Center plus a real workspace-smoke browser retry created /workspace/hello-skill.txt with skill-ok.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `c8e4c734` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 61: Lower Server Auth password minimum

**Date**: 2026-08-16
**Task**: Lower Server Auth password minimum
**Branch**: `main`

### Summary

Lowered the shared Server Auth password minimum to nine characters, synchronized tests and documentation, rebuilt the local backend, rotated the Owner credentials through the recovery transaction, revoked sessions, and verified live login/logout.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `80e03a7a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 62: Converge Chat Agent product runtime

**Date**: 2026-08-20
**Task**: Converge Chat Agent product runtime
**Branch**: `main`

### Summary

Unified Chat and Agent modes, moved MCP behind connectors, added durable artifacts and local_direct operations, retired the legacy Agent control plane, and repaired the final Memory regression gate; full standalone verification passed.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `da0c756f` | (see git log) |
| `8750ec98` | (see git log) |
| `e3df158b` | (see git log) |
| `b44b8799` | (see git log) |
| `3abdf135` | (see git log) |
| `01970ccd` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 63: Deploy and accept Agent tool admission repair

**Date**: 2026-08-20
**Task**: Deploy and accept Agent tool admission repair
**Branch**: `main`

### Summary

Fixed first-request Agent capability admission, restored immutable migration history with forward-only repair 099, deployed pinned local images with paired backup and rollback points, and passed a real file write/read/publish/download acceptance flow.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `0edd0d1e` | (see git log) |
| `0b8d5157` | (see git log) |
| `6990509e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 64: Restore live Agent tools and authorize local project workspace

**Date**: 2026-08-20
**Task**: Restore live Agent tools and authorize local project workspace
**Branch**: `main`

### Summary

Fixed live local_direct Tool event normalization, added a single-root Linux/WSL workspace alias for Oncall_Agent, passed the full standalone gate, deployed immutable Backend/Frontend images with retained rollback state, and accepted a real pre-reload Agent file_read against the WSL UNC README.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `e0ce9970` | (see git log) |
| `d2cf5152` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 65: Repair and deploy Terminal-only Agent completion

**Date**: 2026-08-20
**Task**: Repair and deploy Terminal-only Agent completion
**Branch**: `main`

### Summary

Stopped foreground Terminal-only turns from entering the mutation verifier, added exact local evidence IDs and exact background Job completion tracking, passed full verification, deployed Backend-only with rollback protection, and accepted the original pwd/git-status request without verify_completion.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `e7ba11f2` | (see git log) |
| `4e9cd791` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 66: Fix Chat/Agent mode menu layout and automate commits

**Date**: 2026-08-20
**Task**: Fix Chat/Agent mode menu layout and automate commits
**Branch**: `main`

### Summary

Fixed multiline Chat/Agent composer menu rows with content-driven height and safe wrapping, added regression coverage and frontend spec guidance, passed all frontend quality gates, and recorded the user's automatic-commit preference.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `1e3a532a` | (see git log) |
| `c9fa2642` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 67: Deploy and verify live Chat/Agent menu fix

**Date**: 2026-08-20
**Task**: Deploy and verify live Chat/Agent menu fix
**Branch**: `main`

### Summary

Traced the unchanged UI to a stale Frontend image, retained an exact rollback image and protected environment, built and deployed a Frontend-only candidate from fix commit 1e3a532a, proved the live edge serves the new immutable CSS with correct override order, kept unrelated containers unchanged, passed live readiness and the full standalone gate, and added a release-propagation verification contract.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `23e50d2a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 68: Rename and deploy search menu labels

**Date**: 2026-08-20
**Task**: Rename and deploy search menu labels
**Branch**: `main`

### Summary

Renamed OpenAI built-in search and server search labels to localized built-in/Tavily wording, added exact focused locale/provider tests, persisted the proportional-verification rule, built and deployed a Frontend-only image with retained rollback state, and verified live compiled labels and readiness without running unrelated full suites.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `edb11b06` | (see git log) |
| `10b3792c` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
