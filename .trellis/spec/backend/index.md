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
| [Hosted TTS production](./hosted-tts-production.md) | Dedicated SiliconFlow Voice authority, exact activation, server-mode playback, per-user cache, and cleanup |
| [Memory v2 benchmark](./memory-v2-benchmark.md) | Synthetic-only 500-case Golden lifecycle, strict observation/evaluation contracts, immutable reports, and non-promotional boundaries |

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

For production hosted TTS administration, runtime, playback, or cache changes:

1. Read [`hosted-tts-production.md`](./hosted-tts-production.md).
2. Trace encrypted ingress -> Voice vault -> exact attestation -> executor ->
   File artifact -> authenticated frontend playback.
3. Preserve TTS/STT capability separation and block every server-mode legacy
   Voice route/provider fallback.
4. Prove per-user source ownership, cache reuse, TTL/LRU reclamation, and
   replay-safe object deletion before live activation.

For Memory benchmark, Memory reader, recall-ranking, prompt-injection, or
reader-promotion changes:

1. Read [`memory-v2-benchmark.md`](./memory-v2-benchmark.md).
2. Keep corpus inputs synthetic-only and keep committed drafts explicitly
   ineligible for promotion.
3. Preserve frozen hashes, exact case order, one-shot Holdout, exclusive report
   output, and the separation between evaluation evidence and reader authority.
4. Prove current-fact, false-injection, cross-user, secret, deletion, Provider-
   egress, latency, token, and cost gates before changing a reader.

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
- For production TTS changes, also run migration `051` replay/cache integration,
  frontend server-route regressions, Compose rendering, and the standalone full
  gate. Live activation requires a fresh separately authorized Key.
- For Memory benchmark changes, run focused race tests for `internal/memoryeval`
  and `cmd/memory-eval`, all backend tests, and `go vet ./...`. Reader/runtime
  changes additionally require their owning migration, shadow, and rollback
  gates; PR1 alone does not require Docker or a Live Provider.

**Language**: All documentation should be written in English.
