# Directory Structure

> The frontend lives only in `mm-chat/frontend/`; do not recreate application
> source, package manifests, or Next.js entrypoints at the Git root.

## Directory Layout

```text
mm-chat/frontend/
├── public/                    # Static assets
├── scripts/                   # Frontend-only maintenance/generation scripts
└── src/
    ├── app/                   # Next.js App Router pages, layouts, and API routes
    │   └── api/**/route.ts
    ├── components/            # React UI grouped by product area
    │   ├── app/               # Top-level screens and orchestration
    │   ├── chat|knowledge|settings|.../
    │   └── ui/                # Small reusable UI primitives
    ├── features/chat/hooks/   # Stateful chat-shell feature hooks
    ├── lib/                   # Domain logic, schemas, security, and helpers
    ├── services/api/          # API facade, local/server adapters, transports
    ├── store/                 # Zustand core stores, selectors, persistence
    ├── config/                # Defaults, limits, provider/plugin configuration
    ├── i18n/                  # Locale setup and message catalogs
    ├── utils/                 # Existing cross-cutting browser utilities
    ├── __tests__/             # Central Vitest suite
    ├── middleware.ts
    └── types.ts               # Public type-only barrel
```

## Placement Rules

- Put App Router entrypoints in `src/app`. Pages and layouts are Server
  Components by default; add `"use client"` only when the module uses hooks,
  browser APIs, or interactive event handlers. See `src/app/page.tsx` and
  `src/components/app/ServerAuthGate.tsx`.
- Put rendering and interaction in `src/components/<product-area>/`. The
  existing area map is documented in `src/components/README.md`.
- Put small feature-specific stateful composition in `src/features/<feature>`.
  The current example is `src/features/chat/hooks/`, which keeps shell and
  generation orchestration out of `ChatApp.tsx`.
- Put reusable domain transforms and policies in `src/lib/<domain>/`, not in a
  component. Examples include `src/lib/chat/messageTree.ts`,
  `src/lib/security/safeFetch.ts`, and `src/lib/knowledge/citations.ts`.
- Put API contracts and I/O adapters in `src/services/api/`. The facade in
  `src/services/api/client/index.ts` selects local or server implementations;
  do not duplicate that mode switch in UI components.
- Put shared client state in `src/store/core`, selector hooks in
  `src/store/hooks`, and persistence/migrations in `src/store/storage`.
- Put frontend tests in `src/__tests__/`, even when the implementation is
  nested elsewhere. Use `*.test.ts` or `*.test.tsx`.

## Imports and Public Surfaces

- Use the `@/*` alias for imports that cross top-level `src/` areas:

  ```typescript
  import type { Attachment } from "@/types";
  import { useChatStore } from "@/store/core/chatStore";
  ```

- Relative imports are established practice within one module subtree, such as
  `src/services/api/client/server/httpClient.ts` importing `../types`.
- `src/types.ts`, `src/components/index.ts`, and `src/store/index.ts` are
  intentional barrels. Add to them only when the symbol is genuinely shared;
  area-private code should use a direct path.

## Naming

- React component files and exports use PascalCase: `MessageItem.tsx`,
  `KnowledgeSelectionModal.tsx`.
- Hooks start with `use`: `useMessageComposer.ts`, `useStoreWithSSR.ts`.
- Utilities and variables use camelCase. Existing utility filenames contain
  both camelCase and kebab-case; match the neighboring directory rather than
  renaming unrelated files.
- App Router handlers are always `route.ts`; framework files retain Next.js
  names such as `page.tsx`, `layout.tsx`, `loading.tsx`, and `error.tsx`.

## Avoid

- Do not create a second `src/`, `package.json`, `Dockerfile`, or `app/` at the
  repository root.
- Do not put backend authority, secrets, or persistence migrations inside UI
  components.
- Do not add a new generic `helpers/` layer when the logic belongs to an
  existing domain in `lib/`, an API adapter in `services/`, or state in `store/`.
- Do not move files only to enforce a theoretical layout; this bootstrap
  documents the current mixed `lib/utils` and `utils` structure.
