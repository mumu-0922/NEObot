# Fix Knowledge search provider unavailable

## Goal

Restore selected Knowledge collection retrieval for live chat. A newly submitted
19:54 request against collection `test` currently completes the model answer but
records `dependency_unavailable`, zero Knowledge hits, and no citations.

## What I Already Know

- This is a fresh failure, not the historical 18:23 message.
- Memory recall filtering and Knowledge RAG are independent. The new
  `PJRSVY:gpt-5.6-luna` Memory Judge setting is not the Knowledge retrieval
  Provider.
- Backend, PostgreSQL, Redis, MinIO, Frontend, and RAG Worker are running;
  Backend readiness is green and the RAG Worker `/health` response is alive.
- Collection `test` has four active documents and four active current versions.
- The active Knowledge search generation uses the SiliconFlow BGE-M3 embedding
  and reranker profile and is marked active.
- `RAG:SILICONFLOW` is enabled, secret-bound, and has stored connection-test
  evidence.
- A live-safe SiliconFlow embedding probe returned HTTP 200, the frozen
  `Pro/BAAI/bge-m3` model, and 1,024 dimensions. The exact tool query from the
  failed turn returned 11 fenced hybrid candidates.
- The same failed turn's collection and session returned four fenced lexical
  candidates, and all four reauthorized and hydrated successfully. Upload,
  parsing, projections, lexical search, hybrid search, and session binding are
  therefore excluded as the primary failure.
- The deployment runs with `AUTH_MODE=required`. The selected server-stored
  Provider `PJRSVY` is enabled and has a valid connection attestation, but no
  `pjrsvy/server-stored/gpt-5.6-terra` answer consent exists for either the
  owner query scope or collection `test`.
- Startup answer-consent provisioning is currently gated to development mode,
  so an attested Provider can be usable for chat while remaining unusable for
  Knowledge answer egress in the required-auth deployment.
- The Backend currently collapses candidate, hydration, Provider, and answer
  governance failures into the same user-visible `dependency_unavailable`
  outcome without logging the internal stage.

## Requirements

- Identify the exact failing Knowledge retrieval stage from live-safe evidence.
- Preserve Provider-specific answer governance. Consent for `server-default`
  must not authorize a different server-stored endpoint merely because both
  implement an OpenAI-compatible protocol.
- Restore Knowledge search for active documents in collection `test` without
  weakening user/collection/generation/consent authorization.
- Preserve bounded fail-closed behavior: unavailable evidence must never be
  injected or cited.
- Keep Provider credentials, query text, and document plaintext out of logs and
  commits.
- Add regression or operational coverage for the discovered failure class.
- Deploy only the affected component(s), preserving runtime data and the
  independent Memory Provider selection.

## Acceptance Criteria

- [x] A controlled query against `test` completes Knowledge search without
      `dependency_unavailable`.
- [x] At least one matching active document can produce authorized `[K#]`
      evidence, while a genuine miss remains `no_evidence` rather than an error.
- [x] The effective retrieval generation remains the active SiliconFlow BGE-M3
      profile and Provider secrets remain server-only.
- [x] Backend/RAG focused tests and proportional build checks pass.
- [x] Backend and RAG health remain green after deployment.

## Definition of Done

- Root cause is evidenced from source, database state, or a sanitized synthetic
  runtime probe.
- Sanitized diagnostics identify the internal failure stage without persisting
  queries, credentials, document text, or raw upstream errors.
- Minimal repair is implemented or the invalid runtime configuration is safely
  corrected.
- Relevant RAG contract or operational note records the failure boundary.
- Work is committed, deployed, smoke-tested, and archived through Trellis.

## Out of Scope

- Changing Memory recall filtering or its fixed Luna model.
- Reprocessing documents that are already active unless evidence proves their
  projections are corrupt or incomplete.
- Broad retrieval ranking/relevance changes.
- Logging private Knowledge queries or document bodies.

## Technical Notes

- Knowledge execution: `mm-chat/backend/internal/chat/knowledge_tool.go`
- Assembly: `mm-chat/backend/internal/chat/rag_assembly.go`
- Runtime wiring: `mm-chat/backend/internal/httpserver/server.go`
- Retrieval Provider: `mm-chat/backend/internal/ragproviders/`
- Contract: `.trellis/spec/backend/rag-retrieval-storage.md`
