# Contract Docs

Contract documents define stable boundaries before implementation starts.

- [`frontend-api-client.md`](./frontend-api-client.md) — full frontend API client boundary for local/server migration, chat streaming, file access, auth, settings, provider metadata, plugin placeholders, errors, and rollout order.
- [`auth-session-api.md`](./auth-session-api.md) — Phase 15.1B Email/Password, Invite Acceptance, Recovery, authoritative Bearer resolution, bootstrap provisioning, and abuse-control contract.
- [`knowledge-acl-api.md`](./knowledge-acl-api.md) — Phase 15 contract for implemented Personal/Team Collections, Documents, File binding, Governance, Collection/User Consent, tombstones, and future fenced search.
- [`file-api.md`](./file-api.md) — Phase 6 backend file upload/download/delete contract above the object-store boundary.
- [`chat-crud-api.md`](./chat-crud-api.md) — Phase 5.1/6.3 backend REST contract for Postgres-backed conversation CRUD, completed user-message writes, server file attachment links, DB-disabled `503 DATABASE_REQUIRED` behavior, and non-goals.
- [`chat-stream-api.md`](./chat-stream-api.md) — Phase 5.2/5.4 backend SSE contract for provider-neutral assistant streaming, OpenAI-compatible provider wiring, durable run cancellation, mock-provider tests, and assistant-message finalization.
- [`internal-evidence-api.md`](./internal-evidence-api.md) — Phase 15.2A frozen internal Go→Python Evidence Query contract for private-network mTLS, Ed25519 body/session/profile-bound compact JWS, replay prevention, source-reference-only responses, degradation, Go reauthorization/hydration, answer-purpose BYOK consent, and opaque session-bound citations; implementation remains pending.
- [`provider-wire-fixture.md`](./provider-wire-fixture.md) — Phase 15.2C C0 closed fixture envelope, draft/verified/frozen lifecycle, redaction/integrity gates, MinerU/Jina wire inputs, and test-only in-memory replay boundary; current public fixtures remain blocked drafts.
- [`provider-capture-harness.md`](./provider-capture-harness.md) — Phase 15.2C C0 no-network-by-default Jina/MinerU capture harness, fixed call budget, staged MinerU submit, canonical redacted evidence, manual freeze flow, and rollback boundary; no real capture has been executed.
- [`browser-data-import.md`](./browser-data-import.md) — Phase 8 contract for explicit local-first browser data import, preview validation, ZIP package blobs, idempotency, and rollback.

Future contract docs may cover SSE wire examples and database migration contracts.
