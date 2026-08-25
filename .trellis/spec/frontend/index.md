# Frontend Development Guidelines

> Executable conventions for the Next.js application in `mm-chat/frontend/`.

## Scope and Sources of Truth

These guidelines describe the code that exists today. The frontend is a
Next.js 16 / React 19 TypeScript application using the App Router, Tailwind
CSS 4, Zustand 5, Zod 4, Vitest 4, and `next-intl`.

Repository-wide rules in `AGENTS.md` still apply: the only product root is
`mm-chat/`, TypeScript is strict, imports may use the `@/*` alias, and frontend
behavior changes require coverage under `mm-chat/frontend/src/__tests__/`.

## Guidelines Index

| Guide                                             | Description                                                   | Status   |
| ------------------------------------------------- | ------------------------------------------------------------- | -------- |
| [Directory Structure](./directory-structure.md)   | App Router, feature, service, store, and test boundaries      | Complete |
| [Component Guidelines](./component-guidelines.md) | Components, props, composition, styling, and accessibility    | Complete |
| [Hook Guidelines](./hook-guidelines.md)           | Feature hooks, effects, store selectors, and async lifecycles | Complete |
| [State Management](./state-management.md)         | Local, Zustand, persisted, URL, and server-owned state        | Complete |
| [MCP Tools](./mcp-tools.md)                       | Tools UI, server selection authority, timeline, and Plugin-state retirement | Complete |
| [Chat/Agent Mode](./chat-agent-mode.md)           | Persisted composer mode, model-capability downgrade, and MCP-hidden product surface | Complete |
| [Agent Transcript](./agent-transcript.md)          | Flat durable Context/Think/Tool projection, live/reload parity, and legacy fallback | Complete |
| [Assistant Store](./assistant-store.md)           | My Assistants, Store paging, runtime validation, CAS recovery, and start-chat snapshots | Complete |
| [Skill Store](./skill-store.md)                   | Standalone `/v1/skills/*` package discovery, install/uninstall, URL state, and retired control-plane boundary | Complete |
| [Host Workspaces](./host-workspaces.md)            | Workspace persistence, binding, Conversation grouping, file references, and preview | Complete |
| [Type Safety](./type-safety.md)                   | Domain types, DTOs, runtime validation, and normalization     | Complete |
| [Quality Guidelines](./quality-guidelines.md)     | Formatting, linting, testing, review, and forbidden patterns  | Complete |

## Pre-Development Checklist

Before changing frontend code:

1. Read [Directory Structure](./directory-structure.md) and place the change in
   the existing product area rather than creating a parallel layer.
2. For React UI, read [Component Guidelines](./component-guidelines.md) and
   [Hook Guidelines](./hook-guidelines.md).
3. For Zustand, persistence, URL state, or API synchronization, read
   [State Management](./state-management.md).
4. For requests, stored data, imports, or migrations, read
   [Type Safety](./type-safety.md); trace the untrusted value through runtime
   validation or normalization.
5. Read [Quality Guidelines](./quality-guidelines.md), add focused Vitest
   coverage, and run the required frontend commands from `mm-chat/frontend/`.
6. For Tools UI, MCP DTO/client, timeline, or retired Plugin persistence work,
   read [MCP Tools](./mcp-tools.md) and keep execution/authorization server-
   authoritative.
7. For composer Chat/Agent mode or model Tool-capability behavior, read
   [Chat/Agent Mode](./chat-agent-mode.md); persist requested intent and keep
   effective Tool policy server-authoritative.
8. For Agent execution history, Context/Think blocks, Tool timeline, or Agent
   event normalization, read [Agent Transcript](./agent-transcript.md); preserve
   durable sequence authority and the legacy fallback for old messages.
9. For Assistant library, Store, custom editor, or start-chat changes, read
   [Assistant Store](./assistant-store.md) and keep installation and revisions
   server-authoritative.
10. For Package Skill discovery, install/uninstall, or Skill Store URL state,
   read [Skill Store](./skill-store.md). Keep `/v1/skills/*` server-authoritative
   and do not restore retired Runs, Schedules, Learning, Shadow, Canary, Runner,
    delegation, or OCI controls.
11. For Workspace sidebar/settings, Host directory selection, or Conversation
    grouping, read [Host Workspaces](./host-workspaces.md). Preserve legacy
    settings while keeping canonical paths and revisions server-authoritative.

## Representative Code

- Server/client entry boundary: `mm-chat/frontend/src/app/page.tsx` and
  `mm-chat/frontend/src/components/app/ChatApp.tsx`.
- Shared UI primitives: `mm-chat/frontend/src/components/ui/primitives.tsx`.
- Feature hook composition:
  `mm-chat/frontend/src/features/chat/hooks/useChatShellState.ts`.
- Persisted and server-backed chat state:
  `mm-chat/frontend/src/store/core/chatStore.ts`.
- Runtime request validation: `mm-chat/frontend/src/lib/api/schemas.ts`.
- Typed server client: `mm-chat/frontend/src/services/api/client/`.
- Test patterns: `mm-chat/frontend/src/__tests__/anchoredPortal.test.ts`,
  `chatStore.test.ts`, and `schemas.test.ts`.

**Language**: Project documentation is written in English.
