# Backend Development Guidelines

> Executable contracts for the independent `mm-chat` Go/PostgreSQL backend.

## Guidelines Index

| Guide                                               | Scope                                                                                                                 |
| --------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| [RAG retrieval storage](./rag-retrieval-storage.md) | PostgreSQL major-version, BM25/pgvector shadow, authority, diagnostics, and rollback contracts                        |
| [Chat source fusion](./chat-source-fusion.md)       | Conversation-aware external Search query rewriting, Knowledge/Web authority, diagnostics, and fallback contracts      |
| [Planned chat Tool Loop](./chat-tool-loop.md)       | G19 provider-normalized Tool rounds, three-state Search authority, process persistence, approvals, and citation truth |

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

## Quality Check

- Run the disposable database drill for the changed storage group.
- Run `go vet ./...` and `go test ./...` from `mm-chat/backend`.
- Prove SECURITY DEFINER paths, role grants, deletion invisibility, migration
  manifest stability, and cleanup.
- Update the G18 plan/process records when the retrieval program is affected.
- Run real external-Search hit/follow-up proof and update the G11 source-fusion
  process when query planning is affected.

**Language**: All documentation should be written in English.
