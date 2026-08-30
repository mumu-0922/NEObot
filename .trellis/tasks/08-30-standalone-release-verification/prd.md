# Standalone release verification

## Goal

Prove the current Neo Chat source tree is release-ready by running the complete
isolated standalone gate, repairing only real gate failures, and repeating the
gate until every frontend, backend, RAG, Compose, and structural check passes.

## What I Already Know

- The deterministic Chromium browser suite is complete and previously passed
  17/17 journeys.
- `mm-chat/scripts/verify-standalone.sh --full` creates an isolated copy and
  runs structure/Compose checks plus frontend, backend, and RAG quality gates.
- The script excludes `mm-chat/.env.single-server`, `data/`, `secrets/`, and
  `backup/`; those paths are protected runtime state and must remain untouched.
- The agreed next phase is release verification, not new product development.

## Requirements

- Run `bash mm-chat/scripts/verify-standalone.sh --full` from the repository
  root using the checked-in lockfiles and manifests.
- Preserve the exact first failing command and component when the gate fails.
- Diagnose each failure from source/configuration/lockfiles before changing
  code.
- Apply the smallest coherent repair needed for a genuine product or gate
  defect, then run the focused owning check before repeating the full gate.
- Do not weaken, skip, suppress, or delete a failing test to obtain a green
  result.
- Do not read, rewrite, delete, or stage live runtime state, Provider secrets,
  private chats, user files, backups, or active environment files.
- Record the final gate result and any bounded residual warnings.

## Acceptance Criteria

- [x] The full standalone gate exits successfully.
- [x] Frontend format, lint, typecheck, Vitest, and production build pass in the
      isolated copy.
- [x] Backend gofmt, vet, and test checks pass in the isolated copy.
- [x] RAG frozen sync, Ruff, mypy, and pytest checks pass in the isolated copy.
- [x] Compose topology, standalone source boundaries, Agent runtime checks, and
      Memory script contract checks pass.
- [x] No product defect was discovered; no repair or regression change was
      necessary.
- [x] Protected runtime paths remain unchanged and the final working tree
      contains only task-owned source/Trellis changes.

## Definition of Done

- A clean `verify-standalone.sh --full` run is captured.
- Any necessary repair is reviewed against the owning specs and committed with
  focused verification evidence.
- The task is archived and the session journal records the verification.

## Technical Approach

Run the existing all-component gate without pre-emptive edits. On failure,
classify it as environment, deterministic test, source contract, dependency,
or Compose/release topology. Environment failures are resolved through the
documented toolchain path; source failures follow reproduce -> root cause ->
minimal fix -> focused check -> full rerun.

## Decision (ADR-lite)

**Context:** Browser E2E proves user journeys but not the clean-copy release
boundary or all component toolchains.

**Decision:** Treat the isolated full standalone script as the release
authority and keep live-provider/production smoke outside this task.

**Consequences:** A green result proves reproducible source, build, unit and
integration gates without risking live data. Real Provider, deployed health,
backup/restore, and remote MCP smoke remain the next release phase.

## Out of Scope

- New product features, UI redesign, or broad refactors.
- Live Provider billing, real LobeHub/GitHub package execution, or remote MCP
  compatibility smoke.
- Recreating the live Compose stack or restoring production backups.
- Publishing, tagging, pushing, or deploying a release.

## Technical Notes

- Gate: `mm-chat/scripts/verify-standalone.sh`.
- Operations contract:
  `.trellis/spec/operations/repository-root-boundary.md`.
- Verification must remain non-destructive and use the script's temporary-copy
  cleanup trap.

## Verification Evidence

- Date: 2026-08-30 (Asia/Shanghai).
- Command: `bash mm-chat/scripts/verify-standalone.sh --full`.
- Result: `standalone verification: passed (full)` with exit code zero.
- Frontend: Prettier, ESLint, TypeScript, 201 Vitest files / 1012 tests, and
  Next.js production build passed.
- Backend: gofmt, `go vet ./...`, and `go test ./...` passed.
- RAG: frozen Python 3.13 environment, Ruff check/format, mypy (67 source
  files), and pytest (1910 passed, 7 environment-gated integration tests
  skipped) passed.
- Structural/operations: standalone copy, Compose topology, Agent local
  runtime, Hindsight fixture, Memory regression, and every checked-in Memory
  Vault lifecycle contract passed.
- Non-blocking output: pnpm reported a newer major release; the project remains
  intentionally pinned to pnpm 10.30.3. Next.js reported the known middleware
  convention deprecation while successfully building the current Proxy output.
- Runtime protection: no change under `mm-chat/data/`, `mm-chat/secrets/`,
  `mm-chat/backup/`, or `mm-chat/.env.single-server`.
