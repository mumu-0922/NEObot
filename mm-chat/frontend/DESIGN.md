# Standalone Frontend Consolidation Design

## 1. Scope / Trigger

The final project root is `mm-chat/`, not the original repository root. The
existing Next.js interface is relocated here unchanged and then its server
dependencies are migrated capability by capability to Go. This is a runtime
ownership change, not a visual redesign.

## 2. Signatures

Build and runtime entrypoints:

```text
corepack pnpm install --frozen-lockfile
corepack pnpm dev
corepack pnpm typecheck
corepack pnpm lint
corepack pnpm test
corepack pnpm build
```

Existing server adapter surface:

```ts
createNeoChatApiClient(config?: ApiClientConfig): NeoChatApiClient;
```

The browser uses `/mm-api/v1/*`; the same-origin edge forwards only that prefix
to the private Go backend.

## 3. Contracts

- `NEXT_PUBLIC_API_MODE=server` selects the transition server path.
- `NEXT_PUBLIC_API_BASE_URL=/mm-api` keeps browser requests same-origin.
- `MM_CHAT_BACKEND_INTERNAL_URL` identifies the private Go destination for the
  frontend/proxy runtime and must never be exposed as a browser credential.
- Go remains authoritative for Auth, Chat, Files, Import, Teams, Knowledge, and
  all later migrated provider/plugin/RAG operations.
- Python RAG is private behind Go; the browser never calls it directly.
- Existing theme tokens, components, routes, localization, responsiveness, and
  accessibility behavior form the UI compatibility contract.
- No final build, test, runtime, fixture, or deployment path may traverse from
  `mm-chat/` back into the original root project.

## 4. Validation & Error Matrix

| Condition                                         | Required behavior                                                                    |
| ------------------------------------------------- | ------------------------------------------------------------------------------------ |
| Server API unavailable                            | Surface the existing recoverable server error; never write silently to local storage |
| Unsupported migrated capability                   | Fail closed and keep its UI disabled until the Go contract exists                    |
| Browser receives object-store/private RAG address | Reject as a contract violation                                                       |
| Server response shape is invalid                  | Normalize to the typed API-client error boundary                                     |
| Legacy `/api/*` remains at final gate             | Standalone cutover fails                                                             |
| Import/build/runtime reads original root          | Clean-copy gate fails                                                                |
| Visual baseline changes during relocation         | Visual/interaction parity gate fails                                                 |

## 5. Good / Base / Bad Cases

- **Good:** the existing Chat UI calls `/mm-api/v1/chat/*`, reloads Go-owned
  state, renders the same components, and performs no IndexedDB fallback.
- **Base:** a capability not yet migrated remains visibly unavailable in
  server mode while its legacy handler stays isolated for transition testing.
- **Bad:** a failed Go call silently invokes `/api/*`, localforage, OPFS, an
  external provider, MinIO, or the Python RAG service from browser code.

## 6. Tests Required

- Relocation slice: install, typecheck, and production build from
  `mm-chat/frontend/`; run server adapter/composition tests only.
- Domain cutover: run the changed adapter/handler/store tests and one browser
  smoke for that domain.
- UI preservation: capture agreed desktop/mobile visual baselines and critical
  interaction smoke paths.
- Final closure: run the entire frontend suite once, then clean-copy Compose,
  security, backup/restore, and root-reference scans.

## 7. Wrong vs Correct

### Wrong

```ts
try {
  return await serverClient.send(input);
} catch {
  return localBrowserStore.send(input);
}
```

This hides server failure, creates split authority, and prevents safe deletion
of the original local-first path.

### Correct

```ts
return serverClient.send(input);
```

The typed server boundary reports failure visibly. Browser-local data moves only
through the explicit import flow.

## Migration Boundaries

1. Relocate the complete current frontend without UI changes.
2. Add the persistent same-origin edge and Compose frontend service.
3. Wire existing Go capabilities not yet exposed by the frontend.
4. Replace the remaining legacy Next.js handlers one domain at a time.
5. Remove local-mode authority after parity and import verification.
6. Pass the clean-copy gate before preparing original-root deletion.
