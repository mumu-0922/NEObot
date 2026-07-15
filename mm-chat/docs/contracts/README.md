# Contract Docs

Contract documents define stable boundaries before implementation starts.

- [`frontend-api-client.md`](./frontend-api-client.md) — full frontend API client boundary for local/server migration, chat streaming, file access, auth, settings, provider metadata, plugin placeholders, errors, and rollout order.
- [`auth-session-api.md`](./auth-session-api.md) — Phase 15.1B Email/Password, Invite Acceptance, Recovery, authoritative Bearer resolution, bootstrap provisioning, and abuse-control contract.
- [`knowledge-acl-api.md`](./knowledge-acl-api.md) — Phase 15 contract for implemented Personal/Team Collections, Documents, File binding, Governance, Collection/User Consent, tombstones, and future fenced search.
- [`file-api.md`](./file-api.md) — Phase 6 backend file upload/download/delete contract above the object-store boundary.
- [`chat-crud-api.md`](./chat-crud-api.md) — Phase 5.1/6.3 backend REST contract for Postgres-backed conversation CRUD, completed user-message writes, server file attachment links, DB-disabled `503 DATABASE_REQUIRED` behavior, and non-goals.
- [`chat-stream-api.md`](./chat-stream-api.md) — Phase 5.2/5.4 backend SSE contract for provider-neutral assistant streaming, OpenAI-compatible provider wiring, durable run cancellation, mock-provider tests, and assistant-message finalization.
- [`media-job-executor-seams.md`](./media-job-executor-seams.md) — G6.5c voice/image executor opt-in seams: admitted audit, artifact storage, fail-closed defaults, no inline bytes, and no quota-consuming provider calls by default.
- [`code-execution-sandbox-contract.md`](./code-execution-sandbox-contract.md) — G6.5d hard gate for enabling real code execution: sandbox isolation, storage, audit, cancellation, errors, and required tests.
- [`internal-evidence-api.md`](./internal-evidence-api.md) — Phase 15.2A frozen internal Go→Python Evidence Query contract for private-network mTLS, Ed25519 body/session/profile-bound compact JWS, replay prevention, source-reference-only responses, degradation, Go reauthorization/hydration, answer-purpose BYOK consent, and opaque session-bound citations; implementation remains pending.
- [`provider-wire-fixture.md`](./provider-wire-fixture.md) — Phase 15.2C C0 closed fixture envelope, draft/verified/frozen lifecycle, redaction/integrity gates, MinerU/Jina wire inputs, and test-only in-memory replay boundary; current public fixtures remain blocked drafts.
- [`provider-capture-harness.md`](./provider-capture-harness.md) — Phase 15.2C C0 no-network-by-default Jina/MinerU capture harness, fixed call budget, staged MinerU submit, canonical redacted evidence, manual freeze flow, and rollback boundary; both operator Captures are reviewed but Freeze remains blocked.
- [`provider-capture-promotion-readiness.md`](./provider-capture-promotion-readiness.md) — Evidence-to-Fixture promotion gate, operation-match audit, Terms authority review, MinerU Local Batch versus Remote URL mismatch, and fail-closed Freeze checklist.
- [`mineru-local-batch-draft-contract-plan.md`](./mineru-local-batch-draft-contract-plan.md) — draft-only MinerU Local Upload Batch operation/schema plan, Remote/Local isolation, unknown-wire rules, verification gates, and rollback boundary.
- [`mineru-lifecycle-capture-harness-plan.md`](./mineru-lifecycle-capture-harness-plan.md) — operator-only Allocate/Upload/Poll/Result lifecycle Capture plan, Evidence v2, dynamic-target/ZIP gates, bounded failures, and rollback boundary.
- [`mineru-lifecycle-evidence-promotion-plan.md`](./mineru-lifecycle-evidence-promotion-plan.md) — successful Lifecycle Summary-to-Fixture mapping plan, redaction boundary, residual blockers, verification gates, and rollback.
- [`browser-data-import.md`](./browser-data-import.md) — Phase 8 contract for explicit local-first browser data import, preview validation, ZIP package blobs, idempotency, and rollback.

Future contract docs may cover SSE wire examples and database migration contracts.
