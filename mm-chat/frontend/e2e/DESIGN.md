# Browser E2E design

## Goals

- Test real browser behavior at the production React and `/mm-api` boundary.
- Keep CI deterministic, repeatable, and independent of Provider quota.
- Preserve server-owned state semantics across navigation and reload.
- Produce useful failure evidence without logging private runtime data.

## Non-goals

- Model-quality evaluation or real Provider availability checks.
- Replacing Go integration tests or frontend Vitest coverage.
- Real RAG/Memory retrieval quality, third-party Skill package execution, or
  remote MCP protocol compatibility.
- Multi-browser coverage before the Chromium suite is stable.

## Architecture

```text
Playwright Chromium
  -> production Next.js UI
  -> intercepted same-origin /mm-api requests
  -> isolated NeoChatApiFixture state
```

The UI still performs its normal login, config, Provider, Workspace,
Conversation, Message, SSE, cancellation, and preview requests. Playwright
intercepts only `/mm-api`; all application code and browser persistence remain
unchanged. Each test creates a new fixture and browser context.

The Knowledge fixture models only browser-visible server authority: uploaded
file metadata, collection/document DTOs, processing-to-active transitions,
Conversation-owned collection selection, and terminal answer metadata. Parser,
embedding, vector search, reranking, and Provider work remain outside browser
E2E and retain their backend/RAG test ownership.

The Memory fixture likewise models only browser-visible server authority:
governance snapshots, health, settings/memory mutations, assistant-owned
Activity, revision-fenced undo, and durable `search_memory` process steps.
Embedding/ranking, planner validation, PostgreSQL fences, and background jobs
remain owned by backend/RAG tests.

The Resource fixture models only browser-visible server authority: Skill
Library installation, MCP inventory, revision-bound per-Conversation
selections, and durable redacted Skill/MCP process presentations. Package
admission/extraction, Skill runtime execution, credentials/OAuth, remote MCP
protocol calls, and marketplace compatibility remain backend integration-test
responsibilities.

## Key decisions

| Decision                                    | Reason                                                                          |
| ------------------------------------------- | ------------------------------------------------------------------------------- |
| Mock at `/mm-api`, not inside React         | Exercises production UI composition and client DTO paths without test branches. |
| Keep fixture state in the test process      | Makes reload persistence deterministic and prevents cross-test data leakage.    |
| Delay SSE completion with explicit controls | Proves running indicators and cross-Conversation admission without time sleeps. |
| Use Chromium first                          | Establishes a fast stable baseline before widening browser cost.                |
| Keep real-provider smoke manual             | Prevents flaky CI, secret exposure, and billable requests.                      |
| Keep Memory state in a dedicated fixture    | Preserves reload authority without mixing governance into core Chat routes.     |
| Keep Resource state in a dedicated fixture  | Proves selection/install and transcript boundaries without third-party calls.   |

## Trust boundaries

- Fixture credentials and tokens are inert test literals and never match a
  live environment.
- The fixture intercepts only the configured local test origin's `/mm-api`
  requests; it does not proxy arbitrary URLs.
- No Provider key, chat export, runtime workspace file, or user data is read.
- Failure artifacts may contain fixture text, so CI retains them for seven
  days and uploads them only on failure.

## Known limitations

- Playwright interception validates browser-facing integration, not the Go
  router or PostgreSQL transaction path; existing backend tests own those.
- The first batch runs Chromium only.
- Login has no user-facing logout control today, so the auth journey verifies
  login, refresh retention, and server-invalidated Session recovery.

## Change history

- 2026-08-29: Added the deterministic Chromium foundation and first-batch core
  journeys.
- 2026-08-29: Added deterministic Knowledge upload, citation, and degraded
  retrieval journeys.
- 2026-08-29: Added deterministic Memory governance, recall, Activity, and undo
  journeys.
- 2026-08-29: Added deterministic Skill installation, Skill/MCP selection,
  Resource transcript, reload, and terminal MCP failure journeys.
