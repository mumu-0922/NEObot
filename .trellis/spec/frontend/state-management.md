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

### Refresh-Restored Browser Preferences

- A preference that must survive refresh in both Local and Server mode (for
  example the composer's complete `providerId:modelId` selection) belongs in a
  store backed by `getBrowserPreferenceStorage()`. Do not rely on
  `chatStore`/`getAppDbStorage()` for that contract: Server mode intentionally
  turns the chat IndexedDB adapter into a no-op because chat data is
  server-authoritative.
- Keep the persisted preference and live runtime projection separate when the
  latter already has an owning store. Restore only after both preference and
  Provider/model catalogs hydrate, require an exact available-model match, and
  retain the saved value while the catalog is temporarily empty. If a
  non-empty catalog proves it unavailable, use the existing default fallback
  and persist that resolved choice.
- Regression tests must assert that `partialize` retains the full Provider and
  model identity and that the browser-preference storage adapter receives the
  write in Server mode.

## Scenario: Conversation-owned model selection

### 1. Scope / Trigger

Apply this contract when changing the model selector, Local Session metadata,
Server Conversation DTO mapping, or model bootstrap behavior.

### 2. Signatures

```ts
interface Session { model: string }
updateSessionModel(id: string, model: string): void
updateServerSessionModel(id: string, model: string): Promise<boolean>
PATCH /v1/chat/conversations/{id} { modelRef: { providerId, modelId } }
```

### 3. Contracts

- `Session.model` / Server `Conversation.modelRef` owns the selected model for
  that Conversation. `chatStore.selectedModel` is only the active UI/runtime
  projection; `coreSettingsStore.selectedChatModel` is only the default for a
  newly-created Conversation.
- Selecting a Local or Server Conversation restores its complete
  `providerId:modelId`. Switching Conversations must not rewrite the browser
  default or another Conversation's stored model.
- Server model writes for the same Conversation are serialized in selection
  order. A completion for a non-active Conversation may update its cached
  metadata but must not replace the active runtime model.
- Backend Provider `openai_compatible` maps to frontend `SERVER_DEFAULT` only
  at the Conversation boundary so exact catalog matching survives reload;
  message model projections retain their backend identity.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Local Session has a stored available model | restore it immediately on selection |
| Server Conversation update succeeds | replace only that Conversation and active projection when still selected |
| Server update fails | keep the previous model and show a bounded UI error |
| Stored model is absent | use the new-Conversation browser default |
| Stored model is unavailable in a non-empty catalog | show the normal safe fallback without overwriting stored Conversation metadata |
| Rapid updates A then B | persist A then B; final durable and active value is B |

### 5. Good / Base / Bad Cases

- Good: Conversation A uses model A, Conversation B uses model B, and both
  retain those values across switching and refresh.
- Base: a legacy Conversation without a model uses the current browser default.
- Bad: every model click only calls `setModel` and changes all Conversations,
  or catalog bootstrap persists a fallback into Conversation metadata.

### 6. Tests Required

- Local store test: update, switch between two Sessions, and assert persisted
  `Session.model` isolation.
- Server store test: assert PATCH `modelRef`, Conversation isolation, ordered
  rapid writes, selection restore, and unchanged state on rejection.
- DTO test: assert `openai_compatible` Conversation alias normalization without
  changing generic message model projection.
- ChatApp composition plus frontend format, lint, type-check, full Vitest, and
  production build.

### 7. Wrong vs Correct

#### Wrong

```ts
const onSelectModel = (model: string) => {
  setModel(model);
  setSelectedChatModel(model);
};
```

#### Correct

```ts
const onSelectModel = (conversationId: string, model: string) =>
  serverMode
    ? updateServerSessionModel(conversationId, model)
    : updateSessionModel(conversationId, model);
```

### Refresh-Restored Nested Views

- A nested screen users expect to bookmark, refresh, or traverse with browser
  Back/Forward belongs in URL state, not component-only `useState`. Extend the
  existing pure parse/update helper and keep the top-level shell responsible
  for `pushState`, `replaceState`, and `popstate` synchronization.
- Treat identifiers parsed from the URL as untrusted. Validate their shape,
  match them against a server-returned entity, and only then issue detail or
  child-resource requests. An invalid, inaccessible, or deleted identifier
  must be removed with `replaceState` and resolve to the owning list view.
- Use `pushState` for user navigation between list and detail. Use
  `replaceState` for normalization/deletion so Back does not reopen a broken
  entry. Tests must cover round-trip serialization, unrelated-query
  preservation, invalid/out-of-panel cleanup, and the server-match request
  gate.

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

## Scenario: Durable Assistant total duration

### 1. Scope / Trigger

Apply when changing Chat/Agent run lifecycle, server Message DTO projection,
Local generation cancellation, or Assistant Footer metadata.

### 2. Signatures

```text
ChatMessageDTO.createdAt: ISO timestamp (required)
ChatMessageDTO.completedAt?: ISO timestamp
Message.timing?: { startTime: number; endTime: number; duration: number }
normalizeServerMessageTiming(startTime, completedAt) -> timing | undefined
```

### 3. Contracts

- Server Assistant `createdAt` is the durable run-start boundary and
  `completedAt` is the terminal boundary. Their difference covers the durable
  Retrieval/Reasoning/Model/Tool/approval wait after the Assistant Run exists.
- DTO mapping creates timing only for Assistant messages with a valid terminal
  timestamp. `toStoreMessageFromServer` must preserve it so live terminal
  results, cache snapshots, Conversation switching, and reload agree.
- Local Chat starts timing before prompt preprocessing, Memory/RAG, Search, or
  Provider streaming. Success, failure, and explicit Stop set one terminal
  timestamp and persist it with the Message.
- Components humanize the stored duration but never recalculate it from the
  render clock. Desktop shows `totalDuration`; mobile uses the compact metadata
  tooltip.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| missing `completedAt` | omit final timing while the Run is active |
| invalid or earlier terminal timestamp | omit timing; never render negative/NaN duration |
| duration below one second | render the localized less-than-one-second label |
| Local explicit Stop after a draft exists | finalize and persist timing before stopped-message sync |
| Server cancel/failure returns terminal Message | preserve its timing through DTO -> Store -> Footer |

### 5. Good / Base / Bad Cases

- **Good**: a Tool-backed Agent waits for approval, completes, displays one
  human-readable total, and shows the same total after refresh.
- **Base**: a streaming Assistant has no final duration until it reaches a
  terminal state.
- **Bad**: the Footer uses `Date.now()` on render, Local timing begins after
  RAG, or DTO timing disappears in the Store mapper.

### 6. Tests Required

- duration boundary/invalid-value unit tests;
- DTO `createdAt`/`completedAt` projection tests;
- DTO -> Store durable timing regression test;
- ChatApp cancellation/start-boundary and MessageItem/i18n composition tests.

### 7. Wrong vs Correct

```text
Wrong: duration = renderNow - message.timestamp
Correct: terminal lifecycle writes timing once -> persistence -> pure formatting

Wrong: map completedAt in the API adapter but drop timing in chatStore
Correct: preserve timing across every DTO -> Store -> Message projection
```

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
