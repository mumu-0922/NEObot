# Backend Development Guidelines

> Executable contracts for the independent `mm-chat` Go/PostgreSQL backend.

## Guidelines Index

| Guide                                               | Scope                                                                                                                 |
| --------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| [RAG retrieval storage](./rag-retrieval-storage.md) | PostgreSQL retrieval, Citation authority/display, diagnostics, and rollback contracts                              |
| [Chat source fusion](./chat-source-fusion.md)       | Conversation-aware external Search query rewriting, Knowledge/Web authority, diagnostics, and fallback contracts      |
| [Planned chat Tool Loop](./chat-tool-loop.md)       | G19 provider-normalized Tool rounds, three-state Search authority, process persistence, approvals, and citation truth |
| [Direct chat attachments](./chat-attachments.md)    | Attachment-only messages, native images, bounded document extraction, provider context, and explicit failures       |
| [Hosted media provider smoke](./provider-live-smoke.md) | Exact live-provider authorization, one-off credentials, explicit TTS voices, artifacts, and sanitized evidence    |

## Pre-Development Checklist

For RAG retrieval, PostgreSQL migration, or indexing changes:

1. Read [`rag-retrieval-storage.md`](./rag-retrieval-storage.md).
2. Confirm the running PostgreSQL major and extension availability before
   adding ordinary migrations.
3. Identify the current-authority hydration boundary and rollback artifact.
4. Define a synthetic Golden/negative proof before changing the reader.

For external Search or Knowledge/Web fusion changes:

1. Read [`chat-source-fusion.md`](./chat-source-fusion.md).
2. Prove the active-branch context and runtime model identity reaching the
   rewrite boundary.
3. Keep exact query text and source bodies out of durable diagnostics.
4. Define rewrite-success, unchanged, and fail-open tests before changing the
   Search request.

For G19 Tool Loop or durable process-trace changes:

1. Read [`chat-tool-loop.md`](./chat-tool-loop.md).
2. Identify the owning G19 group and keep unpromoted behavior behind the
   current source-fusion rollback path.
3. Prove provider-native continuation, cancellation, redaction, and current-
   turn Citation reconciliation before promotion.

For chat upload, attachment parsing, or provider attachment changes:

1. Read [`chat-attachments.md`](./chat-attachments.md).
2. Trace file bytes from upload storage through the current provider request.
3. Preserve native images, explicit document failures, context limits, and the
   untrusted-data boundary.
4. Prove attachment-only acceptance and pre-acceptance draft restoration.

For hosted Voice/Image executor or live-smoke changes:

1. Read [`provider-live-smoke.md`](./provider-live-smoke.md).
2. Identify the exact kind/provider/model and any provider-qualified voice.
3. Preserve default denial, one-off credential isolation, and sanitized
   evidence before making a provider call.
4. Define offline zero-network tests and independent output validation before
   running the authorized live smoke.

## Quality Check

- Run the disposable database drill for the changed storage group.
- Run `go vet ./...` and `go test ./...` from `mm-chat/backend`.
- Prove SECURITY DEFINER paths, role grants, deletion invisibility, migration
  manifest stability, and cleanup.
- Update the G18 plan/process records when the retrieval program is affected.
- Run real external-Search hit/follow-up proof and update the G11 source-fusion
  process when query planning is affected.
- For attachment changes, run parser units, image-path regression,
  attachment-only API/UI tests, and one live upload-to-answer replay.
- For hosted provider smoke changes, run focused executor/gate tests, all Go
  tests and vet, a diff secret scan, then the exact authorized live command.

**Language**: All documentation should be written in English.
