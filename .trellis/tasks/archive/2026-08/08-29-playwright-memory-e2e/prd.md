# Playwright Memory lifecycle journeys

## Goal

Extend the deterministic Chromium E2E layer to prove the user-visible Memory
lifecycle across server governance, answer-time recall, and direct-user Memory
activity without calling a real model Provider, embedding worker, or database.

## What I already know

- Core Agent and RAG Playwright batches already exercise the production UI at
  the same-origin `/mm-api` boundary with isolated server-owned fixture state.
- Memory settings are PostgreSQL/Go authoritative in Server mode; browser
  storage must not become a second source of truth.
- Browser E2E should prove visible cross-layer contracts, while retrieval
  ranking, planner validation, revision fences, and SQL transactions retain
  their backend/RAG test ownership.
- The next agreed batch is Memory before Skill/MCP coverage.

## Assumptions

- Chromium remains the only browser in this batch.
- Existing product components receive no E2E-only branches.
- Fixture Memory state persists across reload inside each test and never reads
  live user Memory or Provider credentials.

## Requirements

1. Open the real server Memory governance page, create a deterministic Global
   Memory, change a global policy, and prove both survive browser refresh.
2. Submit an explicit recall question in a Conversation, complete it with a
   deterministic server Memory-search process trace, and verify the recalled
   fact plus its visible search step.
3. Submit a direct-user remember command, expose its terminal Memory Activity,
   and prove revision-fenced undo removes the available undo action.
4. Keep Memory governance, health, Activity, and undo state in a dedicated
   fixture helper instead of expanding the core chat fixture into a monolith.
5. Fail every test on an unhandled `/mm-api` browser contract.

## Acceptance Criteria

- [x] Playwright proves Global Memory creation and policy persistence across
      refresh through production UI controls.
- [x] Playwright proves explicit recall -> Memory search trace -> grounded
      terminal answer.
- [x] Playwright proves direct action -> visible Activity -> successful undo.
- [x] Tests use inert local fixtures and never read real Provider secrets.
- [x] Existing 10 E2E journeys and proportional frontend gates remain green.
- [x] E2E docs describe Memory coverage and its mocked trust boundary.

## Definition of Done

- Memory Playwright specs and reusable fixture contracts are implemented.
- Focused tests, format, lint, typecheck, complete Playwright suite, and the
  proportional frontend gate pass.
- Relevant frontend spec and E2E documentation are updated.
- Verified work is committed; the Trellis task is archived and journaled.

## Out of Scope

- Real embedding, retrieval-quality, Provider planner, PostgreSQL, background
  worker, L2/L3 synthesis, import/export, or multi-user authorization tests.
- Skill/MCP installation and execution journeys.
- Cross-browser coverage and live external services.

## Technical Notes

- Add `NeoChatMemoryApiFixture` beside the existing Knowledge helper and route
  it from `NeoChatApiFixture`.
- Use production DTO shapes for governance snapshot, health, Activity, and
  undo responses.
- Use explicit pending-run controls for chat completion; never add sleeps.
- Represent answer-time recall as durable Agent transcript process steps with
  the production `search_memory`/search-card contract.
- Keep Activity polling deterministic by seeding the assistant-owned terminal
  Activity before resolving the fixture SSE run.
