# Hook Guidelines

> Hooks package reusable client-side state and lifecycles; pure transforms stay
> as ordinary functions so they can run on the server and in fast unit tests.

## Location and Naming

- Custom hooks always start with `use` and use camelCase filenames.
- Feature orchestration hooks live with the feature. Current chat examples are
  under `src/features/chat/hooks/`.
- Reusable store/SSR hooks live under `src/store/hooks/`; locale-specific hooks
  may live with `src/i18n/`.
- Files that call React hooks or browser APIs include `"use client"` when they
  can be reached from the App Router client boundary.

## Established Patterns

### Local state hook

Use typed options with defaults and return a named object. Keep reset behavior
stable with `useCallback`, as in `useMessageComposer.ts`:

```typescript
export function useMessageComposer({
  initialText = "",
  initialAttachments = [],
}: UseMessageComposerOptions = {}) {
  const [text, setText] = useState(initialText);
  const reset = useCallback(() => setText(initialText), [initialText]);
  return { text, setText, reset };
}
```

### Store selector hook

Subscribe only to fields/actions required by the caller and wrap object
selectors in Zustand `useShallow`. See `useSidebarSessions.ts` and
`store/hooks/useShallowStore.ts`:

```typescript
return useChatStore(
  useShallow((state) => ({
    sessions: state.sessions,
    selectSession: state.selectSession,
  })),
);
```

### Effects

- Put every reactive input in the dependency list.
- Remove event listeners, cancel animation frames/timers, and abort owned work
  in cleanup. `useChatThemeEffects.ts` removes its media-query listener;
  `AnchoredPortal.tsx` removes scroll/resize listeners.
- Do not mirror a value into state when it can be calculated during render.
- Use `useRef` for mutable lifecycle identity that should not trigger renders.
  `useChatGenerationController.ts` stores the active `AbortController` and a
  monotonically increasing run ID to reject stale completion callbacks.

## Data Fetching and Async Work

- The project does not use React Query or SWR. Durable server data flows through
  the typed API client in `src/services/api/client/`, service modules, and
  Zustand actions such as the server-read actions in `chatStore.ts`.
- Feature hooks orchestrate those services/stores; they should not create a
  second cache. Small same-origin screen actions currently use direct `fetch`
  in a few components, so follow the owning module rather than inventing a new
  global fetch abstraction.
- Pass `AbortSignal` through service calls when supported. Guard async results
  with request/run identity when a newer request may supersede an older one.
- In Server mode, an accepted chat generation is backend-owned. Conversation,
  new-chat, and assistant-preset navigation must not abort its controller;
  only the explicit Stop action (or deleting the owning active Conversation)
  may abort it and trigger `POST /v1/chat/runs/{runId}/cancel`. Page close is
  handled by detached backend execution, not a React cleanup cancellation.
- For one-off imperative reads inside callbacks, established code uses
  `useChatStore.getState()`; normal render data still uses selector hooks.

## SSR and Hydration

- Use `useStoreWithSSR`, `useHydratedStore`, or an explicit `_hasHydrated`
  selector when persisted values would differ between server and browser.
- Do not implement client detection with an eager `window` read during render.
  `store/hooks/useStoreWithSSR.ts` uses `useSyncExternalStore` and a server
  fallback.

## Common Mistakes

- Returning a new broad store object without `useShallow`, causing avoidable
  renders.
- Omitting effect cleanup or dependency inputs.
- Letting an aborted/stale generation update current UI state.
- Treating an SSE subscription as ownership of the Server Run and aborting it
  during navigation instead of only changing the visible Conversation.
- Hiding domain validation inside a hook instead of a reusable normalizer or
  schema.
- Calling a custom hook conditionally.
