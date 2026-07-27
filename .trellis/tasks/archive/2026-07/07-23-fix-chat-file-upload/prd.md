# Fix Chat Attachment Parsing and Context Injection

## Goal

Ensure supported chat attachments are not merely uploaded and linked to a
message, but are also parsed and made available to the active model context so
the assistant can answer questions about their contents.

## What I already know

- The reported screenshot contains a text prompt (`这是啥`) and a selected
  `g18-rag-acceptance.txt` attachment.
- Upload and persistence are healthy: live Chromium replay reached file upload
  (`201`), message creation (`201`), and stream startup (`200`).
- The generic model reply proves that the uploaded document body was not added
  to model context.
- The live standalone stack serves `mm-chat/frontend/` and uses the Go backend
  under `mm-chat/backend/`.
- The current backend provider attachment resolver only materializes image
  attachments. Text, PDF, and Office attachments remain metadata/file
  references and are invisible to the provider.
- Attachment-only send is a separate adjacent defect: frontend, service, and
  repository validation require non-empty message content, and the composer
  clears state before that failure is returned.
- The repository contains native parser source under
  `mm-chat/rag/src/mm_chat_rag/offline_parser/native/`, but the production
  RAG worker keeps that output behind its Child-internal sidecar boundary and
  does not currently expose a successful `canonical-ir.v2` response. Chat
  attachments must not bypass that sandbox contract or pretend the parser is
  production-callable.
- The working tree has hundreds of pre-existing changes. Implementation must
  not reset, clean, or silently include unrelated work.

## Research References

- [`research/attachment-processing-patterns.md`](research/attachment-processing-patterns.md)
  — OpenAI, Anthropic, and Gemini all expose a direct-document path while
  retrieval systems provide a second path for large or reusable documents.
- [`research/cherry-kelivo-attachments.md`](research/cherry-kelivo-attachments.md)
  — Cherry uses provider-aware native/extracted routing with an 8k inline cap
  and `read_file` overflow; Kelivo injects locally extracted full text directly
  into user messages.

## Requirements

- Preserve the existing image multimodal path.
- Parse supported non-image attachments on the server side rather than
  concatenating browser-decoded base64 into the prompt.
- Make parsed content available to the same model turn that references the
  attachment.
- Bound injected document context by explicit size/token limits.
- Accept source documents up to 20 MiB per file while retaining the existing
  60,000-character per-file and 160,000-character combined context bounds.
- Treat extracted document text as untrusted data and isolate it from system
  instructions.
- Keep the composer/send state pending while attachment bytes are uploaded and
  parsed, and surface an actionable failure instead of silently sending a
  document-dependent prompt without document content.
- Allow attachment-only messages; do not clear the composer until the send is
  durably accepted.
- Accept OpenAI-compatible streams that end with a standard `finish_reason`
  and clean EOF even when the gateway omits the optional trailing `[DONE]`.
  Markerless partial EOF must remain a visible provider failure.
- Do not create embeddings, indexes, projections, or an automatic RAG lifecycle
  for ordinary chat attachments.
- Reject or visibly truncate attachments above the direct-context boundary;
  normal chat usage is expected to use small and medium files.
- Preserve at least filename provenance in the injected document block.

## Acceptance Criteria

- [x] Sending `这是啥` with a small `.txt` fixture results in a response based
      on the fixture contents, not a generic request to provide content.
- [x] Supported text/Office files are parsed synchronously at the provider
      boundary and either enter bounded direct context or show an actionable
      failure.
- [x] PDF files use a bounded backend-local text extractor (not the closed RAG
      parser path) and are available to the model through direct context.
- [x] Images continue to reach providers as native multimodal attachments.
- [x] Inputs above the configured direct-context boundary are rejected or
      visibly truncated; no unbounded prompt growth occurs.
- [x] A message containing only a supported attachment is accepted and sent.
- [x] A failed send preserves the draft text and selected attachments.
- [x] Extracted content cannot override system/developer instructions by being
      concatenated as trusted prompt text.
- [x] Targeted frontend, backend, and parser tests pass; typecheck and lint
      remain green for touched packages.
- [x] The live Chromium replay of the screenshot scenario succeeds.
- [x] A PDF parse followed by `finish_reason` + clean EOF completes normally;
      a partial stream without `[DONE]` or `finish_reason` remains failed.

## Definition of Done (team quality bar)

- Tests cover upload-to-parse-to-context flow, failure state, size boundary,
  and image-path regression.
- Targeted lint, typecheck, tests, and relevant builds pass.
- Durable tracked source is identified before editing the live
  `mm-chat/frontend/` runtime copy.
- Rollback surface and unrelated dirty files are documented.

## Decision (ADR-lite)

**Context**: Upload storage and message linkage work, but non-image document
contents never cross the provider boundary. A browser-only concatenation patch
would duplicate parser logic, weaken security controls, and behave differently
across providers.

**Decision**: Implement Cherry-style provider-aware routing without RAG.
Parse ordinary small and medium documents synchronously in the Go backend and
inject bounded document blocks directly. Keep native image behavior. Do not
call the production-closed Python RAG parser or send non-image documents
through provider image contracts.

**Consequences**: Ordinary files become immediately understandable without an
indexing lifecycle. A hard direct-context boundary is required; files above it
will be rejected or visibly truncated rather than silently consuming unbounded
context. RAG, embeddings, and retrieval are not part of this task.

## Out of Scope (explicit)

- Full attachment-management or knowledge-base redesign.
- Automatic RAG, embeddings, chunk indexes, retrieval, or a `read_file` paging
  tool for chat attachments.
- Optimizing for unusually large uploads beyond a clear size/context boundary.
- OCR for arbitrary scanned images beyond the existing PDF/image capabilities.
- Changing provider-native image behavior.
- Deleting probe conversations/files or unrelated user data.

## Technical Notes

- Active frontend paths:
  `mm-chat/frontend/src/components/chat/MessageInput.tsx`,
  `mm-chat/frontend/src/components/app/ChatApp.tsx`, and
  `mm-chat/frontend/src/services/api/fileService.ts`.
- Active backend paths: `mm-chat/backend/internal/chat/service.go`,
  `mm-chat/backend/internal/chat/repository_postgres.go`, and
  `mm-chat/backend/internal/chat/handler.go`.
- Runtime: frontend `http://127.0.0.1:18080`, backend
  `http://127.0.0.1:8080`, API proxy `/mm-api`.
- Before implementation, determine whether `mm-chat/frontend/` is generated,
  ignored, or synchronized from another tracked source.
