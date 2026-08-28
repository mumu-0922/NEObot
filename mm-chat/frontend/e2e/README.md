# Browser E2E tests

This module exercises Neo Chat's highest-risk browser journeys with Playwright
and a deterministic `/mm-api` fixture. It runs the production UI in Server
mode without reading Provider credentials or making billable model requests.

## Responsibilities

- verify login, session refresh, and expired-session recovery;
- verify Conversation-owned model selection across switching and reload;
- verify concurrent Conversation-owned Agent Runs, failure, cancellation, and
  detached-run refresh recovery;
- verify durable Agent transcript ordering and in-app workspace-file preview;
- retain a trace, screenshot, and video when a browser test fails.

## Run locally

From `mm-chat/frontend/`:

```bash
corepack pnpm install --frozen-lockfile
corepack pnpm test:e2e:install
corepack pnpm test:e2e
```

Use `corepack pnpm test:e2e:headed` for an interactive browser. The suite
starts its own Next.js development server on port `3100`; override it with
`PLAYWRIGHT_PORT` when needed.

## Structure

- `auth.spec.ts`: login, refresh, and invalid-session behavior.
- `conversation-model.spec.ts`: per-Conversation model isolation.
- `agent-harness.spec.ts`: Agent Run lifecycle, transcript, and artifacts.
- `fixtures/neoChatApi.ts`: isolated request/state fixture.
- `fixtures/neoChatApiSupport.ts`: deterministic DTO and SSE builders.
- `fixtures/neoChatApiTypes.ts`: shared fixture-only types.

The fixture owns only test data. Product components contain no E2E-only branch.

## Dependencies

- Node.js 22
- pnpm 10.30.3
- `@playwright/test`
- Playwright Chromium
