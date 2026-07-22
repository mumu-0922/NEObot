# Backend Development Guidelines

> Executable contracts for the independent `mm-chat` Go/PostgreSQL backend.

## Guidelines Index

| Guide | Scope |
|---|---|
| [RAG retrieval storage](./rag-retrieval-storage.md) | PostgreSQL major-version, BM25/pgvector shadow, authority, diagnostics, and rollback contracts |

## Pre-Development Checklist

For RAG retrieval, PostgreSQL migration, or indexing changes:

1. Read [`rag-retrieval-storage.md`](./rag-retrieval-storage.md).
2. Confirm the running PostgreSQL major and extension availability before
   adding ordinary migrations.
3. Identify the current-authority hydration boundary and rollback artifact.
4. Define a synthetic Golden/negative proof before changing the reader.

## Quality Check

- Run the disposable database drill for the changed storage group.
- Run `go vet ./...` and `go test ./...` from `mm-chat/backend`.
- Prove SECURITY DEFINER paths, role grants, deletion invisibility, migration
  manifest stability, and cleanup.
- Update the G18 plan/process records when the retrieval program is affected.

**Language**: All documentation should be written in English.
