# State Management

> State is split by ownership: transient component state, shared Zustand state,
> URL state, browser persistence, and server-authoritative data.

## State Categories

| Category                | Established location                          | Examples                                                                                     |
| ----------------------- | --------------------------------------------- | -------------------------------------------------------------------------------------------- |
| Transient UI/input      | Component or feature hook `useState`/`useRef` | Composer text and attachments in `useMessageComposer.ts`; portal visibility in `Tooltip.tsx` |
| Shared client state     | Zustand stores in `src/store/core/`           | Chat, settings, core preferences, memory, image preview                                      |
| Persisted browser state | Zustand middleware plus `src/store/storage/`  | IndexedDB through LocalForage; preference `localStorage`                                     |
| URL state               | Pure parse/update helpers                     | `src/lib/chat/panelUrlState.ts`                                                              |
| Server-owned state      | Typed API client + store/service actions      | `chatStore.ts` server sessions/messages; `src/services/api/client/server/`                   |
| Static configuration    | `src/config/` constants/defaults              | Limits, assistants, providers, default chat settings                                         |

## Zustand Conventions

- Each core store owns a cohesive domain and exports a `use*Store` hook.
- State and actions are typed in the store module. Updates use Zustand `set`
  with immutable object/array replacement; callers invoke actions rather than
  mutating returned state.
- Components subscribe with narrow selectors. When returning multiple values,
  use `useShallow`; reusable selector bundles live in
  `src/store/hooks/useShallowStore.ts`.
- Imperative workflows may call `useChatStore.getState()` for the latest
  snapshot, as `useChatGenerationController.ts` does when stopping generation.
- Derived state is calculated or normalized rather than stored twice. The chat
  store derives the active message path from `SessionMessageTree` helpers in
  `src/lib/chat/messageTree.ts`.

```typescript
export const useUIStore = create<UIState>((set) => ({
  imagePreview: { isOpen: false, images: [], currentIndex: 0 },
  closeImagePreview: () =>
    set((state) => ({
      imagePreview: { ...state.imagePreview, isOpen: false },
    })),
}));
```

## Persistence and Authority

- `src/store/storage/storageConfig.ts` is the persistence authority. Larger
  application data uses the `neo-chat` LocalForage/IndexedDB instance; core
  browser preferences use `localStorage`.
- Persisted stores have explicit storage versions and migrations. Untrusted old
  data is normalized before entering current state; see
  `src/store/storage/migrations.ts` and `legacyGeminiMigration.ts`.
- In `NEXT_PUBLIC_API_MODE=server`, chat/files/knowledge authority belongs to
  Go/Postgres/MinIO. Browser IndexedDB becomes import-only, while browser-owned
  preferences such as theme/language remain local.
- SSR uses no-op storage and hydration-aware hooks. Never read persisted browser
  data as if it were available during the server render.

## Server State

- There is no separate query-cache library. `createNeoChatApiClient()` selects
  local or server API shells once, and services/stores own synchronization.
- Store async state includes loading/error/generation state where the UI needs
  it. Request IDs and serialized write queues in `chatStore.ts` prevent stale
  reads or writes from replacing newer snapshots.
- Errors must reach a typed error or explicit error field; do not silently
  convert a failed server operation into successful local state.

## When to Promote State

Keep state local until at least one of these is true:

- multiple distant components need the same value/action;
- the value must survive component unmount/navigation;
- the value participates in persistence or server synchronization;
- an imperative workflow needs a single current snapshot.

Promote only into the existing owning store. A modal toggle used by one
component should remain local; the global image preview belongs in `uiStore`
because many content components can open it.

## Avoid

- Duplicating server state in component state and Zustand without a defined
  synchronization path.
- Writing browser persistence directly from arbitrary components.
- Persisting secrets or server-authoritative chat/file/knowledge data in server
  mode.
- Broad store subscriptions that rerender on unrelated updates.
- In-place array/object mutation or updates based on an old async closure.
