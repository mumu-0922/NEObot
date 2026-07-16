# Frontend Call-Site Inventory

This inventory identifies current frontend-facing call sites that must be considered when introducing the `frontend-api-client` contract. It focuses on direct `fetch`, browser storage, OPFS usage, and service imports that can bypass or shape the future local/server boundary.

## Summary

Current frontend behavior is not one single API boundary yet. Calls are split across:

1. `src/services/api/*` — existing browser-facing service layer. This is the safest starting point for adapter wrapping.
2. React components — several components still call `/api/*` directly.
3. Stores and utility modules — persistent state and OPFS/file resolution are embedded in Zustand stores and helpers.
4. Server/helper registries — a few helper classes use internal API endpoints directly.

Phase 2 migration should keep components stable while moving these calls behind domain clients.

## Priority Legend

| Priority | Meaning                                                 |
| -------- | ------------------------------------------------------- |
| P0       | Must be handled before server chat MVP can work safely. |
| P1       | Needed for first usable server-backed app.              |
| P2       | Defer until related capability migrates.                |
| P3       | Keep local/static or revisit later.                     |

## Direct Component Fetch Calls

These are the highest-risk bypasses because UI components know route details directly.

| Priority | File                                           | Current Call                              | Future Boundary                                     | Notes                                                                                          |
| -------: | ---------------------------------------------- | ----------------------------------------- | --------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
|       P1 | `src/components/app/AccessPasswordPage.tsx`    | `POST /api/access/verify`                 | `authApi.login`                                     | Local adapter can call current route; server adapter maps to `/v1/auth/login`.                 |
|       P0 | `src/components/app/ChatApp.tsx`               | G9.3 retired `GET /api/config`            | `settingsApi.getRuntimeConfig`                      | Server mode now uses Go `/v1/config`; local adapter fails closed.                              |
|       P0 | `src/components/app/ChatApp.tsx`               | G9.3 retired `POST /api/providers/models` | `providerApi.listModels`                            | Server mode now uses Go `/v1/providers/models`; local adapter fails closed.                    |
|       P1 | `src/components/settings/ProviderSettings.tsx` | G9.3 retired `POST /api/providers/models` | `providerApi.listModels`                            | Same model-list contract as ChatApp; local adapter fails closed.                               |
|       P2 | `src/components/settings/DeploymentHealth.tsx` | `GET /api/health`                         | `settingsApi.getRuntimeConfig` or `healthApi` later | Can remain a local/server health widget, not part of chat MVP.                                 |
|       P2 | `src/components/knowledge/KnowledgeBase.tsx`   | `fetch(blobUrl)`                          | `fileApi.getContent` / local OPFS resolver          | This fetches local blob/object URLs, not backend API, but must be separated from server files. |

## Existing Service-Layer Fetch Calls

These are already closer to the desired boundary. Phase 2 should wrap these into local adapters before any component rewrite.

| Priority | Service                               | Current Routes                                                                                                                                     | Future Client                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| -------: | ------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
|       P0 | `src/services/api/chatService.ts`     | `/api/chat`, `/api/chat/generate`, `/api/chat/generate-title`, `/api/chat/related-questions`, `/api/chat/generate-image`, `/api/chat/execute-code` | `chatApi` first; G6.1 blocks server-mode image/code calls, G6.3/G6.4 register fail-closed Go image/code admission routes, G6.5c.1 defines stored image result artifacts, G6.5c.3a adds the opt-in image executor seam, and G6.5c.3b.1 adds OpenAI-compatible image execution while keeping frontend capability disabled until configured-provider smoke passes. G9.2 removed `/api/chat/rag-queries` and query rewrite now falls back to the original prompt. |
|       P1 | `src/services/api/ragService.ts`      | G9.2 retired `/api/rag/query`, `/api/rag/upsert`, `/api/rag/delete`                                                                                | Fail-closed local shim; server Knowledge/RAG uses Go API client.                                                                                                                                                                                                                                                                                                                                                                                              |
|       P2 | `src/services/api/docParseService.ts` | G9.2 retired `/api/doc-parse`, `/api/doc-parse/jobs/:id`                                                                                           | Fail-closed local shim; server document upload/indexing uses Go API client.                                                                                                                                                                                                                                                                                                                                                                                   |
|       P2 | `src/services/api/pluginService.ts`   | G9.4 retired `/api/plugins/list` and `/api/plugins/install`; local adapter fails closed                                                            | Server adapter targets `/v1/plugins*`; G4.5c.2b persists supplied plugin payloads in Go/Postgres when `DATABASE_URL` is configured and converts custom/OpenAPI manifest installs in Go.                                                                                                                                                                                                                                                                       |
|       P2 | `src/utils/pluginUtils.ts`            | G9.4 retired `/api/plugins/execute`; local adapter fails closed                                                                                    | Server adapter targets `/v1/plugins/execute`; G4.5c.2c sends id-only payloads, Go resolves built-ins/registered/custom OpenAPI plugins from memory/Postgres, and Go normalizes built-in plugin result envelopes.                                                                                                                                                                                                                                              |
|       P2 | `src/services/api/searchService.ts`   | `/api/search`                                                                                                                                      | `searchApi` or chat-side capability later.                                                                                                                                                                                                                                                                                                                                                                                                                    |
|       P2 | `src/services/api/voiceService.ts`    | `/api/voice/transcribe`, `/api/voice/synthesize`                                                                                                   | `voiceApi` later; G6.1 blocks server-mode service calls, G6.2 registers fail-closed Go `/v1/voice/*` admission routes, G6.5c.1 defines stored audio result artifacts, and G6.5c.2a adds the opt-in executor seam without enabling live providers.                                                                                                                                                                                                             |
|       P3 | `src/services/api/agentService.ts`    | G9.4 retired `/api/agents` and `/api/agents/:identifier`; local adapter fails closed                                                               | Server adapter targets `/v1/agents*`; Go owns the localized public catalog surface.                                                                                                                                                                                                                                                                                                                                                                           |
|       P3 | `src/services/api/skillService.ts`    | `/data/skills/*`                                                                                                                                   | static asset loader; no Go dependency for MVP.                                                                                                                                                                                                                                                                                                                                                                                                                |

## Store and Utility Fetch Calls

These bypass the service layer or belong to infra helpers. They must be classified before server mode.

| Priority | File                                                   | Current Call                            | Future Boundary                                          | Notes                                                                      |
| -------: | ------------------------------------------------------ | --------------------------------------- | -------------------------------------------------------- | -------------------------------------------------------------------------- |
|       P0 | `src/lib/byok/client.ts`                               | G9.3 retired `GET /api/byok/public-key` | `byokApi.getPublicKey`                                   | Server mode now uses Go `/v1/byok/public-key`; local adapter fails closed. |
|       P1 | `src/store/core/settingsStore.ts`                      | provider model fetch                    | `providerApi.listModels`                                 | Store should not know provider-model route shape.                          |
|       P1 | `src/lib/data/clearAppData.ts`                         | G9.2 retired `POST /api/rag/delete`     | Local reset only; server Knowledge deletion uses Go APIs | Server mode must distinguish local reset from server data deletion.        |
|       P2 | `src/lib/plugin/serverRegistry.ts`                     | server registry endpoint fetches        | `pluginApi` / backend registry later                     | Keep out of first MVP.                                                     |
|       P2 | `src/lib/api/docParseJobs.ts`                          | no active route caller after G9.2       | delete with remaining local authority cleanup            | Former Next document job helper is now dead code.                          |
|       P2 | `src/lib/security/rateLimitStore.ts`                   | rate-limit endpoint fetches             | backend Redis/rate-limit integration                     | Server-side rate limits should be authoritative.                           |
|       P3 | `src/lib/security/safeFetch.ts`                        | outbound safe fetch                     | backend-only helper                                      | Not a frontend API client target.                                          |
|       P3 | `src/lib/utils/attachments.ts`, `src/lib/utils/rag.ts` | `fetch(blobUrl)`                        | local file resolver                                      | Blob/object URL fetches stay local; do not route to Go.                    |
|       P3 | `src/store/core/knowledgeStore.ts`                     | `fetch(objectUrl)`                      | local file resolver                                      | OPFS/blob content conversion, not backend route.                           |

## Browser Storage and OPFS Call Sites

These define the local adapter and import boundary.

### Persistent Store Roots

| Priority | File                                         | Current Storage                                           | Future Boundary                                          |
| -------: | -------------------------------------------- | --------------------------------------------------------- | -------------------------------------------------------- |
|       P0 | `src/store/storage/storageConfig.ts`         | `localforage` app DB + `window.localStorage`              | G9.5a makes Zustand storage no-op in server mode; explicit import may still direct-read. |
|       P0 | `src/store/core/chatStore.ts`                | `getAppDbStorage`, `deleteFromOPFS`                       | Persist adapter, OPFS delete, and direct chat-message IndexedDB writes are fenced in server mode. |
|       P1 | `src/store/core/coreSettingsStore.ts`        | `getBrowserLocalStorage`                                  | Persist adapter is fenced in server mode; server settings must come from Go.             |
|       P1 | `src/store/core/settingsStore.ts`            | `getAppDbStorage`                                         | Persist adapter is fenced in server mode; no direct IndexedDB writes remain here.                   |
|       P1 | `src/store/core/knowledgeStore.ts`           | `getAppDbStorage`, OPFS helpers                           | Persist adapter and OPFS write/delete helpers are fenced in server mode.                            |
|       P2 | `src/store/core/memoryStore.ts`              | `getAppDbStorage`                                         | local-only until memory/server strategy is designed.     |
|       P2 | `src/store/storage/legacyGeminiMigration.ts` | legacy localStorage/localforage migration                 | Must be preserved for local import/export compatibility. |
|       P2 | `src/app/layout.tsx`                         | reads `neo-chat-core-settings` from `window.localStorage` | theme/bootstrap compatibility                            | Keep minimal inline bootstrap; do not expand server coupling here. |

### OPFS Consumers

| Priority | File                                                 | Current OPFS Use                                     | Future Boundary                                          |
| -------: | ---------------------------------------------------- | ---------------------------------------------------- | -------------------------------------------------------- |
|       P0 | `src/utils/opfs.ts`                                  | source helper for `opfs://` save/resolve/delete/list | Local `fileApi` implementation.                          |
|       P0 | `src/components/chat/MessageAttachmentView.tsx`      | resolves OPFS URLs for display                       | `fileApi.getObjectUrl` later.                            |
|       P0 | `src/components/chat/MessageInputAttachmentTray.tsx` | resolves OPFS URLs for attachment tray               | `fileApi.getObjectUrl` later.                            |
|       P1 | `src/components/media/ImagePreview.tsx`              | resolves OPFS images                                 | `fileApi.getObjectUrl` later.                            |
|       P1 | `src/components/content/MarkdownRendererClient.tsx`  | resolves OPFS assets in rendered content             | local file resolver / `fileApi`.                         |
|       P1 | `src/components/layout/WorkspaceSettingsModal.tsx`   | `saveToOPFS`, `deleteFromOPFS` workspace files       | `fileApi.upload/delete` local adapter first.             |
|       P1 | `src/components/knowledge/KnowledgeBase.tsx`         | resolves OPFS knowledge files                        | `fileApi.getObjectUrl`; server mode later uses `fileId`. |
|       P1 | `src/lib/data/appExport.ts`                          | collects `opfs://` references                        | Import/export contract later.                            |
|       P1 | `src/lib/data/clearAppData.ts`                       | deletes OPFS directories                             | local reset API; server reset must be explicit.          |
|       P2 | `src/lib/chat/messageProcessor.ts`                   | resolves OPFS attachments for model input            | `fileApi` + attachment normalization.                    |
|       P2 | `src/lib/utils/rag.ts`                               | resolves OPFS files for RAG                          | RAG sidecar import path later.                           |
|       P2 | `src/lib/utils/documentAttachments.ts`               | parses uploaded files                                | document API later.                                      |

## Service Imports in UI/Domain Code

These imports show where component behavior will feel the API boundary change.

| Priority | Import Consumers                                               | Imported Service                                | Migration Note                                                                     |
| -------: | -------------------------------------------------------------- | ----------------------------------------------- | ---------------------------------------------------------------------------------- |
|       P0 | `src/components/app/ChatApp.tsx`, chat hooks/stores indirectly | `chatService`, skill/agent services             | Main chat spine must wrap or replace `chatService` first.                          |
|       P0 | `src/components/chat/MessageInput.tsx`, `MessageItem.tsx`      | `chatService`, `voiceService`, artifact service | Keep UI stable; move backend route awareness into clients.                         |
|       P1 | `src/components/settings/MemorySettings.tsx`                   | memory dream chat helper                        | Defer server memory; keep local capability gated.                                  |
|       P1 | `src/store/core/knowledgeStore.ts`                             | fail-closed `docParseService`, `ragService`     | Server Knowledge/RAG uses Go UI/client; local authority cleanup continues in G9.5. |
|       P2 | `src/components/plugin/PluginMarket.tsx`                       | `pluginService`                                 | Server mode goes through Go `pluginApi`; local plugin routes are gone and fail closed. |
|       P2 | `src/components/assistant/*`                                   | `agentService`, `chatService`, artifact service | Agent catalog goes through Go `agentApi`; local agent routes are gone and fail closed. |
|       P2 | `src/components/content/*`                                     | `chatService`, artifact service                 | Artifact/code execution helpers are not MVP chat spine.                            |
|       P3 | `src/components/skill/SkillMarket.tsx`                         | `skillService`                                  | Static assets; not server MVP.                                                     |

## Phase 2 Migration Order

Recommended implementation order after this inventory:

1. Add `createNeoChatApiClient` factory and shared contract types.
2. Wrap current `chatService` as `local.chatApi` without changing behavior.
3. Move direct config/model fetches in `ChatApp` and `ProviderSettings` behind `settingsApi` and `providerApi`.
4. Wrap OPFS helpers as `local.fileApi` and replace direct display resolvers gradually.
5. Add `server` mode HTTP adapter for `/v1/config`, `/health`, and provider model listing smoke tests.
6. Add server chat CRUD/SSE only after the local adapter passes parity tests.
7. Keep RAG/doc parse, plugin/agent, voice, image generation, and code execution behind typed API-client capability gates; G9.2-G9.4 removed their replaced local Next route fallbacks where Go ownership exists, while G6.1+ keeps voice/image/code execution opt-in and fail-closed until each provider path passes its own gate.

## Risks and Guardrails

- Do not make components choose between local/server routes directly.
- Do not send `opfs://` URLs to Go except inside an explicit import payload.
- Do not expose MinIO object keys or direct object-store URLs to components.
- Do not reintroduce plugin or agent Next routes; server mode must use Go `/v1/*` adapters and local mode must fail closed.
- Do not remove legacy local storage migrations before import/export compatibility is proven.
- Treat `NEXT_PUBLIC_API_MODE` as default/fallback only; runtime rollback should come from config endpoints when possible.

## Completion Criteria

This inventory completes the Phase 2 progress item “Identify components that directly call storage or fetch.” It should be revisited before actual code migration because call sites may change as the upstream app evolves.
