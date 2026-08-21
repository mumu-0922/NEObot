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
- Request identity gates presentation, not accepted Server work. Once
  `appendUserMessage` succeeds, `sendServerMessageAndStream` must dispatch the
  stream even if navigation has superseded the current read request. Its stale
  deltas/terminal result may be ignored; PostgreSQL remains authoritative and
  selecting/reloading that Conversation hydrates the completed assistant.
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

## Message Version Navigation

`SessionMessageTree` uses sibling nodes for both edited User prompts and
regenerated Assistant answers, but their navigation contracts differ:

- User sibling selection is branch selection. Use `switchMessageBranch`; the
  selected prompt's own continuation becomes visible.
- Assistant sibling selection is answer-slot selection. Use
  `switchMessageVersionInTree`; it swaps the current and target nodes'
  descendant attachments, repairs direct-child `parentMessageId` links, and
  then keeps the already visible downstream message IDs in the same order.
- Assistant sibling creation is also answer-slot selection. Use
  `createModelResponseBranch`; the new active sibling must inherit the source
  node's descendants immediately, before an empty regeneration draft renders.
  The source becomes childless and moved direct children point to the new node.
- Both `switchMessageVersion` and `switchServerMessageVersion` in
  `chatStore.ts` must use the answer-slot operation. Do not fix only the Local
  or Server projection.
- Server regeneration must pass its source Assistant ID through the draft and
  terminal insertion paths so a first `message.started` event and a terminal-
  only response both use `createModelResponseBranch`. Ordinary new Assistant
  messages still use `appendServerMessageToTree`.
- Server reload must reconstruct the same invariant from chronological durable
  messages. When `appendServerMessageToTree` sees another Model child under a
  User parent whose active child is already a Model, it must route that newer
  sibling through `createModelResponseBranch`; otherwise a refresh reattaches
  the continuation to the old answer and hides it below the newest version.

This distinction prevents `1/N` or `2/N` answer navigation from making later
messages disappear while preserving the tree's single-parent invariant and
every inactive subtree. Regression tests must assert the downstream ID list,
the repaired direct-child parent, Local persistence, Server cache isolation,
unchanged User-branch behavior, and the state observed synchronously after the
Server `message.started` callback. Testing navigation alone is insufficient:
every creation and selection entry point must satisfy the same slot-local
contract, including a clean Server reload from persisted messages.

## Avoid

- Duplicating server state in component state and Zustand without a defined
  synchronization path.
- Writing browser persistence directly from arbitrary components.
- Persisting secrets or server-authoritative chat/file/knowledge data in server
  mode.
- Broad store subscriptions that rerender on unrelated updates.
- In-place array/object mutation or updates based on an old async closure.
- Returning a synthetic cancelled result before dispatching an accepted Server
  message merely because another Conversation became the active read snapshot.
