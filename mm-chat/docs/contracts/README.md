# Contract Docs

Contract documents define stable boundaries before implementation starts.

- [`frontend-api-client.md`](./frontend-api-client.md) — full frontend API client boundary for local/server migration, chat streaming, file access, auth, settings, provider metadata, plugin placeholders, errors, and rollout order.
- [`file-api.md`](./file-api.md) — Phase 6 backend file upload/download/delete contract above the object-store boundary.
- [`chat-crud-api.md`](./chat-crud-api.md) — Phase 5.1/6.3 backend REST contract for Postgres-backed conversation CRUD, completed user-message writes, server file attachment links, DB-disabled `503 DATABASE_REQUIRED` behavior, and non-goals.
- [`chat-stream-api.md`](./chat-stream-api.md) — Phase 5.2/5.4 backend SSE contract for provider-neutral assistant streaming, OpenAI-compatible provider wiring, durable run cancellation, mock-provider tests, and assistant-message finalization.
- [`browser-data-import.md`](./browser-data-import.md) — Phase 8 contract for explicit local-first browser data import, preview validation, ZIP package blobs, idempotency, and rollback.

Future contract docs may cover SSE wire examples and database migration contracts.
