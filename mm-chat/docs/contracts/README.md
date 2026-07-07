# Contract Docs

Contract documents define stable boundaries before implementation starts.

- [`frontend-api-client.md`](./frontend-api-client.md) — full frontend API client boundary for local/server migration, chat streaming, file access, auth, settings, provider metadata, plugin placeholders, errors, and rollout order.
- [`file-api.md`](./file-api.md) — Phase 6 backend file upload/download/delete contract above the object-store boundary.
- [`chat-crud-api.md`](./chat-crud-api.md) — Phase 5.1 backend REST contract for Postgres-backed conversation CRUD and completed user-message writes, including DB-disabled `503 DATABASE_REQUIRED` behavior and non-goals.
- [`chat-stream-api.md`](./chat-stream-api.md) — Phase 5.2/5.4 backend SSE contract for provider-neutral assistant streaming, OpenAI-compatible provider wiring, durable run cancellation, mock-provider tests, and assistant-message finalization.

Future contract docs may cover SSE wire examples, import/export schemas, and database migration contracts.
