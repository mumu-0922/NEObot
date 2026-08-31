# Fix GitHub CI Fresh-Checkout Parity

## Goal

Make the committed repository produce the same frontend and operational checks
as the developer working tree, so GitHub Actions succeeds from a clean checkout.

## Problem

- `mm-chat/.gitignore` ignores every directory named `data`, including
  `mm-chat/frontend/src/lib/data/`.
- Three required frontend source modules exist locally but are absent from Git,
  so clean checkouts fail TypeScript resolution and the frontend Docker build.
- Release verification scripts are executable locally but committed with mode
  `100644`; a clean checkout cannot execute `release-images.sh` from its test.

## Requirements

1. Anchor runtime-state ignores to the `mm-chat/` root so nested source
   directories named `data` remain trackable.
2. Add the existing frontend browser export, clear-data, and import-package
   modules to Git without changing their product behavior.
3. Commit executable mode for the standalone/release operator scripts required
   by CI.
4. Preserve `mm-chat/data/`, `mm-chat/secrets/`, `mm-chat/backup/`, and the live
   environment file as ignored runtime state.
5. Record the clean-checkout contract in the operations specification.
6. Push the focused repair and monitor GitHub Actions through terminal state.
7. Remove undeclared runner-tool assumptions, keep wall-clock acceptance tests
   isolated from parallel file contention, and emit actionable Ruff annotations
   when the remote lint gate fails.
8. Preserve Compose `create_host_path: false` verification across serializers
   that omit explicit false values from rendered JSON, while still rejecting an
   explicit true value.

## Acceptance Criteria

- A tree created only from staged Git content contains all three frontend data
  modules and does not contain protected runtime state.
- `git ls-files -s` records the required operator scripts as executable.
- Standalone verification succeeds from the Git-only tree.
- Frontend format, lint, typecheck, tests, and production build succeed from the
  Git-only tree.
- RAG formatting, lint, typecheck, and tests succeed from the Git-only tree.
- GitHub CI and Docker workflows complete successfully for the pushed commit.

## Scope

- `mm-chat/.gitignore`
- `mm-chat/frontend/src/lib/data/`
- `mm-chat/scripts/` executable metadata
- `.trellis/spec/operations/repository-root-boundary.md`
- `.github/workflows/ci.yml`
- `mm-chat/frontend/vitest.config.ts`
- `.trellis/spec/frontend/quality-guidelines.md`
- Task records under this directory

## Non-goals

- No product behavior, API, persistence schema, dependency, or deployment
  topology changes.
- No deletion or rewriting of local runtime state.

## Verification

Build a temporary archive from the staged Git tree, then run the standalone,
frontend, and RAG gates there. After the local gates pass, push and inspect the
GitHub Actions run and annotations until all required jobs reach a terminal
state.
