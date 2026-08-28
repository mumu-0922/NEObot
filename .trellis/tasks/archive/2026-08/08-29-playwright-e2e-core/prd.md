# Playwright E2E foundation and core journeys

## Goal

Add a deterministic Playwright browser-test layer for the highest-risk user
journeys without consuming a real model-provider quota.

## Scope

### In scope

1. Authentication lifecycle
   - unauthenticated users are routed to login
   - login opens the application
   - refresh preserves the authenticated session
   - an expired server session returns to login
2. Per-conversation model state
   - two conversations can retain different selected models
   - a refresh restores each conversation's last selected model
   - changing one conversation does not mutate the other
3. Agent Harness lifecycle
   - a running agent remains visibly running when the user switches chats
   - two conversations in the same workspace can run independently
   - tool/progress output remains interleaved in execution order
   - cancellation and failed runs reach a terminal UI state
   - refresh restores the server-owned terminal state
   - generated workspace artifacts can be opened in the in-app preview
4. Test infrastructure
   - Chromium Playwright project
   - deterministic API fixture; no real Provider calls
   - screenshot, trace, and video retained on failure
   - focused CI job and local scripts

### Out of scope

- RAG ingestion and retrieval E2E
- Memory extraction and recall E2E
- Skill and MCP installation/execution E2E
- real-provider model quality or billing smoke tests
- cross-browser coverage beyond Chromium in the first batch

## Acceptance criteria

- `pnpm test:e2e` runs the first-batch journeys from a clean checkout after the
  documented browser install step.
- Tests never require or read a real Provider secret.
- Each test owns isolated fixture state and can run repeatedly.
- Failure artifacts make the last browser state and network/run trace visible.
- Existing frontend unit tests, lint, type-check, and build remain green.
- CI runs the deterministic E2E suite separately from unit tests.

## Design constraints

- Exercise the production UI and `/mm-api` browser contract; do not add
  test-only branches to product components.
- Prefer accessible roles and visible text over brittle CSS selectors. Add a
  stable test id only where no user-facing locator can express the contract.
- Fixture state must model server ownership across page reloads.
- Keep real-provider smoke tests manual and explicitly opt-in.

## Verification

- focused Playwright specs during implementation
- frontend format, lint, type-check, unit tests, and production build
- Playwright suite with failure artifact settings enabled
