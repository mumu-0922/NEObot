# Contract Docs

Contract documents define stable boundaries before implementation starts.

- [`frontend-api-client.md`](./frontend-api-client.md) — full frontend API client boundary for local/server migration, chat streaming, file access, auth, settings, provider metadata, plugin placeholders, errors, and rollout order.
- [`auth-session-api.md`](./auth-session-api.md) — Phase 15.1B Email/Password, Invite Acceptance, Recovery, authoritative Bearer resolution, bootstrap provisioning, and abuse-control contract.
- [`knowledge-acl-api.md`](./knowledge-acl-api.md) — Phase 15 contract for implemented Personal/Team Collections, Documents, File binding, Governance, Collection/User Consent, tombstones, and future fenced search.
- [`file-api.md`](./file-api.md) — Phase 6 backend file upload/download/delete contract above the object-store boundary.
- [`chat-crud-api.md`](./chat-crud-api.md) — Phase 5.1/6.3 backend REST contract for Postgres-backed conversation CRUD, completed user-message writes, server file attachment links, DB-disabled `503 DATABASE_REQUIRED` behavior, and non-goals.
- [`chat-stream-api.md`](./chat-stream-api.md) — Phase 5.2/5.4 backend SSE contract for provider-neutral assistant streaming, OpenAI-compatible provider wiring, durable run cancellation, mock-provider tests, and assistant-message finalization.
- [`chat-tool-loop.md`](./chat-tool-loop.md) — G19 provider-normalized multi-round Tool execution, strict three-state Search authority, reasoning/process SSE and persistence, approval classes, Knowledge/Web tools, and citation truth.
- [`conversation-context.md`](./conversation-context.md) — G11.13 server current-branch replay, model-aware input budgets, versioned Postgres rolling summaries, exact prefix validation, guarded untrusted-history injection, and recent-tail degradation.
- [`media-job-executor-seams.md`](./media-job-executor-seams.md) — G6.5c Voice/Image execution: admitted audit, dedicated runtime resolvers, stored artifacts, bounded provider responses, and fail-closed dependency/authority errors.
- [`voice-provider-reservation.md`](./voice-provider-reservation.md) — production `VOICE:SILICONFLOW` encrypted administration, exact CosyVoice2/`claire` attestation, TTS-only runtime capability, authenticated playback, and bounded cache cleanup.
- [`provider-live-smoke-authorization.md`](./provider-live-smoke-authorization.md) — G6.5e opt-in authorization contract for any quota-consuming live provider smoke.
- [`code-execution-sandbox-contract.md`](./code-execution-sandbox-contract.md) — G6.5d hard gate for enabling real code execution: sandbox isolation, storage, audit, cancellation, errors, and required tests.
- [`internal-evidence-api.md`](./internal-evidence-api.md) — Phase 15.2A frozen internal Go→Python Evidence Query contract for private-network mTLS, Ed25519 body/session/profile-bound compact JWS, replay prevention, source-reference-only responses, degradation, Go reauthorization/hydration, answer-purpose BYOK consent, and opaque session-bound citations; implementation remains pending.
- [`rag-query-hybrid-retrieval.md`](./rag-query-hybrid-retrieval.md) — G11.9C.2 private Python/Jina `retrieval.query` 1024 boundary, Postgres keyword/Dense RRF function, conservative pre-rerank signal gates, Go keyword degradation, and required live proofs.
- [`go-web-search-providers.md`](./go-web-search-providers.md) — G11.9E.1 closed Go Tavily/Firecrawl/Exa/Bocha adapter, HTTPS/SSRF/redirect/size/error fences, normalized result contract, and fixture gates.
- [`provider-wire-fixture.md`](./provider-wire-fixture.md) — Phase 15.2C C0 closed fixture envelope, draft/verified/frozen lifecycle, redaction/integrity gates, MinerU/Jina wire inputs, and test-only in-memory replay boundary; current public fixtures remain blocked drafts.
- [`provider-capture-harness.md`](./provider-capture-harness.md) — Phase 15.2C C0 no-network-by-default Jina/MinerU capture harness, fixed call budget, staged MinerU submit, canonical redacted evidence, manual freeze flow, and rollback boundary; both operator Captures are reviewed but Freeze remains blocked.
- [`provider-capture-promotion-readiness.md`](./provider-capture-promotion-readiness.md) — Evidence-to-Fixture promotion gate, operation-match audit, Terms authority review, MinerU Local Batch versus Remote URL mismatch, and fail-closed Freeze checklist.
- [`mineru-local-batch-draft-contract-plan.md`](./mineru-local-batch-draft-contract-plan.md) — draft-only MinerU Local Upload Batch operation/schema plan, Remote/Local isolation, unknown-wire rules, verification gates, and rollback boundary.
- [`mineru-lifecycle-capture-harness-plan.md`](./mineru-lifecycle-capture-harness-plan.md) — operator-only Allocate/Upload/Poll/Result lifecycle Capture plan, Evidence v2, dynamic-target/ZIP gates, bounded failures, and rollback boundary.
- [`mineru-lifecycle-evidence-promotion-plan.md`](./mineru-lifecycle-evidence-promotion-plan.md) — successful Lifecycle Summary-to-Fixture mapping plan, redaction boundary, residual blockers, verification gates, and rollback.
- [`browser-data-import.md`](./browser-data-import.md) — Phase 8 contract for explicit local-first browser data import, preview validation, ZIP package blobs, idempotency, and rollback.
- [`memory-benchmark-workflow.md`](./memory-benchmark-workflow.md) — Memory v2 PR1 synthetic-only 500-case Golden lifecycle, strict observation schema, deterministic scorer, immutable report, and non-promotional boundary.
- [`memory-governance-api.md`](./memory-governance-api.md) — Memory v2 PR9 authenticated Project/Conversation policy, scoped governance CRUD, Review/detail/Activity UI contracts, current-only plaintext hydration, and guarded rollout/rollback.

Future contract docs may cover SSE wire examples and database migration contracts.
