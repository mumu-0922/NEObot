# G18.5A retrieval profile pointer

This disposable PostgreSQL 16 harness validates the extension-free first stage
of the retrieval cutover. Migration `037` moves the Go candidate query behind a
server-owned profile pointer while retaining exact legacy behavior.

## Run

```bash
mm-chat/scripts/run-g18-profile-pointer-drill.sh
```

The drill applies all migrations to synthetic PG16 data and proves:

- the durable pointer defaults to `legacy` revision `1`;
- the profiled reader is row-for-row identical to the legacy hybrid reader;
- only runtime roles can read candidates and only `rag_replay_operator` can
  compare-and-swap the pointer;
- `pg17_bm25_pgvector_v1` fails closed until the PG17 storage migration exists;
- migration down refuses a non-legacy pointer atomically;
- a controlled rollback to legacy permits down/reapply and preserves the old
  reader.

Reports are retained under `/tmp/mm-chat-g18-profile-pointer.*`; the exit trap
removes all project-scoped containers, networks, and volumes.

## Files

- `20-verify-pointer.sql`: parity, role, failure, and hardened-path assertions.
- `25-verify-rollback-guard.sql`: failed-down atomicity proof.
- `30-verify-rollback.sql`: pointer-only rollback proof.
- `DESIGN.md`: cutover sequencing and trust boundary.
