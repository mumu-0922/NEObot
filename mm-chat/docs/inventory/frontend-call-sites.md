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

| Priority | Meaning |
|---|---|
| P0 | Must be handled before server chat MVP can work safely. |
| P1 | Needed for first usable server-backed app. |
| P2 | Defer until related capability migrates. |
| P3 | Keep local/static or revisit later. |

## Direct Component Fetch Calls

These are the highest-risk bypasses because UI components know route details directly.

| Priority | File | Current Call | Future Boundary | Notes |
|---:|---|---|---|---|
| P1 | `src/components/app/AccessPasswordPage.tsx` | `POST /api/access/verify` | `authApi.login` | Local adapter can call current route; server adapter maps to `/v1/auth/login`. |
| P0 | `src/components/app/ChatApp.tsx` | `GET /api/config` | `settingsApi.getRuntimeConfig` | Runtime rollback depends on this becoming the mode/capability bootstrap. |
| P0 | `src/components/app/ChatApp.tsx` | `POST /api/providers/models` | `providerApi.listModels` | Provider/model identity must return `providerId + modelId`. |
| P1 | `src/components/settings/ProviderSettings.tsx` | `POST /api/providers/models` | `providerApi.listModels` | Same model-list contract as ChatApp; avoid duplicate parsing. |
| P2 | `src/components/settings/DeploymentHealth.tsx` | `GET /api/health` | `settingsApi.getRuntimeConfig` or `healthApi` later | Can remain a local/server health widget, not part of chat MVP. |
| P2 | `src/components/knowledge/KnowledgeBase.tsx` | `fetch(blobUrl)` | `fileApi.getContent` / local OPFS resolver | This fetches local blob/object URLs, not backend API, but must be separated from server files. |

## Existing Service-Layer Fetch Calls

These are already closer to the desired boundary. Phase 2 should wrap these into local adapters before any component rewrite.

| Priority | Service | Current Routes | Future Client |
|---:|---|---|---|
| P0 | `src/services/api/chatService.ts` | `/api/chat`, `/api/chat/generate`, `/api/chat/generate-title`, `/api/chat/related-questions`, `/api/chat/rag-queries`, `/api/chat/generate-image`, `/api/chat/execute-code` | `chatApi` first; defer image/code/tool helpers behind capabilities. |
| P1 | `src/services/api/ragService.ts` | `/api/rag/query`, `/api/rag/upsert`, `/api/rag/delete` | `ragApi` later; not first chat MVP. |
| P2 | `src/services/api/docParseService.ts` | `/api/doc-parse`, `/api/doc-parse/jobs/:id` | `documentApi` / RAG sidecar later. |
| P2 | `src/services/api/pluginService.ts` | `/api/plugins/list`, `/api/plugins/install` | `pluginApi`, capability-gated. |
| P2 | `src/utils/pluginUtils.ts` | `/api/plugins/execute` | `pluginApi.execute`, disabled until sandbox design. |
| P2 | `src/services/api/searchService.ts` | `/api/search` | `searchApi` or chat-side capability later. |
| P2 | `src/services/api/voiceService.ts` | `/api/voice/transcribe`, `/api/voice/synthesize` | `voiceApi` later. |
| P3 | `src/services/api/agentService.ts` | `/api/agents`, `/api/agents/:identifier` | static/catalog API; can remain static initially. |
| P3 | `src/services/api/skillService.ts` | `/data/skills/*` | static asset loader; no Go dependency for MVP. |

## Store and Utility Fetch Calls

These bypass the service layer or belong to infra helpers. They must be classified before server mode.

| Priority | File | Current Call | Future Boundary | Notes |
|---:|---|---|---|---|
| P0 | `src/lib/byok/client.ts` | `GET /api/byok/public-key` | `providerApi` / `authApi` secret bootstrap | Server mode should not expose plaintext provider secrets. |
| P1 | `src/store/core/settingsStore.ts` | provider model fetch | `providerApi.listModels` | Store should not know provider-model route shape. |
| P1 | `src/lib/data/clearAppData.ts` | `POST /api/rag/delete` | `ragApi.delete` or import/reset API later | Server mode must distinguish local reset from server data deletion. |
| P2 | `src/lib/plugin/serverRegistry.ts` | server registry endpoint fetches | `pluginApi` / backend registry later | Keep out of first MVP. |
| P2 | `src/lib/api/docParseJobs.ts` | document job endpoint fetches | `documentApi` later | Defer with RAG/doc parse. |
| P2 | `src/lib/security/rateLimitStore.ts` | rate-limit endpoint fetches | backend Redis/rate-limit integration | Server-side rate limits should be authoritative. |
| P3 | `src/lib/security/safeFetch.ts` | outbound safe fetch | backend-only helper | Not a frontend API client target. |
| P3 | `src/lib/utils/attachments.ts`, `src/lib/utils/rag.ts` | `fetch(blobUrl)` | local file resolver | Blob/object URL fetches stay local; do not route to Go. |
| P3 | `src/store/core/knowledgeStore.ts` | `fetch(objectUrl)` | local file resolver | OPFS/blob content conversion, not backend route. |

## Browser Storage and OPFS Call Sites

These define the local adapter and import boundary.

### Persistent Store Roots

| Priority | File | Current Storage | Future Boundary |
|---:|---|---|---|
| P0 | `src/store/storage/storageConfig.ts` | `localforage` app DB + `window.localStorage` | Local adapter source of truth; import/export source. |
| P0 | `src/store/core/chatStore.ts` | `getAppDbStorage`, `deleteFromOPFS` | `chatApi` local adapter + `fileApi` local adapter. |
| P1 | `src/store/core/coreSettingsStore.ts` | `getBrowserLocalStorage` | `settingsApi` local adapter. |
| P1 | `src/store/core/settingsStore.ts` | `getAppDbStorage` | `settingsApi` local adapter. |
| P1 | `src/store/core/knowledgeStore.ts` | `getAppDbStorage`, OPFS helpers | `fileApi` local adapter + later RAG import. |
| P2 | `src/store/core/memoryStore.ts` | `getAppDbStorage` | local-only until memory/server strategy is designed. |
| P2 | `src/store/storage/legacyGeminiMigration.ts` | legacy localStorage/localforage migration | Must be preserved for local import/export compatibility. |
| P2 | `src/app/layout.tsx` | reads `neo-chat-core-settings` from `window.localStorage` | theme/bootstrap compatibility | Keep minimal inline bootstrap; do not expand server coupling here. |

### OPFS Consumers

| Priority | File | Current OPFS Use | Future Boundary |
|---:|---|---|---|
| P0 | `src/utils/opfs.ts` | source helper for `opfs://` save/resolve/delete/list | Local `fileApi` implementation. |
| P0 | `src/components/chat/MessageAttachmentView.tsx` | resolves OPFS URLs for display | `fileApi.getObjectUrl` later. |
| P0 | `src/components/chat/MessageInputAttachmentTray.tsx` | resolves OPFS URLs for attachment tray | `fileApi.getObjectUrl` later. |
| P1 | `src/components/media/ImagePreview.tsx` | resolves OPFS images | `fileApi.getObjectUrl` later. |
| P1 | `src/components/content/MarkdownRendererClient.tsx` | resolves OPFS assets in rendered content | local file resolver / `fileApi`. |
| P1 | `src/components/layout/WorkspaceSettingsModal.tsx` | `saveToOPFS`, `deleteFromOPFS` workspace files | `fileApi.upload/delete` local adapter first. |
| P1 | `src/components/knowledge/KnowledgeBase.tsx` | resolves OPFS knowledge files | `fileApi.getObjectUrl`; server mode later uses `fileId`. |
| P1 | `src/lib/data/appExport.ts` | collects `opfs://` references | Import/export contract later. |
| P1 | `src/lib/data/clearAppData.ts` | deletes OPFS directories | local reset API; server reset must be explicit. |
| P2 | `src/lib/chat/messageProcessor.ts` | resolves OPFS attachments for model input | `fileApi` + attachment normalization. |
| P2 | `src/lib/utils/rag.ts` | resolves OPFS files for RAG | RAG sidecar import path later. |
| P2 | `src/lib/utils/documentAttachments.ts` | parses uploaded files | document API later. |

## Service Imports in UI/Domain Code

These imports show where component behavior will feel the API boundary change.

| Priority | Import Consumers | Imported Service | Migration Note |
|---:|---|---|---|
| P0 | `src/components/app/ChatApp.tsx`, chat hooks/stores indirectly | `chatService`, skill/agent services | Main chat spine must wrap or replace `chatService` first. |
| P0 | `src/components/chat/MessageInput.tsx`, `MessageItem.tsx` | `chatService`, `voiceService`, artifact service | Keep UI stable; move backend route awareness into clients. |
| P1 | `src/components/settings/MemorySettings.tsx` | memory dream chat helper | Defer server memory; keep local capability gated. |
| P1 | `src/store/core/knowledgeStore.ts` | `docParseService`, `ragService` | Knowledge/RAG stays later phase. |
| P2 | `src/components/plugin/PluginMarket.tsx` | `pluginService` | `pluginApi` placeholder now; implementation deferred. |
| P2 | `src/components/assistant/*` | `agentService`, `chatService`, artifact service | Agent catalog can remain static/local until server catalog exists. |
| P2 | `src/components/content/*` | `chatService`, artifact service | Artifact/code execution helpers are not MVP chat spine. |
| P3 | `src/components/skill/SkillMarket.tsx` | `skillService` | Static assets; not server MVP. |

## Phase 2 Migration Order

Recommended implementation order after this inventory:

1. Add `createNeoChatApiClient` factory and shared contract types.
2. Wrap current `chatService` as `local.chatApi` without changing behavior.
3. Move direct config/model fetches in `ChatApp` and `ProviderSettings` behind `settingsApi` and `providerApi`.
4. Wrap OPFS helpers as `local.fileApi` and replace direct display resolvers gradually.
5. Add `server` mode HTTP adapter for `/v1/config`, `/health`, and provider model listing smoke tests.
6. Add server chat CRUD/SSE only after the local adapter passes parity tests.
7. Defer plugin, RAG, doc parse, voice, image generation, and code execution behind disabled capabilities.

## Risks and Guardrails

- Do not make components choose between local/server routes directly.
- Do not send `opfs://` URLs to Go except inside an explicit import payload.
- Do not expose MinIO object keys or direct object-store URLs to components.
- Do not move plugin execution into server mode until sandbox and confirmation flows are redesigned.
- Do not remove legacy local storage migrations before import/export compatibility is proven.
- Treat `NEXT_PUBLIC_API_MODE` as default/fallback only; runtime rollback should come from config endpoints when possible.

## Completion Criteria

This inventory completes the Phase 2 progress item “Identify components that directly call storage or fetch.” It should be revisited before actual code migration because call sites may change as the upstream app evolves.
