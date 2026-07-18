# API Route Inventory

This inventory maps current Next.js API routes to future `mm-chat` Go backend ownership. It is based on static route inspection under `src/app/api`.

## Summary

Current server routes are mostly proxy/adaptor endpoints around chat
generation and voice. Web Search has moved to Go.

## Route Table

| Current Route                 | Method | Current Responsibility | Future Owner                                           |
| ----------------------------- | -----: | ---------------------- | ------------------------------------------------------ |
| `/api/health`                 |    GET | Health check           | Go `/health`, `/ready`                                 |
| `/api/access/verify`          |   POST | Access/password gate   | Go auth/access middleware                              |
| `/api/chat`                   |   POST | Main chat stream       | Go chat streaming spine                                |
| `/api/chat/generate`          |   POST | Simple generate stream | Go provider generate endpoint                          |
| `/api/chat/generate-title`    |   POST | Title generation       | Go chat helper or async job                            |
| `/api/chat/related-questions` |   POST | Follow-up generation   | Go chat helper                                         |
| `/api/chat/generate-image`    |   POST | Image generation       | Go provider adapter or later worker                    |
| `/api/chat/execute-code`      |   POST | Code execution helper  | Separate sandbox service; do not put in core initially |
| `/api/voice/transcribe`       |   POST | Speech-to-text         | Go proxy or Python/media service later                 |
| `/api/voice/synthesize`       |   POST | Text-to-speech         | Go proxy or media service later                        |

## Retired During G9

| Retired Route              | Former Method | Replacement                                               |
| -------------------------- | ------------: | --------------------------------------------------------- |
| `/api/chat/rag-queries`    |          POST | Server strict Knowledge/RAG query planning through Go/RAG |
| `/api/doc-parse`           |          POST | Go Knowledge document upload/index pipeline               |
| `/api/doc-parse/jobs/[id]` |    GET/DELETE | Go Knowledge document status/deletion APIs                |
| `/api/rag/delete`          |          POST | Go Knowledge document/collection deletion events          |
| `/api/rag/query`           |          POST | Go strict RAG query path                                  |
| `/api/rag/upsert`          |          POST | Go Knowledge auto-index pipeline                          |
| `/api/config`              |           GET | Go `/v1/config` through API client                        |
| `/api/providers/models`    |          POST | Go `/v1/providers/models` through API client              |
| `/api/byok/public-key`     |           GET | Go `/v1/byok/public-key` through API client               |
| `/api/agents`              |           GET | Go `/v1/agents` through API client                        |
| `/api/agents/[identifier]` |           GET | Go `/v1/agents/{identifier}` through API client           |
| `/api/plugins/list`        |           GET | Go `/v1/plugins` through API client                       |
| `/api/plugins/install`     |          POST | Go `/v1/plugins/install` through API client               |
| `/api/plugins/execute`     |          POST | Go `/v1/plugins/execute` through API client               |
| `/api/search`              |          POST | Go chat stream plus authenticated `/v1/search`            |

## Migration Priority

1. `health` — low risk smoke test for Go backend.
2. `chat`, `chat/generate` — core streaming path.
3. `providers/models` — provider metadata and server-side secret isolation.
4. `files` — new Go endpoints; current app has OPFS rather than server file API.
5. `voice` — later services after chat spine is stable.

## Replacement API Sketch

```text
GET    /health
GET    /ready
GET    /v1/config
GET    /v1/version

POST   /v1/chat/conversations
GET    /v1/chat/conversations
GET    /v1/chat/conversations/:id/messages
POST   /v1/chat/conversations/:id/messages
POST   /v1/chat/conversations/:id/stream
POST   /v1/chat/runs/:runId/cancel

POST   /v1/providers/models
POST   /v1/files
GET    /v1/files/:id
GET    /v1/files/:id/content
DELETE /v1/files/:id
```

## Risks

- Current `/api/chat` accepts provider runtime config from the browser; server-backed mode should move provider secret resolution server-side.
- Some helper routes are intertwined with local memory, plugin, search, and RAG logic from `chatService.ts`; extract the core chat path first.
- Plugin execution and code execution are high-risk and should not be part of the first backend MVP.

## Verification for This Inventory

Static route list was collected from `src/app/api/**/route.ts` and method exports were identified for each route.
