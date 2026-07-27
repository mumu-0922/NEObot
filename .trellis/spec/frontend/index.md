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
