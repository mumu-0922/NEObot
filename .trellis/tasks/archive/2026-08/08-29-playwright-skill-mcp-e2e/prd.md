# Playwright Skill and MCP E2E journeys

## Goal

Extend the deterministic Chromium suite with the remaining high-risk Skill and
MCP browser journeys so installation, Conversation-owned selection, Agent
execution, reload persistence, and terminal failure are proven at the real UI
and `/mm-api` boundary without live providers or third-party services.

## What I already know

- Core Chat/Agent, RAG, and Memory Playwright journeys are already green.
- Skill inventory and Conversation selection are server-authoritative through
  `/v1/skills/library` and `/v1/skills/conversations/:id/selection`.
- MCP inventory and Conversation selection are server-authoritative through
  `/v1/mcp/servers` and `/v1/mcp/conversations/:id/selection`.
- Installing a Skill changes Library inventory only; it must not silently
  select that Skill for any Conversation.
- Skill and MCP selection must survive refresh and remain isolated between
  Conversations.
- Agent process output is rendered from durable, redacted transcript events;
  the browser never invokes a Skill or MCP server directly.

## Requirements

- Add a dedicated deterministic Resource fixture for Skill and MCP state.
- Exercise Skill installation through the production Skill UI and prove the
  installed package appears without becoming implicitly selected.
- Exercise compact Skill and MCP composer selection and prove exact
  Conversation isolation plus reload persistence.
- Exercise an Agent answer containing interleaved Skill and MCP process cards
  followed by a terminal answer.
- Exercise a decisive failed Tool outcome that reaches a stable terminal UI
  and removes the running indicator.
- Keep every request inside intercepted `/mm-api`; use no Provider quota,
  LobeHub/GitHub request, or live MCP server.

## Acceptance Criteria

- [x] Installed Skill is visible after UI installation and Library refresh.
- [x] Installation alone leaves every Conversation Skill selection empty.
- [x] Conversation A Skill/MCP selections survive `page.reload()`.
- [x] Conversation B retains its independent empty selections.
- [x] Agent transcript renders Skill and MCP steps in durable order before the
      final answer.
- [x] Failed MCP execution renders a terminal failed step/answer state and no
      Conversation running indicator remains.
- [x] Resource-focused Chromium tests pass with zero unhandled fixture routes.
- [x] Changed files pass Prettier, ESLint, and TypeScript checks.
- [x] E2E documentation records the new Resource fixture and journeys.

## Definition of Done

- Deterministic Playwright fixture and focused specs are committed.
- Focused Chromium, formatting, lint, and typecheck checks pass.
- No product-only E2E branches, real secrets, external requests, sleeps, or
  disabled tests are introduced.

## Technical Approach

Compose `NeoChatApiFixture` with a small `NeoChatResourceApiFixture`, matching
the existing Knowledge and Memory fixture pattern. The Resource fixture owns
Library/install inventory, revision-bound per-Conversation Skill/MCP
selections, MCP inventory, and strict fixture request validation. Reuse the
existing delayed SSE completion control and generate schema-version-1 Skill
and MCP presentations inside Transcript v2 events.

## Decision (ADR-lite)

**Context:** Browser E2E must prove cross-component behavior but must not test
third-party availability or duplicate backend package/runtime tests.

**Decision:** Mock only the authenticated `/mm-api` boundary with mutable
server-authoritative fixture state, and use production UI controls plus durable
Agent events.

**Consequences:** The suite proves browser integration, reload semantics, and
failure convergence deterministically. Go integration tests remain responsible
for PostgreSQL CAS, package admission, real MCP protocol execution, and remote
marketplace behavior.

## Out of Scope

- Live LobeHub, GitHub, Provider, or MCP Server calls.
- Marketplace pagination/detail compatibility matrices already owned by Vitest
  and backend integration tests.
- Credentials, OAuth, private MCP Server lifecycle, or real Skill package
  extraction/runtime execution.
- Product behavior changes unrelated to a browser-test-discovered defect.

## Technical Notes

- Relevant contracts: `.trellis/spec/frontend/skill-store.md`,
  `.trellis/spec/frontend/mcp-tools.md`,
  `.trellis/spec/frontend/resource-commands.md`,
  `.trellis/spec/frontend/agent-transcript.md`, and
  `.trellis/spec/frontend/quality-guidelines.md`.
- Relevant production surfaces:
  `ConversationResourcePickers.tsx`, `SkillStore.tsx`,
  `McpToolsPage.tsx`, server Skill/MCP API clients, and transcript projection.
- Expansion sweep: future auto-selected Run snapshots, credentialed MCP
  installation, and live remote compatibility are preserved as backend-owned
  extension points but excluded from this deterministic browser batch.
